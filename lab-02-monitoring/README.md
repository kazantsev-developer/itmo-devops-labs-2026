# Лабораторная работа №2: Мониторинг сервиса в Kubernetes (Метрики, Логи, Трейсы, Алерты)

- **Язык сервиса:** Go 1.26.3
- **Стенд:** Встроенный Kubernetes-кластер OrbStack
- **Инструменты развертывания:** Helm v4

---

## Введение и цель работы

Что тут вообще происходит? На пальцах...
Если в первой лабораторной работе целью было запереть процесс в изолированную цифровую клетку с помощью механизмов ядра Linux (`namespaces`, `cgroups`), то в этой работе задача обратная - развернуть вокруг изолированного сервиса полную инфраструктуру наблюдаемости (`Observability`).

Необходимо доказать, что распределенное приложение в облачной среде (в данном случае в кластере Kubernetes) нельзя эксплуатировать вслепую. Я разверну полноценный стек мониторинга, который позволит контролировать состояние сервиса на всех уровнях:

1. **Метрики** `Prometheus + Grafana` - чтобы видеть общее здоровье и скорость работы (пульс системы).
2. **Логи** `Loki + Grafana Alloy` - чтобы читать текстовый вывод приложения и понимать, что происходит в конкретный момент времени.
3. **Трассировка** `OpenTelemetry + Jaeger` - чтобы рентгеном просвечивать путь одного запроса сквозь функции кода и видеть, где именно уходит время.
4. **Оповещения** `Alertmanager + Karma` - чтобы система сама сообщала о критических авариях до того, как о них узнает пользователь.

Всю инфраструктуру я буду разворачивать исключительно внутри `Kubernetes кластера` с помощью пакетного менеджера `Helm`. Моя цель - связать все эти компоненты в единую экосистему, где по одной строке лога можно будет мгновенно открыть соответствующий трейс в `Jaeger`.

---

## Часть 0 - Создание подопытного сервиса на Go

Для генерации телеметрии я разработал HTTP-сервис, содержащий необходимый набор эндпоинтов для симуляции нештатных ситуаций:

- `GET /health` — проверка живучести подов.
- `GET /fail` — генерация ошибок 500 (Internal Server Error) для проверки алертов.
- `GET /slow` — задержка ответа на 1–3 секунды для генерации Latency.
- `GET /load` — параллельные запросы для создания RPS-нагрузки.

Внутрь приложения я внедрил:

1. Клиент Prometheus (`/metrics`) для сбора RED-метрик.
2. JSON-логирование (пакет `slog`) с автоматическим пробросом `trace_id`.
3. SDK OpenTelemetry для генерации gRPC/HTTP-трейсов.

---

## Часть 1 — Метрики (Prometheus + Grafana)

### Шаг 1.1. Добавление Helm-репозиториев и развертывание kube-prometheus-stack

Прежде чем разворачивать стек мониторинга, я убеждаюсь, что активный кластер — встроенный Kubernetes OrbStack, а не забытый kind-кластер от предыдущей лабораторной.

Выполняю две команды: `kubectl cluster-info` и `kubectl config current-context`. Первая подтверждает, что control plane и CoreDNS работают. Вторая — что все будущие команды `kubectl` и `helm` будут применяться именно к OrbStack.

![](img/01_kubectl.png)

Далее я подключаю официальные Helm-репозитории — `prometheus-community` для чарта `kube-prometheus-stack` и `grafana` для чартов Loki и Alloy (они понадобятся в следующих частях). После этого обновляю локальный индекс чартов командой `helm repo update`.

Развёртывание стека мониторинга я выношу в отдельный namespace `monitoring`, чтобы поды Prometheus, Grafana и Alertmanager не смешивались с прикладными сервисами и системными компонентами кластера. Namespace создаётся командой `kubectl create namespace monitoring`.

Сам стек устанавливается одной командой `helm install kube-prom prometheus-community/kube-prometheus-stack -n monitoring`. Чарт `kube-prometheus-stack` разворачивает Prometheus, Grafana, Alertmanager, kube-state-metrics, node-exporter и Prometheus Operator с полным набором CRD (`ServiceMonitor`, `PrometheusRule`, `PodMonitor`). Это стандартный способ установки мониторинга в Kubernetes — вручную собирать все компоненты и связывать их конфигами не нужно.

Сразу после установки поды находятся в процессе инициализации — образы скачиваются, init-контейнеры выполняют подготовку:

![](img/02_helm_install.png)
![](img/03_pods_installing.png)
![](img/04_pods_running.png)

---

### Шаг 1.2. Конфигурация ServiceMonitor для обнаружения сервиса

Чтобы Prometheus узнал о существовании моего Go-сервиса и начал собирать с него данные, я подготовил два манифеста в папке `deploy/`:

1. **`deploy/api.yaml`** — создает Deployment на две реплики приложения и Service для балансировки входящего трафика. В параметры контейнеров я прокинул порт `8080`, `OTEL_SERVICE_NAME=api-service` и `OTEL_EXPORTER_OTLP_ENDPOINT` с адресом коллектора Jaeger (он будет развёрнут в Части 3). Благодаря встроенному Kubernetes в OrbStack, мне не пришлось вручную загружать образ `api-slim:1.0.0` в кластер — система увидела его автоматически сразу после сборки.
2. **`deploy/service-monitor.yaml`** — манифест ServiceMonitor (специальный Custom Resource от Prometheus-оператора). Он указывает системе: «ищи сервисы с меткой `app: api-server`, подключайся к порту `http-web` и забирай метрики с эндпоинта `/metrics` каждые 5 секунд». Лейбл `release: kube-prom` здесь обязателен — по нему оператор понимает, что этот монитор нужно активировать.

Применяю конфигурацию в кластере:

```bash
kubectl apply -f deploy/api.yaml
kubectl apply -f deploy/service-monitor.yaml
```

Проверяю, что обе реплики приложения успешно поднялись и перешли в статус `Running`:

```bash
kubectl get pods -n monitoring -l app=api-server
```

![](img/05_api_pods_running.png)

Чтобы убедиться, что Prometheus увидел новые цели, я пробросил его порт наружу командой `kubectl port-forward -n monitoring svc/kube-prom-kube-prometheus-prometheus 9090:9090` и зашел на страницу `Status -> Targets`. Мой ServiceMonitor успешно обнаружил оба пода, подключился к ним, и они оба горят зеленым статусом `UP`:

![](img/06_prometheus_targets.png)

---

### Шаг 1.3. Построение RED-дашборда в Grafana

Метрики успешно собираются в Prometheus. Чтобы визуализировать их по классическому RED-методу, я использую Grafana. Так как весь стек развернут через один Helm-чарт, Prometheus уже автоматически подключен к Grafana в качестве источника данных (`Data Source`).

![](img/07_grafana_datasources.png)

Я вытащил сгенерированный пароль администратора из секретов Kubernetes, пробросил порт Grafana наружу на локальный порт `3000` и создал пустой дашборд, в который добавил три панели со следующими PromQL-запросами:

1. **RPS — Интенсивность запросов (Rate):** Считает суммарную скорость входящего трафика в секунду по всем подам сервиса, исключая запросы самого Prometheus на эндпоинт `/metrics`.

![](img/08_grafana_rps_panel.png)

2. **Доля ошибок 5xx (%) (Errors):** Показывает процентное отношение запросов со статусом 5xx к общему числу входящих запросов. Позволяет мгновенно оценить объем брака в системе.

![](img/09_grafana_errors_panel.png)

Дополнительно в терминале я проверил сырой вывод метрик, чтобы убедиться, что счетчик пятисотых ошибок на эндпоинте `/metrics` действительно инкрементируется приложением при вызовах `/fail`:

```bash
curl -s http://localhost:8090/metrics | grep fail
```

![](img/10_metrics_fail_raw.png)

3. **Время ответа p95 (Duration):** Использует функцию `histogram_quantile` для расчета 95-го перцентиля на основе бакетов гистограммы, агрегируя данные между репликами. Этот график показывает время, в которое укладываются 95% всех пользователей, отсекая случайные единичные всплески.

![](img/11_grafana_p95_panel.png)

Все три панели объединены на одном общем дашборде. Пока нагрузка шла только на быстрые эндпоинты (`/load`, `/fail`), p95-задержка стабильно держалась на уровне около 4.7 миллисекунды.

![](img/12_grafana_red_dashboard.png)

---

### Шаг 1.4. Проверка реакций графиков под нагрузкой

Чтобы доказать работоспособность RED-дашборда, я провел серию стресс-тестов, поочередно запуская в терминале Bash-скрипты для имитации различных типов нагрузок:

1. **Тест RPS через `/load`:** Запустил цикл с запросами каждые `0.1s`. На графике RPS мгновенно отобразилась устойчивая полка в районе 10-11 запросов в секунду.
2. **Тест брака через `/fail`:** Параллельно запустил спам ручки `/fail` каждые `0.2s`. График ошибок резко подскочил и зафиксировался на уровне около 37% (успешные запросы от `/load` разбавили общую статистику).
3. **Тест задержек через `/slow`:** Остановил первые два скрипта и пустил трафик на ручку `/slow` каждые `1s`. Из-за пятиминутного временного окна в формуле `rate([5m])`, график p95 отреагировал с ожидаемой замеряемой задержкой: по мере вытеснения быстрых запросов медленными, кривая времени ответа устремилась вверх, наглядно показав скачок задержки с 4 миллисекунд до 4 секунд.

![](img/13_grafana_p95_under_slow.png)

**Итог этапа:** Метрики корректно генерируются приложением, успешно агрегируются Prometheus и мгновенно визуализируются в Grafana. Первая часть лабораторной работы полностью закрыта.

---

## Часть 2 - Логи (Loki + Grafana)

### Шаг 2.1. Развертывание хранилища Loki через Helm

Loki — хранилище логов. Сам он их ниоткуда не забирает: логи в него пушит отдельный агент-сборщик. Поэтому сначала я поднял Loki как приёмник.

Развернул в namespace `monitoring` в режиме `SingleBinary` — один под `loki-0` обслуживает и запись, и чтение. Параметры вынес в `deploy/values/loki.yaml`: `deploymentMode: SingleBinary`, `auth_enabled: false`, retention 168h, `schema: v13` с `from: 2024-01-01`, плюс `extraVolumes`/`extraVolumeMounts` с `emptyDir` в `/var/loki`.

Последний пункт — фикс после первой попытки: `loki-0` падал в `CrashLoopBackOff` с `mkdir /var/loki: read-only file system`. Удалил релиз и переустановил:

```bash
helm install loki grafana/loki -n monitoring -f deploy/values/loki.yaml
```

`STATUS: deployed` (chart 7.3.0, Loki 3.6.12). Поды:

```bash
kubectl get pods -n monitoring -l app.kubernetes.io/name=loki
```

Все три в `Running`: `loki-0` (2/2), `loki-canary` (1/1), `loki-gateway` (1/1).

![](img/14_loki_pod_running.png)

---

### Шаг 2.2. Настройка и запуск агента сбора логов Grafana Alloy

Loki — только приёмник. Чтобы логи подов реально доезжали до него, нужен агент-сборщик на каждой ноде кластера. Я использую Grafana Alloy — он читает stdout контейнеров из `/var/log/pods/`, добавляет Kubernetes-метки и пушит поток в Loki по HTTP.

Alloy разворачивается как DaemonSet — по одному поду на ноду. Все параметры вынес в `deploy/values/alloy.yaml`:

- `controller.type: daemonset`;
- `alloy.configMap` — River-конфиг Alloy;
- `alloy.mounts.varlog: true` — монтирует `/var/log` хоста, иначе Alloy не увидит логи подов;
- `rbac.create: true` — ServiceAccount с правом читать поды через API.

River-конфиг описывает цепочку из четырёх компонентов:

1. `discovery.kubernetes "pods"` — находит поды через API Kubernetes.
2. `discovery.relabel "api"` — оставляет только поды с меткой `app: api-server` и прокидывает в лог-стрим labels `namespace`, `pod`, `container`, `app`.
3. `loki.source.kubernetes "api"` — читает stdout/stderr контейнеров этих подов.
4. `loki.write "default"` — пишет всё в Loki через gateway по адресу `http://loki-gateway.monitoring.svc.cluster.local/loki/api/v1/push`.

Установил Alloy через Helm:

```bash
helm install alloy grafana/alloy -n monitoring -f deploy/values/alloy.yaml
```

`STATUS: deployed`. Проверил, что под поднялся:

```bash
kubectl get pods -n monitoring -l app.kubernetes.io/name=alloy
```

Под `alloy-wtxhh` перешёл в `2/2 Running` — оба контейнера (сам Alloy и config-reloader) живы, рестартов нет.

![](img/15_alloy_deployed.png)

Дальше проверил, что Alloy реально увидел поды нашего сервиса и открыл для них потоки логов:

```bash
kubectl logs -n monitoring -l app.kubernetes.io/name=alloy --tail=-1 | grep -E "Alloy is running|tailer running|opened log stream"
```

В выводе — пять строк: `Alloy is running` (процесс стартовал), два `tailer running` с target `monitoring/api-deployment-7fb96f8bf5-hcwwl:api` и `monitoring/api-deployment-7fb96f8bf5-krlw6:api` (оба пода api-service найдены), и два `opened log stream` с теми же target (для каждого пода открыт поток чтения). Ни одной строки `level=error` в логах нет.

![](img/16_alloy_logs.png)

Alloy готов: он находит поды api-service и читает их stdout.

---

### Шаг 2.3. Валидация логирования в Grafana Explore

Чтобы логи были видны в одном окне с метриками, я подключил Loki как ещё один datasource в Grafana. Пошёл в `Connections → Data sources → + Add new data source`, выбрал из списка Loki. В поле URL указал внутрикластерный адрес Loki gateway:

```
http://loki-gateway.monitoring.svc.cluster.local
```

Это тот же самый gateway, через который Alloy пишет логи (см. Шаг 2.2). Аутентификацию оставил как `No Authentication` — Loki у нас поднят с `auth_enabled: false`. После нажатия `Save & test` Grafana показала зелёное сообщение `Data source successfully connected.`.

![](img/17_loki_datasource_settings.png)

![](img/18_loki_datasource_connected.png)

Дальше открыл Explore (`Explore data` прямо со страницы datasource) и выполнил LogQL-запрос по меткам, которые проставляет Alloy:

```logql
{namespace="monitoring", app="api-server"}
```

Первый запуск вернул `No logs found` — причина оказалась в том, что api-service пишет лог только при входящем HTTP-запросе, а с момента остановки нагрузочных скриптов в Части 1 к сервису никто не обращался. Проверил это через API Loki (`/loki/api/v1/labels` и `/loki/api/v1/label/<name>/values`) — в хранилище действительно были только логи `loki-canary`. Тогда прогнал серию запросов к `/fail`:

```bash
for i in $(seq 1 20); do curl -s -o /dev/null http://localhost:8090/fail; sleep 0.5; done
```

После этого в Loki появились метки `app`, `container`, `instance`, `job`, `namespace`, а запрос в Explore вернул строки логов. На панели `Logs volume` видны синие столбики info и красные столбики error; легенда показывает `error Total: 20`, `info Total: 40`, `unknown Total: 3`. Общее количество строк — `63`, common labels содержат `app=api-server`, `container=api`, `instance=monitoring/api-deploym…`.

![](img/19_grafana_explore_logs.png)

В тех же строках видна и сама ошибка: `"level":"ERROR"`, `"msg": "Сгенерирована ошибка 500 по запросу /fail"`. Рядом — сопровождающие info-строки `"Входящий запрос"` с `"method":"GET"`, `"path":"/fail"` и `"Запрос успешно обработан"` со `"status":500`. В каждой строке присутствует `trace_id` — поле, которое понадобится в Части 3 для перехода из лога в трейс.

![](img/20_grafana_explore_error.png)

Метрики и логи теперь видны в одном окне: дашборд RED по метрикам и логи api-service через Loki datasource — в одной Grafana.

---

## Часть 3 - Трейсы (OpenTelemetry + Jaeger)

### Шаг 3.1. Развертывание Jaeger All-in-One в кластере

_Установка Jaeger и конфигурация сетевых портов (OTLP/gRPC и OTLP/HTTP) для приема трейсов от приложения._

### Шаг 3.2. Анализ распределенных трейсов для медленных и ошибочных запросов

_Анализ водопадных графиков в Jaeger UI. Проверка вложенного спана (slow-op) для /slow и фиксация статуса error (красный цвет) для /fail._

### Шаг 3.3. Настройка сквозной интеграции (Data Links) между Grafana и Jaeger

_Конфигурация Derived Fields в источнике данных Loki для автоматического превращения текстового trace_id из логов в кликабельную ссылку на трейс._

---

## Часть 4 - Алерты (Alertmanager + Karma)

### Шаг 4.1. Написание правил оповещения PrometheusRule

_Разработка и декларация трех критических алертов на PromQL (описание условий срабатывания, важности и инструкций для дежурного)._

### Шаг 4.2. Настройка маршрутизации в Alertmanager

_Конфигурация values для Alertmanager, определение получателей алертов и правил группировки._

### Шаг 4.3. Развертывание и интеграция дашборда Karma

_Установка Karma через Helm, подключение к API Alertmanager для визуального контроля за авариями._

### Шаг 4.4. Симуляция инцидентов и проверка состояния Firing

_Искусственное провоцирование сбоев, перевод разработанных алертов в активную фазу и фиксация их отображения в Karma UI._
