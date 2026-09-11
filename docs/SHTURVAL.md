# Развёртывание и проверка в Shturval 2.14

Инструкция проверена на management-кластере `demo214`. Сервис устанавливается в
namespace `blackbox`, получает image из локального Nexus и запускается только на
infra-узлах. Системные Pod и control plane не изменяются.

## Артефакты выпуска

| Артефакт | Значение |
|---|---|
| Image | `mirror.ip-10-28-32-189.shturval.link/kube-blackbox:4faa2b2` |
| Image digest | `sha256:5b7a52c94e04dbd65425db71da428f6a1175a10cf3b4c7596e204211e1c6d557` |
| Платформа | `linux/amd64` |
| Nexus Docker hosted repository | `kube-blackbox` |
| Nexus Docker group | `r.shturval.tech-group` |
| Nexus Helm repository | `https://mirror.ip-10-28-32-189.shturval.link/repository/shturval_helm/` |
| Shturval repository | `mirror-stp` |
| Chart | `kube-blackbox` |
| Chart version | `0.1.3` |
| Chart SHA-256 | `21a6cd0c24a81cd536908d94722afd8a725f1965d4e8dd022e90cbba1fff4253` |

Chart использует image по digest. Повторная публикация того же тега не изменит
исполняемый образ.

## Что создаёт chart

- `Deployment` со стратегией `Recreate`;
- `PersistentVolumeClaim` размером 5 GiB либо подключение существующего PVC;
- отдельный `ServiceAccount`;
- read-only `ClusterRole` и `ClusterRoleBinding` только с `get`, `list`, `watch`;
- запуск от UID/GID `65532`, read-only root filesystem и удалённые capabilities;
- размещение только на `linux/amd64` infra-узлах.

Recorder читает 13 типов Kubernetes-ресурсов cluster-wide. Он не имеет прав на
Secrets, `pods/log`, `pods/exec` или изменение workloads.

## Доверие CA Nexus без рестарта containerd

Nexus выдаёт self-signed сертификат для
`mirror.ip-10-28-32-189.shturval.link`. Проверенный SHA-256 fingerprint:

```text
3D:D7:AC:14:83:A9:99:F8:17:95:03:FF:2B:98:36:DF:9D:4E:C1:55:86:22:8B:ED:AA:64:12:98:C2:9E:DC:0D
```

CA устанавливается отдельно на каждом infra-узле только для hostname Nexus:

```text
/etc/containerd/certs.d/mirror.ip-10-28-32-189.shturval.link/ca.crt
```

Безопасная установка:

```bash
tmpfile=$(mktemp)
trap 'rm -f "$tmpfile"' EXIT

openssl s_client \
  -connect mirror.ip-10-28-32-189.shturval.link:443 \
  -servername mirror.ip-10-28-32-189.shturval.link \
  </dev/null 2>/dev/null \
  | openssl x509 -outform PEM > "$tmpfile"

openssl x509 -in "$tmpfile" -noout -fingerprint -sha256

sudo install -d -m 0755 \
  /etc/containerd/certs.d/mirror.ip-10-28-32-189.shturval.link
sudo install -o root -g root -m 0644 "$tmpfile" \
  /etc/containerd/certs.d/mirror.ip-10-28-32-189.shturval.link/ca.crt
```

Не изменяйте `/etc/containerd/config.toml`, каталог `_default` или настройки других
registry. Не используйте `skip_verify` и не перезапускайте containerd. Проверить
отсутствие рестарта до и после установки:

```bash
systemctl show containerd -p MainPID -p ActiveEnterTimestamp
```

Проверка image через CRI:

```bash
sudo crictl pull \
  mirror.ip-10-28-32-189.shturval.link/kube-blackbox@sha256:5b7a52c94e04dbd65425db71da428f6a1175a10cf3b4c7596e204211e1c6d557
```

На infra-узлах `10.28.32.131` и `10.28.32.139` pull проверен успешно. PID и время
старта containerd не изменились.

## Helm repository и установка

В Shturval используется существующая запись `mirror-stp`:

```text
https://mirror.ip-10-28-32-189.shturval.link/repository/shturval_helm/
```

URL `/repository/kube-blackbox/` нельзя указывать как Helm repository: это Docker
repository без `index.yaml`. Credentials не должны храниться в Git.

Для новой установки:

1. Создайте namespace `blackbox`.
2. Откройте **Сервисы и репозитории** → **Доступные чарты** → `mirror-stp`.
3. Выберите `kube-blackbox` версии `0.1.3`.
4. Укажите namespace `blackbox`, режим **Авто** и значения из
   `deploy/shturval-values.yaml`.

Chart планирует Pod только на узлы с labels:

```yaml
nodeSelector:
  kubernetes.io/arch: amd64
  node-role.kubernetes.io/infra: ""
```

Для совершенно нового release очистите `persistence.existingClaim`: chart создаст
PVC после выбора infra-узла.

## Особенность существующего release demo214

Первоначальный local-path PVC был привязан к обычному worker. Он сохранён. Для
переноса приложения на infra создан новый PVC:

```text
deploy/shturval-infra-pvc.yaml
```

Текущий экземпляр использует:

```yaml
persistence:
  existingClaim: kube-blackbox-lq7q2-data-infra
```

Старые PVC не удаляются автоматически.

## Проверка статуса

```bash
KUBECONFIG=/absolute/path/to/demo214.conf

kubectl --kubeconfig "$KUBECONFIG" get ns blackbox

kubectl --kubeconfig "$KUBECONFIG" -n blackbox \
  get deployment,pod,pvc -o wide

kubectl --kubeconfig "$KUBECONFIG" -n blackbox \
  rollout status deployment/kube-blackbox-lq7q2 --timeout=60s

kubectl --kubeconfig "$KUBECONFIG" -n blackbox \
  logs deployment/kube-blackbox-lq7q2 --tail=100
```

Ожидается: Deployment `1/1`, Pod `Running` с `RESTARTS 0` на infra-узле, PVC
`Bound`, а в логах есть `initial Kubernetes snapshots persisted`.

## Функциональная проверка

Контейнер построен `FROM scratch`, поэтому shell в нём отсутствует. Для чтения
evidence вызывайте сам бинарник.

Версия и timeline:

```bash
kubectl --kubeconfig "$KUBECONFIG" -n blackbox exec \
  deployment/kube-blackbox-lq7q2 -- /kube-blackbox version

kubectl --kubeconfig "$KUBECONFIG" -n blackbox exec \
  deployment/kube-blackbox-lq7q2 -- \
  /kube-blackbox timeline --data-dir=/var/lib/kube-blackbox \
  --namespace=blackbox --limit=20
```

Структурный diff сохранённых версий Pod:

```bash
kubectl --kubeconfig "$KUBECONFIG" -n blackbox exec \
  deployment/kube-blackbox-lq7q2 -- \
  /kube-blackbox diff --data-dir=/var/lib/kube-blackbox \
  --namespace=blackbox --kind=Pod --limit=500
```

Проверка редактирования ConfigMap:

```bash
kubectl --kubeconfig "$KUBECONFIG" -n blackbox exec \
  deployment/kube-blackbox-lq7q2 -- \
  /kube-blackbox timeline --data-dir=/var/lib/kube-blackbox \
  --kind=ConfigMap --output=records --limit=20
```

У записей ConfigMap отсутствуют `object.data` и `object.binaryData`, а поле
`object.redaction` равно `configmap payload removed`. Запрос с `--kind=Secret`
должен вернуть пустой массив.

Для проверки главного MVP-сценария нужен отдельный тестовый namespace: изменить
Deployment/NetworkPolicy, дождаться readiness failure, удалить затронутый Pod и
после удаления запросить timeline по его UID. Такой тест изменяет тестовые workloads
и должен запускаться только с отдельным разрешением владельца кластера.

## Проверка опубликованного выпуска без Kubernetes

```bash
make release-verify
```

Команда проверяет image digest, платформу `linux/amd64`, SHA-256 chart `0.1.3`,
Helm lint и render.

Ручная проверка Helm repository:

```bash
helm repo add blackbox \
  https://mirror.ip-10-28-32-189.shturval.link/repository/shturval_helm/ \
  --insecure-skip-tls-verify
helm repo update
helm search repo blackbox/kube-blackbox --versions
helm show values blackbox/kube-blackbox --version 0.1.3
```

## Диагностика

| Симптом | Что проверить |
|---|---|
| `x509: certificate signed by unknown authority` | `ca.crt` в точном каталоге hostname Nexus и fingerprint |
| `ImagePullBackOff` | image digest, события Pod и `crictl pull` на infra |
| Pod `Pending` | selectors, taints и node affinity local-path PVC |
| PVC `Pending` | default StorageClass и `WaitForFirstConsumer` |
| `forbidden` в логах | ClusterRole/Binding с `get`, `list`, `watch` |
| `permission denied` в data directory | поддержку `fsGroup: 65532` выбранным CSI |
