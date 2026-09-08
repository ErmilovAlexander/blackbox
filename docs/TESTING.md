# Тестирование

## Локальная проверка одним вызовом

```bash
make verify
```

Команда должна завершиться без вывода ошибок и создаёт `bin/kube-blackbox`. Она включает:

1. `gofmt` check;
2. `go vet ./...`;
3. `go test -race ./...`;
4. production-like static build с `CGO_ENABLED=0`;
5. render `deploy/` через встроенный в kubectl Kustomize.

## Что покрывают unit-тесты

- `internal/model`: обязательные поля schema и допустимые enum values;
- `internal/redact`: удаление Secret payload, ConfigMap opt-in, plaintext env, managed fields и рискованных annotations;
- `internal/store/jsonl`: append/query, фильтры, read-only режим и проверка границ store/segment;
- `internal/collector`: initial LIST выдаёт `SNAPSHOT`, новый watch object выдаёт `ADD`;
- `internal/timeline`: хронологическая сортировка и сохранение evidence record ID.

Отдельный запуск пакета:

```bash
go test -race -count=1 ./internal/store/jsonl
go test -race -count=1 ./internal/collector
```

Проверка всего проекта без race detector, удобная для кросс-сборки:

```bash
make test-short
```

## Проверка бинарника и container image

```bash
make build VERSION=test
./bin/kube-blackbox version

make image IMAGE=kube-blackbox:test VERSION=test
docker run --rm kube-blackbox:test version
```

Обе version-команды должны вывести `test`.

## Локальный integration test через kubeconfig

Запустите recorder:

```bash
./bin/kube-blackbox recorder \
  --kubeconfig=/absolute/path/to/kubeconfig \
  --cluster=integration \
  --data-dir=./data \
  --retention=1h \
  --max-store-bytes=134217728 \
  --max-segment-bytes=8388608
```

После log-сообщения об initial snapshots в другом терминале создайте и удалите объект:

```bash
kubectl create namespace kbb-manual-test
kubectl -n kbb-manual-test create configmap probe --from-literal=password=must-not-be-stored
kubectl -n kbb-manual-test label configmap probe phase=updated
kubectl -n kbb-manual-test delete configmap probe
```

Проверьте evidence после удаления:

```bash
./bin/kube-blackbox timeline \
  --data-dir=./data \
  --namespace=kbb-manual-test \
  --kind=ConfigMap \
  --name=probe \
  --output=records
```

Ожидаются `ADD`, `UPDATE`, `DELETE`; поля `object.data` и `previous.data`, а также строка `must-not-be-stored`, должны отсутствовать.

После теста:

```bash
kubectl delete namespace kbb-manual-test
```

## Kubernetes smoke-тест

Сначала установите recorder по [KUBERNETES.md](KUBERNETES.md), затем:

```bash
./hack/kubernetes-smoke-test.sh
```

Переопределяемые параметры:

```bash
KBB_NAMESPACE=custom-namespace \
KBB_DEPLOYMENT=kube-blackbox \
KBB_TEST_NAMESPACE=kbb-smoke-fixed \
KUBECTL=kubectl \
./hack/kubernetes-smoke-test.sh
```

Успешный результат:

```text
PASS: RBAC, watch delivery, persistence, deletion history, and ConfigMap redaction are working.
```

Smoke-тест намеренно использует ConfigMap, а не внешний workload image: он работает в изолированном кластере и проверяет весь основной путь M0 от Kubernetes watch до evidence после удаления.

## Что ещё не покрыто автоматически

- длительная нагрузка и измерение RAM/IO на большом кластере;
- crash consistency при принудительном завершении во время записи;
- поведение конкретных CSI drivers и encrypted volume;
- rollover на многогигабайтном store и retention в течение суток;
- upgrade/rollback с реальным RWO volume;
- полный сценарий Deployment + NetworkPolicy + readiness failure из Definition of MVP success.

Эти проверки нужны перед production rollout. Текущий smoke подтверждает корректность M0-механизма, но не заменяет soak/load-тест.
