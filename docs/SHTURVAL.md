# Развёртывание в Shturval 2.14 без kubectl

Эта инструкция предназначена для management-кластера стенда
`shturval.demo214.ip-10-28-32-101.shturval.link`. Все операции выполняются через
графический интерфейс Shturval. Менять kubeconfig, запускать `kubectl`, подключаться
к control plane `10.28.32.129` или изменять конфигурацию Kubernetes/узлов не требуется.

## Готовые артефакты

| Артефакт | Значение |
|---|---|
| OCI image | `mirror.ip-10-28-32-189.shturval.link/kube-blackbox:4faa2b2` |
| Image digest | `sha256:0a742b83f0bfca6db4da14707020c40ecea9db838d6d8e951a1d6ca2e0836575` |
| Архитектура | `linux/amd64` (`x86_64`) |
| OCI Helm repository | `oci://mirror.ip-10-28-32-189.shturval.link/helm` |
| Chart | `kube-blackbox` |
| Chart version | `0.1.0` |
| Chart digest | `sha256:d60f1dc4570c3fca81d0aa8ae87bb320ca23a679b568af6e430a48d961172f36` |
| Целевой namespace | `kube-blackbox` |

Chart использует image по digest, поэтому повторная публикация того же тега не
изменит установленный бинарник.

## Что создаёт chart

- один `Deployment` со стратегией `Recreate`;
- `PersistentVolumeClaim` размером 5 GiB;
- отдельный `ServiceAccount`;
- read-only `ClusterRole` и `ClusterRoleBinding` только с `get`, `list`, `watch`;
- ограничение планирования `kubernetes.io/arch: amd64`.

Chart не создаёт Secret с доступом к Nexus и не содержит логин/пароль. PVC помечен
`helm.sh/resource-policy: keep`, чтобы удаление Helm-релиза не удалило evidence.

Recorder работает cluster-wide: он читает объекты во всех namespace, а также Nodes
и PV. Поэтому учётная запись, выполняющая установку через Shturval, должна иметь
право на создание ClusterRole и ClusterRoleBinding. Namespace-only установка не
сможет выполнить назначение продукта.

## 1. Создать namespace

1. Откройте [стенд Shturval](https://shturval.demo214.ip-10-28-32-101.shturval.link/).
2. В левом меню выберите management-кластер.
3. Создайте namespace `kube-blackbox` штатной формой интерфейса.

Namespace нужно создать до установки chart, потому что в нём сначала создаётся
секрет доступа к registry.

## 2. Создать ImagePullSecret через интерфейс

Локально подготовьте Docker config. Команды ниже не обращаются к Kubernetes и не
сохраняют пароль в истории shell:

```bash
export REGISTRY_HOST=mirror.ip-10-28-32-189.shturval.link
read -r "REGISTRY_USER?Nexus user: "
read -rs "REGISTRY_PASSWORD?Nexus password: "; printf '\n'
export REGISTRY_USER REGISTRY_PASSWORD

jq -n \
  --arg host "${REGISTRY_HOST}" \
  --arg user "${REGISTRY_USER}" \
  --arg password "${REGISTRY_PASSWORD}" \
  --arg auth "$(printf '%s' "${REGISTRY_USER}:${REGISTRY_PASSWORD}" | base64 | tr -d '\n')" \
  '{auths: {($host): {username: $user, password: $password, auth: $auth}}}' \
  > /tmp/kube-blackbox-dockerconfig.json

unset REGISTRY_PASSWORD
```

В Shturval выберите management-кластер → namespace `kube-blackbox` →
**Хранилище** → **Secrets** и создайте Secret:

- имя: `kube-blackbox-registry`;
- тип: `dockerconfigjson`;
- ключ: `.dockerconfigjson`;
- значение: содержимое `/tmp/kube-blackbox-dockerconfig.json`.

После загрузки удалите временный локальный файл. Не добавляйте его в Git.

## 3. Подключить OCI Helm repository

В management-кластере откройте **Сервисы и репозитории** → **Репозитории** →
**+ Добавить репозиторий** и задайте:

- название: `blackbox`;
- URL: `oci://mirror.ip-10-28-32-189.shturval.link/helm`;
- логин и пароль: учётная запись Nexus;
- проверка сертификата: включена, если Shturval принимает сертификат Nexus.

На 8 сентября 2026 года Nexus отдаёт self-signed сертификат без SAN. Строгие TLS
клиенты могут отклонить его. Предпочтительное исправление — корректный сертификат
на стороне Nexus. Если это невозможно на тестовом стенде, Shturval позволяет
отключить проверку сертификата только для этого подключения репозитория; перед этим
сверьте SHA-256 fingerprint:

```text
2F:85:07:35:AA:0A:DD:1D:8C:DD:B4:98:E4:EC:DF:20:5C:E1:6D:98:41:85:98:74:3D:D3:60:8A:64:32:CE:73
```

Отключение проверки для Helm repository не исправляет TLS при pull контейнерного
image. Worker-узлы должны уже уметь получать образы с этого внутреннего registry.
Изменять их runtime в рамках этой установки не нужно и не следует.

## 4. Установить chart

1. Откройте **Сервисы и репозитории** → **Доступные чарты** → вкладку `blackbox`.
2. Для OCI repository вручную введите chart `kube-blackbox`.
3. Нажмите **Проверить версию**, выберите `0.1.0`, затем **Проверить чарт**.
4. На странице установки укажите:
   - название экземпляра: `kube-blackbox`;
   - namespace: существующий `kube-blackbox`;
   - режим управления: **Авто**.
5. В **Спецификации сервиса** оставьте defaults или укажите только отличия:

```yaml
clusterName: management

imagePullSecrets:
  - name: kube-blackbox-registry

persistence:
  storageClass: ""
  size: 5Gi
```

Готовый вариант для вставки также хранится в
[`deploy/shturval-values.yaml`](../deploy/shturval-values.yaml); в нём нет
учётных данных Nexus.

Пустой `storageClass` означает default StorageClass. Если на стенде default-класса
нет, выберите существующий класс в Shturval и укажите его точное имя.

## 5. Проверить запуск в интерфейсе

1. **Сервисы и репозитории** → **Установленные сервисы** → `kube-blackbox`:
   ожидаемый статус — `Healthy`.
2. Namespace `kube-blackbox` → **Нагрузки** → **Deployments**:
   ожидается одна доступная реплика `kube-blackbox`.
3. На странице Pod проверьте **События**, **Volumes** и **Логи**.
4. Успешный initial LIST подтверждают сообщения:

```text
watch started
initial Kubernetes snapshots persisted
```

В логах не должно быть `forbidden`, `permission denied`, `ImagePullBackOff` или
`ErrImagePull`.

## Диагностика без kubectl

| Симптом в Shturval | Что проверить |
|---|---|
| `ImagePullBackOff` / `ErrImagePull` | имя Secret, registry hostname, права пользователя Nexus, события Pod |
| ошибка `x509` при pull image | уже существующее доверие runtime к Nexus; ImagePullSecret TLS не исправляет |
| PVC остаётся `Pending` | наличие default StorageClass или значение `persistence.storageClass` |
| `forbidden` в логах | разрешена ли установщику chart регистрация read-only ClusterRole/Binding |
| `permission denied` для data directory | поддерживает ли выбранный CSI `fsGroup: 65532` |
| сервис не появляется в каталоге | для OCI ввести имя chart вручную и выполнить обе проверки |

Если pull image завершается ошибкой `x509`, а менять Kubernetes и runtime узлов
запрещено, это внешний блокер: сертификат должен быть исправлен на Nexus либо
registry уже должен быть разрешён штатной конфигурацией стенда.

## Локальная проверка chart

Эти команды работают только с файлами репозитория и не подключаются к кластеру:

```bash
make chart-lint
helm template kube-blackbox charts/kube-blackbox \
  --namespace kube-blackbox > /tmp/kube-blackbox-rendered.yaml
```

Официальные страницы Shturval 2.14:

- [подключение своего Helm repository](https://docs.demo214.ip-10-28-32-101.shturval.link/ru2/cluster-admin/services/custom-repo/);
- [установка сервиса из chart через GUI](https://docs.demo214.ip-10-28-32-101.shturval.link/ru2/cluster-admin/services/deploy-service/);
- [ImagePullSecrets через GUI](https://docs.demo214.ip-10-28-32-101.shturval.link/ru2/cluster-admin/services/private-registry/).
