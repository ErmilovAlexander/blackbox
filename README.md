# Kube Blackbox

Always-on, offline-first flight recorder для Kubernetes. Сервис делает начальный снимок и затем записывает изменения Kubernetes-объектов в ограниченное по времени и размеру локальное хранилище. После инцидента историю можно запросить, даже если исходный объект уже удалён, а Kubernetes Event исчез.

Проект не изменяет ресурсы кластера, не читает Secrets, не требует внешней БД, SaaS или LLM.

## Что уже работает

- cluster-wide `list/watch` для Pod, Service, ConfigMap, Node, PVC, PV, Event, Deployment, ReplicaSet, StatefulSet, DaemonSet, EndpointSlice и NetworkPolicy;
- начальный LIST маркируется как `SNAPSHOT`, последующие события — `ADD`, `UPDATE`, `DELETE`;
- единый формат записи `kbb.io/record/v1alpha1`;
- редактирование чувствительных полей до записи на диск;
- сегментированное append-only JSONL-хранилище с retention по времени и объёму;
- CLI-фильтры по времени, namespace, kind, name и UID;
- безопасное read-only чтение краткой timeline или полных canonical records;
- Deployment, PVC, ServiceAccount и минимальный read-only ClusterRole;
- unit/race-тесты, CI и smoke-тест для реального Kubernetes-кластера.

Точная граница между реализованным кодом и целевой архитектурой описана в [docs/IMPLEMENTATION.md](docs/IMPLEMENTATION.md).

## Быстрый локальный запуск

Требуется Go 1.26+ и доступный kubeconfig.

```bash
make verify

./bin/kube-blackbox recorder \
  --kubeconfig=/absolute/path/to/kubeconfig \
  --cluster=development \
  --data-dir=./data \
  --retention=24h
```

В другом терминале:

```bash
./bin/kube-blackbox timeline \
  --data-dir=./data \
  --namespace=default \
  --from=2026-09-08T09:00:00Z \
  --to=2026-09-08T10:00:00Z
```

Полные очищенные записи вместо сокращённой timeline:

```bash
./bin/kube-blackbox timeline --data-dir=./data --output=records --limit=100
```

## Сборка контейнера и установка

```bash
make image IMAGE=kube-blackbox:dev VERSION=dev

# Для kind:
kind load docker-image kube-blackbox:dev

kubectl apply -k deploy
kubectl -n kube-blackbox rollout status deployment/kube-blackbox --timeout=120s
make smoke
```

Для удалённого кластера образ сначала нужно отправить во внутренний registry, затем заменить image у Deployment. Полный порядок действий, настройка PVC и диагностика: [docs/KUBERNETES.md](docs/KUBERNETES.md).

## Основные команды

```text
make build       собрать bin/kube-blackbox
make test        запустить тесты с race detector
make verify      format-check + vet + test + build + render manifests
make image       собрать OCI image (IMAGE=... VERSION=...)
make install     выполнить kubectl apply -k deploy
make smoke       выполнить Kubernetes smoke-тест
make vendor      подготовить зависимости для air-gapped сборки
```

Все варианты тестирования перечислены в [docs/TESTING.md](docs/TESTING.md). Архитектурные ограничения и будущие этапы находятся в [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) и [docs/MVP.md](docs/MVP.md).

## Air-gapped сборка

В подключённой контролируемой среде:

```bash
make vendor
```

Перенесите репозиторий вместе с `vendor/` и заранее загруженным базовым образом `golang:1.26` в изолированный контур. После этого:

```bash
go build -mod=vendor -trimpath -o bin/kube-blackbox ./cmd/kube-blackbox
docker build --network=none -t registry.internal/kube-blackbox:v0.1.0 .
```
