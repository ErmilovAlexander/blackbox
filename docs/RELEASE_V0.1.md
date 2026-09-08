# План первого полноценного выпуска v0.1

Цель v0.1 — не просто записывать Kubernetes events, а давать инженеру переносимый, проверяемый ответ на вопросы: какой объект изменился, какие поля изменились, с чем он был связан и какие детерминированные incident conditions наблюдались.

## Этап 1 — recorder foundation

Статус: реализован.

- canonical record v1alpha1;
- cluster-wide LIST/WATCH;
- redaction до persistence;
- bounded segmented JSONL store;
- timeline/records query;
- Kubernetes manifests, RBAC, PVC, CI и smoke harness.

## Этап 2 — change reconstruction

Статус: structured diff реализован; kubectl plugin ещё не реализован.

- RFC 6901 JSON Pointer paths;
- операции `ADD`, `REMOVE`, `REPLACE`;
- стабильный порядок output;
- lifecycle diff для ADD/DELETE;
- пропуск initial snapshots и no-op updates;
- CLI `kube-blackbox diff` с теми же фильтрами, что timeline.

Критерий: после изменения Deployment инженер получает точные paths изменённых полей без ручного сравнения двух JSON documents.

## Этап 3 — relation graph и correlation

Следующий этап реализации.

- индекс объектов по UID и `kind/namespace/name`;
- Deployment → ReplicaSet → Pod через ownerReferences;
- Service → EndpointSlice → Pod через service-name label и targetRef UID;
- Pod → Node через `spec.nodeName`;
- Pod → PVC → PV;
- Event → involvedObject UID;
- Pod → ConfigMap/Secret reference metadata без Secret payload;
- NetworkPolicy → selected Pods по label selector;
- CLI `related` с evidence record IDs и направлением graph edge.

Критерий: запрос для удалённого Pod восстанавливает его owners, Node, Events, storage и Service endpoint relations только из retained evidence.

## Этап 4 — deterministic incident rules

- CrashLoopBackOff/container restart spike;
- readiness collapse;
- Pending timeout;
- Node NotReady;
- исчезновение всех ready endpoints Service;
- изменение NetworkPolicy перед потерей endpoints;
- evidence IDs, confidence и machine-readable reason для каждого finding.

Критерий: rule не выдаёт finding без ссылок на конкретные canonical records и graph edges.

## Этап 5 — portable evidence

- versioned `.kbb` bundle;
- manifest с cluster, time range, schema versions и counts;
- JSONL records и findings;
- SHA-256 checksums;
- offline `inspect`, `timeline`, `diff`, `related` без Kubernetes API;
- защита от path traversal и повреждённых bundles.

Критерий: bundle, созданный в кластере, проверяется и анализируется на машине без доступа к этому кластеру.

## Этап 6 — operational release

- настоящий `kubectl blackbox` plugin, который выполняет запросы к recorder Pod;
- health/readiness contract и понятные startup failures;
- metrics только как optional local endpoint, без внешней зависимости;
- resource/load/retention tests;
- upgrade/rollback test на RWO PVC;
- multi-architecture image;
- immutable image digest и release checksums;
- полный smoke: Deployment rollout + NetworkPolicy + readiness failure + удалённый Pod.

## Definition of done v0.1

Выпуск готов, когда CI, Kubernetes smoke и offline bundle tests проходят, а сценарий из `docs/MVP.md` восстанавливает ordered timeline, structured diffs, related objects и rule findings после удаления проблемного Pod.
