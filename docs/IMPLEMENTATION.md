# Разбор реализации M0

## Фактический поток данных

```text
Kubernetes API
  -> dynamic shared informers (LIST + WATCH)
  -> canonical Record
  -> redaction
  -> Store.Append
  -> segment-*.jsonl on PVC

segment-*.jsonl
  -> Store.Query
  -> timeline, structured diff или полные canonical records
  -> stdout
```

В текущем M0 нет отдельной очереди `record bus`, rule engine, correlation graph или HTTP API. Эти блоки на общей архитектурной схеме — целевая архитектура следующих этапов, а не уже работающие процессы.

## Модули кода

| Путь | Реализованная ответственность |
| --- | --- |
| `cmd/kube-blackbox/main.go` | CLI `recorder`, `timeline`, `diff`, `version`; конфигурация клиента Kubernetes; graceful shutdown |
| `internal/collector/kubernetes.go` | 13 cluster-wide informer-ов, классификация `SNAPSHOT/ADD/UPDATE/DELETE`, сбор metadata и Event reason/message |
| `internal/model/record.go` | Версионированная canonical schema и проверка обязательных полей/enums |
| `internal/redact/redact.go` | Удаление `managedFields`, Secret/ConfigMap payload, plaintext env и рискованных annotations |
| `internal/diff/diff.go` | Детерминированный structural diff с RFC 6901 paths и операциями ADD/REMOVE/REPLACE |
| `internal/store/store.go` | Интерфейс, отделяющий collector/timeline от формата хранилища |
| `internal/store/jsonl/store.go` | Append-only сегменты, rotation, time/size retention, линейный query, read-only open |
| `internal/timeline/timeline.go` | Хронологическая проекция canonical records в компактные entries |
| `deploy/` | Namespace, ServiceAccount, read-only RBAC, PVC, single-replica Deployment и Kustomize entrypoint |
| `hack/kubernetes-smoke-test.sh` | Проверка реального кластера без загрузки тестового workload image |

## Что происходит при запуске recorder

1. CLI читает flags и создаёт in-cluster client (или client из явно переданного kubeconfig).
2. JSONL store создаёт writable segment в `--data-dir`.
3. Для каждого поддержанного GroupVersionResource запускается informer.
4. Объекты из первоначального LIST сохраняются как `SNAPSHOT`.
5. Recorder ждёт синхронизации всех informer-ов и пишет об этом отдельный log event.
6. Последующие watch events сохраняются как `ADD`, `UPDATE` и `DELETE`.
7. Перед записью объект очищается. Для UPDATE сохраняются `object` и `previous`; для DELETE — `previous`.
8. Каждая строка немедленно flush-ится. При достижении порога начинается новый segment, старые segments удаляются по retention.
9. SIGTERM/SIGINT останавливает informer-ы и закрывает активный segment.

## Гарантии безопасности M0

- В RBAC отсутствует ресурс `secrets`, поэтому Secret payload нельзя получить из API.
- Даже если функция redaction отдельно получит Secret JSON, поля `data` и `stringData` будут удалены.
- ConfigMap watch включён, но `data`/`binaryData` удаляются по умолчанию. Сохранение payload возможно только явным `--include-configmaps`.
- Plaintext `env[].value` заменяется на `<redacted>`; ссылки `valueFrom` сохраняются как доказательство связи без значения секрета.
- Core имеет только `get/list/watch`; `pods/log`, `pods/exec` и mutation verbs отсутствуют.
- Контейнер работает как UID/GID 65532, без capabilities и privilege escalation, с read-only root filesystem.

Данные всё равно содержат имена объектов, labels, specs и Event messages. PVC должен считаться чувствительным evidence store и шифроваться средствами storage backend.

## Ограничения и технический долг

- Один writer и одна replica — сознательное ограничение JSONL M0. Leader election и HA пока нет.
- Informer хранит текущее состояние объектов в памяти; расход RAM зависит от размера кластера.
- Каждое UPDATE хранит полный current/previous JSON, а не компактный diff.
- Query линейно сканирует все retained segments; индексов пока нет.
- Массивы в structured diff сравниваются как атомарные значения. List-aware стратегии появятся только для полей с надёжным ключом элемента.
- `observedAt` — локальное UTC-время recorder-а, поэтому узлы кластера должны иметь корректную синхронизацию времени.
- Time retention опирается на mtime сегмента. Активный сегмент удаляется только после rotation.
- Если один из обязательных API resources недоступен или RBAC неполон, все initial snapshots не перейдут в состояние synced; причина будет видна в logs reflector-а.
- Локальный API, `.kbb` export, correlation graph, deterministic rules и node agent ещё не реализованы.

## Почему Kubernetes Deployment устроен именно так

- Отдельный namespace уменьшает связность с `kube-system`.
- `ClusterRole` нужен потому, что recorder наблюдает cluster-wide и читает cluster-scoped Node/PV.
- PVC с `ReadWriteOnce` и стратегия `Recreate` обеспечивают единственного writer-а и не запускают два Pod на одном store во время rollout.
- `fsGroup: 65532` даёт non-root процессу доступ к тому PVC, где CSI driver поддерживает стандартную fsGroup семантику.
- Service не создаётся: M0 не открывает сетевой порт. Timeline запускается через тот же бинарник и читает PVC внутри Pod.

## Соответствие исходной ARCHITECTURE.md

| Архитектурный блок | Статус |
| --- | --- |
| Kubernetes collector | Реализован |
| Normalize/redact | Реализован как canonical mapping + redaction |
| Canonical record v1alpha1 | Реализован |
| Segmented local store + retention | Реализован |
| Timeline query | Реализован |
| Structured object diff | Реализован |
| Record bus | Пока прямой вызов `Store.Append` |
| Rule engine | Не реализован |
| Correlation graph | Не реализован |
| Local API | Не реализован |
| `.kbb` export | Не реализован |
| Node agent | Не реализован |
| External analyzer contract | Только архитектурное решение, кода пока нет |

## Проверка на demo214

11 сентября 2026 года image `4faa2b2` и chart `0.1.3` были проверены в management-
кластере `demo214`:

- Deployment и Pod перешли в `Ready 1/1` на infra-узле;
- все 13 informer-ов синхронизировались;
- JSONL содержал 784 `SNAPSHOT`, 30 `UPDATE` и 1 `DELETE`;
- размер активного segment вырос с 3 285 948 до 3 341 092 байт за 10 секунд;
- все проверенные записи имели schema `kbb.io/record/v1alpha1`;
- timeline вернул сохранённые Kubernetes Event с `reason` и `summary`;
- diff показал переход Pod из `Pending` в `Running` по RFC 6901 paths;
- удалённый ReplicaSet отсутствовал в Kubernetes, но его `DELETE` record с
  `previous` оставался в локальном store;
- ConfigMap payload отсутствовал, а запрос `kind=Secret` вернул пустой массив.

Это подтверждает M0 и реализованную часть M1. Полный acceptance scenario из
`docs/MVP.md` с изменением NetworkPolicy пока не выполнялся, поскольку он требует
создания и изменения тестовых workloads.
