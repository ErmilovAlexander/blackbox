# Сборка и запуск в Kubernetes

## 1. Требования

- Go 1.26+ для локальной сборки;
- Docker/Podman или совместимый OCI builder;
- `kubectl` с контекстом целевого кластера;
- права на создание Namespace, ClusterRole, ClusterRoleBinding, PVC и Deployment;
- default StorageClass либо заранее выбранный `storageClassName`;
- registry, доступный worker-узлам, если это не локальный kind/minikube cluster.

Проверьте контекст до любых изменений:

```bash
kubectl config current-context
kubectl cluster-info
kubectl get storageclass
```

## 2. Проверка исходников и локальная сборка

```bash
make verify
./bin/kube-blackbox version
```

`make verify` выполняет format check, `go vet`, unit/race-тесты, собирает бинарник и рендерит Kustomize manifests.

Чтобы проверить recorder вне кластера:

```bash
./bin/kube-blackbox recorder \
  --kubeconfig=/absolute/path/to/kubeconfig \
  --cluster=development \
  --data-dir=./data \
  --retention=24h \
  --max-store-bytes=2147483648 \
  --max-segment-bytes=67108864
```

Пользователь из kubeconfig должен иметь `get/list/watch` на те же ресурсы, что перечислены в `deploy/rbac.yaml`.

## 3. Сборка image

Локальный tag:

```bash
make image IMAGE=kube-blackbox:dev VERSION=dev
docker run --rm kube-blackbox:dev version
```

Для registry используйте неизменяемый tag или digest:

```bash
export KBB_IMAGE=registry.example.com/platform/kube-blackbox:v0.1.0
docker build --build-arg VERSION=v0.1.0 -t "${KBB_IMAGE}" .
docker push "${KBB_IMAGE}"
```

Для multi-architecture кластера:

```bash
docker buildx build \
  --platform=linux/amd64,linux/arm64 \
  --build-arg VERSION=v0.1.0 \
  -t "${KBB_IMAGE}" \
  --push .
```

## 4. Настройка manifests

До установки проверьте `deploy/deployment.yaml`:

- `--cluster` — логическое имя кластера, которое попадёт во все records;
- `--retention` — окно хранения;
- `--max-store-bytes` — общий предел retained segments;
- `--max-segment-bytes` — размер segment, не больше `max-store-bytes`;
- PVC size — должен быть больше `max-store-bytes` с запасом для filesystem overhead;
- `resources.limits.memory` — увеличьте для большого количества объектов.

Если default StorageClass отсутствует, добавьте в PVC:

```yaml
spec:
  storageClassName: your-storage-class
```

Для production также настройте шифрование volume на уровне выбранного CSI/storage backend.

## 5. Установка

Сначала отрендерите итоговый YAML:

```bash
kubectl kustomize deploy
```

Локальный kind cluster:

```bash
kind load docker-image kube-blackbox:dev
kubectl apply -k deploy
```

Локальный minikube cluster:

```bash
minikube image load kube-blackbox:dev
kubectl apply -k deploy
```

Удалённый cluster с registry:

```bash
kubectl apply -k deploy
kubectl -n kube-blackbox set image deployment/kube-blackbox recorder="${KBB_IMAGE}"
```

Если registry приватный, создайте `imagePullSecret` по правилам вашей платформы и добавьте его в `spec.template.spec.imagePullSecrets`.

Дождитесь запуска:

```bash
kubectl -n kube-blackbox rollout status deployment/kube-blackbox --timeout=120s
kubectl -n kube-blackbox get pod,pvc
kubectl -n kube-blackbox logs deployment/kube-blackbox --tail=100
```

Успешный initial LIST подтверждается JSON-log сообщением `initial Kubernetes snapshots persisted`.

## 6. Проверка RBAC

```bash
export KBB_SUBJECT=system:serviceaccount:kube-blackbox:kube-blackbox

kubectl auth can-i list pods --all-namespaces --as="${KBB_SUBJECT}"
kubectl auth can-i watch networkpolicies.networking.k8s.io --all-namespaces --as="${KBB_SUBJECT}"
kubectl auth can-i get secrets --all-namespaces --as="${KBB_SUBJECT}"
kubectl auth can-i create pods --all-namespaces --as="${KBB_SUBJECT}"
```

Ожидаемый результат: первые две команды — `yes`, последние две — `no`.

## 7. Чтение истории

Краткая timeline:

```bash
kubectl -n kube-blackbox exec deployment/kube-blackbox -- \
  /kube-blackbox timeline \
  --data-dir=/var/lib/kube-blackbox \
  --namespace=default \
  --kind=Pod \
  --limit=100
```

Полные очищенные records:

```bash
kubectl -n kube-blackbox exec deployment/kube-blackbox -- \
  /kube-blackbox timeline \
  --data-dir=/var/lib/kube-blackbox \
  --namespace=default \
  --kind=Pod \
  --output=records \
  --limit=100
```

Чтение открывает store в read-only режиме и не создаёт новый JSONL segment.

Structured diff для одного объекта:

```bash
kubectl -n kube-blackbox exec deployment/kube-blackbox -- \
  /kube-blackbox diff \
  --data-dir=/var/lib/kube-blackbox \
  --namespace=default \
  --kind=Deployment \
  --name=api \
  --from=2026-09-08T09:00:00Z \
  --to=2026-09-08T10:00:00Z
```

Команда исключает initial `SNAPSHOT` и no-op updates. Для object maps она выдаёт path-sorted изменения `ADD`, `REMOVE`, `REPLACE`; массивы сравниваются целиком.

## 8. Автоматический smoke-тест

```bash
make smoke
```

Тест создаёт временный namespace и ConfigMap, ждёт `ADD`, изменяет labels и ждёт `UPDATE`, удаляет объект и ждёт `DELETE`. После удаления он проверяет retained evidence, structured diff, отсутствие ConfigMap payload и отсутствие лишних RBAC-прав. Временный namespace удаляется автоматически.

## 9. Диагностика

`PVC Pending`:

```bash
kubectl -n kube-blackbox describe pvc kube-blackbox-data
kubectl get storageclass
```

Выберите существующий StorageClass или установите default StorageClass.

`ImagePullBackOff`:

```bash
kubectl -n kube-blackbox describe pod -l app.kubernetes.io/name=kube-blackbox
```

Проверьте tag, architecture, доступ worker-а к registry и imagePullSecret.

`permission denied` для `/var/lib/kube-blackbox`:

Проверьте поддержку `fsGroup` у CSI driver. Для backend-а без fsGroup потребуется platform-specific volume permission setup; не запускайте основной контейнер от root только ради обхода проблемы.

Informer пишет `forbidden`:

```bash
kubectl -n kube-blackbox logs deployment/kube-blackbox
kubectl describe clusterrole kube-blackbox-readonly
kubectl describe clusterrolebinding kube-blackbox-readonly
```

Recorder не должен получать доступ к Secrets или mutation verbs; исправляйте отсутствующие read permissions точечно.

## 10. Обновление и удаление

Обновление image:

```bash
kubectl -n kube-blackbox set image deployment/kube-blackbox recorder=registry.example.com/platform/kube-blackbox:v0.1.1
kubectl -n kube-blackbox rollout status deployment/kube-blackbox --timeout=120s
```

Deployment использует стратегию `Recreate`, чтобы не было двух writers на одном RWO volume.

Удаление workload и RBAC без удаления данных:

```bash
kubectl -n kube-blackbox delete deployment kube-blackbox
kubectl delete clusterrolebinding kube-blackbox-readonly
kubectl delete clusterrole kube-blackbox-readonly
```

PVC и Namespace намеренно не удаляются этими командами. Удаляйте PVC отдельно только после сохранения нужных evidence: удаление PVC может быть необратимым и зависит от reclaim policy StorageClass.

## Официальные справочные материалы

- [Kustomize и `kubectl apply -k`](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization/)
- [RBAC: Role, ClusterRole и binding](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [Pod securityContext, fsGroup и fsGroupChangePolicy](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/)
- [Загрузка image из приватного registry](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)
