# Хроники Гофера. Сезон 2: Под куполом Облака: Мониторинг сервиса в Kubernetes (Метрики, Логи, Трейсы, Алерты)

- **Язык сервиса:** Go 1.26.3
- **Стенд:** Встроенный Kubernetes-кластер OrbStack
- **Инструменты развертывания:** Helm v4

---

## Введение и цель работы. Сюжетная арка: Жизнь за стеклом

Что тут вообще происходит? На пальцах...

Если в первом сезоне моей практики главной целью было запереть Гофера в изолированную цифровую клетку с помощью суровых механизмов ядра Linux (`namespaces`, `cgroups`), то во втором задача разворачивается ровно на 180 градусов. Мой подопытный суслик пошел на повышение и переехал в полноценную облачную среду - `встроенный Kubernetes-кластер OrbStack`, где разворачивать всё я буду через пакетный менеджер `Helm v4`.

![](img/00_gopher.png)

Чтобы исправить это, я разверну полноценную инфраструктуру наблюдаемости `Observability` и докажу, что могу просветить Гофера рентгеном на всех уровнях:

1. Метрики `Prometheus + Grafana` - чтобы замерить пульс системы, видеть общее здоровье, объемы трафика и скорость работы моего суслика.
2. `Логи Loki + Grafana Alloy` - чтобы читать личный дневник приложения в структурированном JSON-формате и понимать, что конкретно пошло не так в данную секунду.
3. Трассировка `OpenTelemetry + Jaeger` - чтобы просветить кости каждого входящего запроса рентгеном и увидеть, в какой именно функции кода Гофер решил прикорнуть на пару секунд.
4. Оповещения `Alertmanager + Karma` - чтобы у меня в бункере орала ядерная сирена и сообщала о критических авариях до того, как о них узнает разъяренный пользователь.

Короче говоря, хочется не просто навалить в кластер гору бессвязного софта, а спаять все эти инструменты в один монолитный, бесшовно работающий комбайн. Я настрою сквозную интеграцию так, чтобы по одной строчке матерного` JSON-лога` в `Grafana` можно было в один клик провалиться в точный трехсекундный трейс `Jaeger`. Полная конспирация отменяется, шоу `За стеклом` официально объявляется открытым!

---

## Часть 0 - Создание подопытного сервиса на Go. Сюжетная арка: Прошивка для киборга

Перед отправкой под купол Kubernetes я превратил Гофера в идеального киборга для опытов. Обычный сервис на Go нем как рыба, поэтому я снабдил его специальными ручками, чтобы дистанционно устраивать ему пытки и проверять системы слежения:

- `GET /health` - проверка пульса. Показывает Kubernetes, что поды живы.
- `GET /fail` - выстрел в колено. Генерирует 500 ошибки для проверки алертов.
- `GET /slow` - сонная лощина. Засыпает на 1–3 секунды, имитируя дикую задержку (Latency).
- `GET /load` - режим берсерка. Спамит запросами сам в себя, взвинчивая RPS до небес.

Чтобы фиксировать эти мучения, я зашил в рантайм три шпионских чипа:

1. `Клиент Prometheus (/metrics)` - переводит каждое действие в сухие цифры RED-метрик.
2. `Пакет slog` - выдает жесткий JSON-лог, вживляя в каждую строчку уникальный паспорт (`trace_id`).
3. `SDK OpenTelemetry` - распределенная нервная система, шьющая OTLP-трейсы прямо наружу.

![](img/01_gopher.png)

---

## Часть 1 - Метрики (Prometheus + Grafana). Сюжетная арка: Палата №6

### Шаг 1.1. Замер пульса

Сначала проверяю, что я вообще там, где думаю. А то вдруг это старый `kind-кластер` из прошлой лабы, а не мой любимый OrbStack.

Дёргаю две команды:

```bash
kubectl cluster-info
kubectl config current-context
```

Первая говорит: мозги кластера на месте, DNS крутится. Вторая: да, это OrbStack, никуда не уехали

![](img/01_kubectl.png)

Теперь тащу нужные репозитории: `prometheus-community` и `grafana`. Потом `helm` `repo update`, чтобы список чартов был свежий, как хлеб из печки.

Дальше отвожу мониторингу отдельный закуток - `namespace monitoring`. Чтобы поды `Prometheus` и `Grafana` не толкались локтями с рабочими сервисами. `kubectl create namespace monitoring`.

И финальный босс одной командой:

```bash
helm install kube-prom prometheus-community/kube-prometheus-stack -n monitoring
```

Этот чарт как комбо в файтинге: сразу `Prometheus`, `Grafana`, `Alertmanager`, `kube-state-metrics`, `node-exporter` и целый выводок `CRD`. Руками такое собирать просто мазохизм...

Первые секунды поды вялые, образы качаются, init-контейнеры копошатся. Дам им минутку.

![](img/02_helm_install.png)
![](img/03_pods_installing.png)
![](img/04_pods_running.png)

![](img/1_1_gopher.png)

---

### Шаг 1.2. Цепляем датчики

Гофер лежит в палате, но врачи его не видят. Пора прицепить аппаратуру.

Первая бумажка это сам пациент. Две копии Гофера, чтобы если один загнётся, второй доработал смену. Плюс вахтёрша на входе, раскидывает посетителей между копиями, чтобы никто не толкался в одну дверь.

Вторая бумажка это направление на слежку. Кладу рядом с подопечным и пишу: `этот наш, следи за ним, снимай показания каждые 5 секунд`. Без одной волшебной печати бумажка для системы просто фантик.

Кидаю обе в регистратуру:

```bash
kubectl apply -f deploy/api.yaml
kubectl apply -f deploy/service-monitor.yaml
```

Проверяю, как пациент:

```bash
kubectl get pods -n monitoring -l app=api-server
```

![](img/05_api_pods_running.png)

Обе копии дышат, обе в строю.

Теперь открываю окно в ординаторскую:

```bash
kubectl port-forward -n monitoring svc/kube-prom-kube-prometheus-prometheus 9090:9090
```

И вижу, доктор Prometheus уже прицепил обоим по датчику. Оба зелёные, оба на связи. Пациент под наблюдением...

![](img/06_prometheus_targets.png)

![](img/1_2_gopher.png)

---

### Шаг 1.3. Выводим кардиограмму

Пациент весь в датчиках, но врачи пока ничего не видят. Данные капают в `Prometheus`, а толку ноль. Пора вывести всё на мониторы в ординаторской.

Захожу в `Grafana`. Она уже подцеплена к `Prometheus` как источник, это сделал сам чарт при установке. Намёк понял, работаем...

![](img/07_grafana_datasources.png)

Вытаскиваю пароль админа из секретов, пробрасываю порт 3000 наружу и открываю чистый лист. Дальше вешаю на него три прибора. `Классика RED`.

Первый прибор. `Пульс`.
Считает, сколько запросов прилетает в секунду. Если линия ровная, пациент дышит спокойно. Если шарашит вверх, значит, кто-то его дёргает без остановки.

![](img/08_grafana_rps_panel.png)

Второй прибор. `Осложнения`.
Показывает долю пятисотых ошибок от всех запросов. Другими словами, сколько раз пациент не справился и выдал брак. На скрине как раз видно, как я его разогнал до `32 процентов`.

![](img/09_grafana_errors_panel.png)

Дополнительно заглянул в сырые показания, чтобы убедиться, что счётчик реально тикает. Дёрнул метрики и увидел, что счётчик по ручке `/fail` уже накрутил `297 вызовов`:

```bash
curl -s http://localhost:8090/metrics | grep fail
```

![](img/10_metrics_fail_raw.png)

Третий прибор. `Скорость реакции`.
Тут хитрее. Это `p95`, то есть время, за которое отвечает 95 процентов всех обращений. Единичные тормоза отсекаются, берётся общая картина. Если пациент начинает тупить, линия ползёт вверх.

![](img/11_grafana_p95_panel.png)

Все три прибора висят на одном мониторе. Пока я дёргал только быстрые ручки, p95 держался где-то в районе `4.7 миллисекунды`. То есть пациент даже не вспотел :)

![](img/12_grafana_red_dashboard.png)

![](img/1_3_gopher_.png)

---

### Шаг 1.4. Ставим капельницу

Пациент лежит под мониторами, но пока дышит ровно. Скучно. Пора вкатить ему дозу и посмотреть, как задёргается.

Накрутил три разных препарата и по очереди вкатываю.

Доза первая. Лёгкая встряска через `/load`.
Гоняю запросы каждые `0.1 секунды`. Пульс на мониторе тут же выпрямился в полку на 10-11 ударах в секунду. Пациент бодрый, дышит ровно, ничего не предвещает беды.

Доза вторая. Отрава через `/fail`.
Параллельно спамлю ручку /fail каждые 0.2 секунды. График осложнений резко скакнул и залип на 37 процентах. Успешные запросы от первой дозы слегка разбавили картину, иначе было бы ещё жёстче.

Доза третья. Замедлитель через `/slow`.
Первые два шприца выдернул, пустил трафик на /slow раз в секунду. Тут самое интересное. Монитор времени реакции не дёрнулся сразу, потому что формула смотрит на пятиминутное окно. Но постепенно быстрые запросы вытеснились медленными, и линия поползла вверх. `С 4 миллисекунд до 4 секунд`. Пациент знатно затупил.

![](img/13_grafana_p95_under_slow.png)

**Итог сеанса.** Все три прибора отработали чётко: сервис выдал метрики, Prometheus их проглотил, Grafana нарисовала. Первая палата официально закрыта.

![](img/1_4_gopher_.png)

---

## Часть 2 - Логи (Loki + Grafana). Сюжетная арка: "Карта пациента"

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

Jaeger — система распределённой трассировки. Разворачиваю в режиме **all-in-one**: один под, в котором сразу collector, query и UI. Для локального кластера этого достаточно, данные трейсов хранятся в памяти пода.

Подключил Helm-репозиторий и установил чарт:

```bash
helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm repo update
helm install jaeger jaegertracing/jaeger -n monitoring -f deploy/values/jaeger.yaml
```

`deploy/values/jaeger.yaml` — ключевое:

- `provisionDataStore` — все `false`, внешние БД не поднимаем;
- `storage.type: memory` — трейсы в памяти;
- `allInOne.enabled: true`, `replicas: 1`;
- `agent`, `collector`, `query` — выключены, чтобы чарт не разворачивал лишние поды;
- `serviceMonitor.enabled: false`.

`STATUS: deployed`, Jaeger `2.21.0` (chart `4.14.0`). Под поднялся:

```bash
kubectl get pods -n monitoring -l app.kubernetes.io/name=jaeger
```

`jaeger-65b9785957-4vk5h  1/1  Running`.

![](img/21_jaeger_pod_running.png)

Jaeger all-in-one по умолчанию открывает OTLP-приёмники: `4317` (gRPC) и `4318` (HTTP). Проверил, что сервис `jaeger` в кластере слушает эти порты:

```bash
kubectl get svc -n monitoring | grep -i jaeger
```

В портах сервиса — `4317/TCP`, `4318/TCP`, `16686/TCP` (UI).

Оставалось направить api-service на реальный адрес Jaeger. В `deploy/api.yaml` был прописан несуществующий хост `jaeger-collector`, из-за чего экспортёр трейсов падал с `no such host`. Заменил адрес на внутрикластерный:

```yaml
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: "http://jaeger.monitoring.svc.cluster.local:4318"
```

Применил манифест:

```bash
kubectl apply -f deploy/api.yaml
```

Deployment обновился (`configured`), поды api-service пересоздались rolling update — оба в `Running 1/1`. Проверка, что в файле реально новый адрес:

```bash
grep -nE "OTEL|jaeger|4318" deploy/api.yaml
```

![](img/22_jaeger_service_ports.png)

### Шаг 3.2. Анализ распределенных трейсов для медленных и ошибочных запросов

На предыдущем шаге мы направили OTLP-экспортёр api-service на реальный сервис Jaeger. Оказалось, что до этого трейсы не улетали ещё по одной причине: `port-forward` к api-service отвалился после rolling restart подов, и наши curl'ы просто не доходили до сервиса. Перезапустил `port-forward`, дёрнул `/health` для проверки — в логах api-service появилась свежая строка `Входящий запрос` с `trace_id`, а в Jaeger UI в списке сервисов появился `api-service`. Трейсы поехали.

Сгенерировал свежие трейсы, чтобы было что смотреть:

```bash
for i in $(seq 1 3); do curl -s -o /dev/null http://localhost:8090/slow; done
for i in $(seq 1 3); do curl -s -o /dev/null http://localhost:8090/fail; done
```

В Jaeger UI выбрал `Service = api-service`, `Lookback = Last 1 hour` и нажал `Find Traces`. Jaeger нашёл 7 трейсов. Среди них сразу видно две группы:

- **`/slow`** — длительностью **3.0s / 3.0s / 2.0s**, у всех **`Spans = 2`**. Два спана — потому что помимо корневого `HTTP GET /slow` в обработчике создаётся вложенный `slow-op`.
- **`/fail`** — быстрые (десятки–сотни микросекунд), **`Spans = 1`**, и у всех в колонке **`Errors`** стоит красная метка `1`.
- Плюс один трейс `/health` с `Spans = 1` — от проверки, что сервис жив.

![](img/23_jaeger_traces_list.png)

Открыл один из трейсов `/slow`. В водопаде видно ровно то, что требуется: корневой спан `HTTP GET /slow` длится все 3 секунды, и внутри него — вложенный спан `slow-op` той же длительности. Это и есть ответ на вопрос «на каком шаге ушло время»: не в middleware, не в роутинге, а конкретно в медленной операции, обёрнутой в отдельный спан.

![](img/24_jaeger_trace_slow.png)

Открыл трейс `/fail`. Слева от спана `HTTP GET /fail` Jaeger показывает **красный маркер ошибки**. В раскрытой панели атрибутов спана видно:

- `error = true`
- `otel.status_code = ERROR`
- `otel.status_description = искусственный сбой`
- `otel.scope.name = api-tracer`

Это тот самый `span.SetStatus(codes.Error, "искусственный сбой")`, который вызывается в обработчике `/fail`. Jaeger распознал статус и подсветил спан красным.

![](img/25_jaeger_trace_fail.png)

### Шаг 3.3. Настройка сквозной интеграции (Data Links) между Grafana и Jaeger

Задание требует, чтобы от строки лога в Grafana можно было перейти к тому же трейсу в Jaeger по `trace_id`. В Grafana это делается через **Derived Fields** в настройках datasource Loki — механизм, который вытаскивает из лога поле регуляркой и делает его кликабельной ссылкой.

Пошёл в `Connections → Data sources → Loki`, прокрутил страницу до секции `Derived fields` и добавил новое поле:

- **Name:** `trace_id`
- **Type:** `Regex in log line`
- **Regex:** `"trace_id":"(\w+)"` — вытаскивает значение из JSON-лога
- **URL:** `http://localhost:16686/trace/${__value.raw}` — плейсхолдер `${__value.raw}` подставляется тем, что нашла регулярка; такой URL понимает Jaeger UI для открытия конкретного трейса
- **URL Label:** оставил пустым
- **Internal link:** выключен — ссылка ведёт во внешний сервис Jaeger, а не внутрь Grafana
- **Open in new tab:** включён

Сохранил через `Save & test` — Grafana подтвердила `Data source successfully connected.`

![](img/26_loki_derived_field.png)

Проверил, что связка работает. Открыл Explore, выбрал Loki, запрос `{namespace="monitoring", app="api-server"}`, диапазон `Last 1 hour`. Логи api-service на месте, и в каждой строке видно поле `trace_id`, вынесенное в отдельный столбец — `Showing only selected fields: trace_id`. Значения `trace_id` теперь кликабельны: обычный серый текст стал ссылкой.

![](img/27_grafana_logs_with_traceid.png)

Кликнул на `trace_id` из строки с ошибкой `/fail` (`1c8a26c8c2d663d7bfd1b299de824ee0`) — открылась новая вкладка с Jaeger UI по адресу `http://localhost:16686/trace/1c8a26c8c2d663d7bfd1b299de824ee0`. В Jaeger виден **тот самый трейс** `api-service: HTTP GET /fail` с красным error-спаном и `otel.status_code = ERROR`. То есть из строки лога в Grafana мы попали ровно в тот трейс в Jaeger, к которому эта строка относится.

![](img/28_grafana_to_jaeger.png)

Сквозная связка «лог → трейс» работает: по `trace_id` из лога открывается тот же трейс в Jaeger.

---

## Часть 4 - Алерты (Alertmanager + Karma)

### Шаг 4.1. Написание правил оповещения PrometheusRule

Метрики мы собираем, логи читаем, трейсы разглядываем — но до сих пор мы делаем это глазами. А глаза, как известно, имеют свойство отвлекаться на кофе, обед и совещания. Значит, пора вооружить сервис датчиками, которые будут орать сами.

В `kube-prometheus-stack` (Шаг 1) вместе с Prometheus приехал **Prometheus Operator** — контроллер, который управляет Prometheus'ом через Custom Resources. Один из таких CRD — **`PrometheusRule`**. Логика простая: описываешь правила в YAML, применяешь через `kubectl apply`, оператор видит объект по метке `release: kube-prom` и автоматически подгружает правила в Prometheus. Никакой ручной правки конфигов, всё декларативно.

Описал три критичных алерта в файле `deploy/prometheus-rules.yaml`. Я специально взял три разных класса проблем, чтобы покрыть и качество ответов, и их скорость, и доступность сервиса в целом.

**1. `ApiHighErrorRate` — доля 5xx выше 5% в течение 2 минут**

Ловит массовые ошибки. Если каждый двадцатый запрос уходит с 500-й, это уже не «случайный сбой», это стабильный брак на проде. Порог 5% — потому что ниже этого у пользователей ещё есть шанс не заметить, а выше — уже видно невооружённым глазом. `for: 2m` — чтобы единичный всплеск при деплое или разовом сбое не поднимал дежурного среди ночи.

**2. `ApiHighLatencyP95` — p95 времени ответа выше 1 секунды в течение 2 минут**

Ловит деградацию отклика. Тут речь не про ошибки, а про то, что 95% пользователей ждут ответа дольше секунды. Если это не единичный всплеск, а стабильная полка — сервис тормозит. Даже при нулевых 5xx это боль: люди уходят, не дожидаясь ответа. `for: 2m` — чтобы не срабатывать на джиттер.

**3. `ApiDown` — Prometheus не может скрейпить ни один под api-service 1 минуту**

Ловит полный отказ. Тут `up == 0`, то есть Prometheus вообще не смог получить `/metrics` ни с одной реплики. Возможно, поды в `CrashLoopBackOff`, возможно, кластер лежит, возможно, сеть. Это клиническая смерть сервиса — пользователи не могут достучаться вообще ни до чего.

**Что делать дежурному по каждому алерту:**

- **`ApiHighErrorRate`** — открыть RED-дашборд в Grafana, посмотреть, какой эндпоинт сыпет 5xx. Затем в Explore с Loki найти строки `level=ERROR`, взять `trace_id` и открыть тот же трейс в Jaeger по нашей сквозной связке из Шага 3.3. Смотреть, на каком шаге падает.
- **`ApiHighLatencyP95`** — в Jaeger открыть трейсы с `Duration > 1s`, развернуть водопад и найти вложенный спан, который съедает время. Возможно, БД, возможно, внешний API, возможно, GC.
- **`ApiDown`** — сначала `kubectl get pods -n monitoring -l app=api-server`, потом события кластера, потом логи api-service. Дальше по ситуации.

Применил правило:

```bash
kubectl apply -f deploy/prometheus-rules.yaml
```

`prometheusrule.monitoring.coreos.com/api-rules created`. Проверил в Prometheus UI (`Status → Rules`), что оператор подхватил правило. Группа `api.rules` на месте, все три алерта прогоняются каждые 30 секунд.

![](img/29_prometheus_rules_error_rate.png)

![](img/30_prometheus_rules_latency_down.png)

Правила готовы и активны, все три в статусе `OK` — пока условия не выполнены. Скоро мы это исправим и заставим их гореть.

---

### Шаг 4.2. Настройка маршрутизации в Alertmanager

Правила алертов у нас есть, но Prometheus сам по себе никому ничего не сообщает — он только **обнаруживает** проблему. Раскладывать уведомления по получателям должен **Alertmanager**. Он уже стоит в кластере вместе с `kube-prometheus-stack` (приехал ещё в Шаге 1), но с дефолтным конфигом: все алерты уходят в пустоту, receiver называется `"null"`.

Нам надо:

1. Поднять получателя, которому можно слать уведомления. Slack/SMTP у нас нет, поэтому используем **webhook** — самый простой вариант.
2. Перенастроить Alertmanager, чтобы он слал в этого получателя.
3. Настроить группировку, чтобы одинаковые алерты не спамили дежурного пачкой уведомлений.

**Шаг 1. Подняли webhook-приёмник.** В кластер поставили сервис `webhook-echo` — маленький под на образе `ealen/echo-server`, который принимает POST-запросы и пишет их в свой stdout. Именно туда Alertmanager будет слать уведомления, и мы сможем смотреть их в логах пода.

**Шаг 2. Написали `deploy/values/alertmanager.yaml`.** Это values для чарта `kube-prometheus-stack`, в которых мы переопределяем секцию `alertmanager`:

- **`alertmanagerSpec.configSecret`** — ключевое поле. Оно говорит оператору, из какого Secret брать конфиг. Без него оператор генерирует дефолтный конфиг сам, и наши values просто игнорируются.
- **`config.receivers`** — два получателя: `webhook-receiver` (шлёт POST на `http://webhook-echo.monitoring.svc.cluster.local/`) и `"null"` (для пустышки Watchdog).
- **`config.route`** — главный маршрут ведёт в `webhook-receiver`, группировка по `alertname` и `severity`, `group_wait: 30s`, `group_interval: 1m`, `repeat_interval: 1h`.
- **Sub-route для `severity="critical"`** — критические алерты обрабатываются быстрее: `group_wait: 10s` и `repeat_interval: 5m`. Логика простая: если у сервиса клиническая смерть — нечего ждать полчаса, звони дежурному сразу и напоминай, пока не починят.
- **Sub-route для `alertname="Watchdog"`** — этот алерт всегда горит и служит только для проверки, что Alertmanager жив. Уводим его в `"null"`, чтобы не спамил.

**Шаг 3. Поймали две ошибки.** Первая: сначала не прописали `alertmanagerSpec.configSecret` — оператор не знал, откуда брать наш конфиг, и продолжал генерировать дефолтный. Вторая: слово `null` в YAML — зарезервированное. Если написать `name: null` без кавычек, YAML превращает это в «имя отсутствует», и оператор падает с ошибкой:

```
provision alertmanager configuration: failed to initialize from secret: missing name in receiver
```

Лечится кавычками: `name: "null"` и `receiver: "null"`. После этого оператор успокоился и пересоздал `-generated` Secret с нашим конфигом.

**Шаг 4. Применили конфиг.** Два `helm upgrade` подряд (после каждой правки), финальная ревизия релиза — `REVISION: 4`. Дальше перезапустили под Alertmanager:

```bash
kubectl rollout restart statefulset -n monitoring alertmanager-kube-prom-kube-prometheus-alertmanager
```

Чтобы убедиться, что Alertmanager читает именно наш конфиг, зашли в его UI: `http://localhost:9093/#/status`. В блоке Config теперь видно:

- `route.receiver: webhook-receiver`;
- `group_by: [alertname, severity]`;
- sub-route `severity="critical"` → `webhook-receiver` с `group_wait: 10s`, `repeat_interval: 5m`;
- sub-route `alertname="Watchdog"` → `receiver: "null"`.

![](img/31_alertmanager_routes.png)

И в секции `receivers`:

- `webhook-receiver` с `send_resolved: true`;
- `"null"`.

![](img/32_alertmanager_receivers.png)

Поле `url: <secret>` в UI — это не баг, а фича: Alertmanager намеренно скрывает адрес webhook'а в отображении, чтобы не светить endpoint. Реальная настройка в Secret'е содержит `http://webhook-echo.monitoring.svc.cluster.local/`, и мы это проверяли отдельно через `kubectl get secret`.

Alertmanager готов принимать алерты от Prometheus и рассылать их нашему webhook-приёмнику. Осталось поднять удобный дашборд для наблюдения за алертами — Karma.

---

### Шаг 4.3. Развертывание и интеграция дашборда Karma

Штатный UI Alertmanager (`localhost:9093`) показывает алерты, но он бедноват: никакой группировки по сервису, фильтров, сортировок, тёмной темы. Когда алертов 3 — терпимо. Когда их 50 в 3 часа ночи — нужен пульт управления, где всё видно одним взглядом. Karma — ровно этот пульт.

Karma **не хранит алерты сам**. Он периодически дёргает API Alertmanager (`/api/v2/alerts`), получает список активных алертов и отрисовывает их с группировкой по меткам. Тонкий клиент, никаких CR и configSecret — только URL Alertmanager'а в его конфиге.

Официального Helm-чарта от авторов Karma нет, поэтому искал через `helm search hub karma`. Свежим и живым оказался чарт `zekker6/karma` (app version `v0.133`, обновлён за неделю до нашей работы, security report grade A). Добавил репозиторий:

```bash
helm repo add zekker6 https://zekker6.github.io/helm-charts/
helm repo update
```

Конфиг Karma — минимальный. В `deploy/values/karma.yaml` указал только URL Alertmanager'а через переменную окружения:

```yaml
env:
  ALERTMANAGER_URI: "http://kube-prom-kube-prometheus-alertmanager.monitoring:9093"
```

Это внутрикластерный адрес нашего Alertmanager'а (сервис `kube-prom-kube-prometheus-alertmanager` в namespace `monitoring`, порт `9093`). Больше ничего не задавал: сервис Karma по умолчанию слушает `8080`.

Установил:

```bash
helm install karma zekker6/karma -n monitoring -f deploy/values/karma.yaml
```

`STATUS: deployed`. Под поднялся:

```bash
kubectl get pods -n monitoring -l app.kubernetes.io/name=karma
```

`karma-55ff8b9998-vht2j  1/1  Running`.

Пробросил UI на `localhost:8080` и открыл в браузере. Karma подключилась к Alertmanager и сразу показала то, что в нём уже есть — 4 активных алерта от `kube-prometheus-stack`:

- **Watchdog** (`severity: none`) — стандартный алерт-пустышка от чарта, всегда горит для проверки, что Alertmanager жив. Уходит в `@receiver: null` — это **наш** sub-route из Шага 4.2, и он реально работает.
- **KubeSchedulerDown**, **KubeControllerManagerDown**, **KubeProxyDown** (`severity: critical`) — стандартные алерты чарта, сработали потому что в OrbStack-кластере этих компонентов нет как targets для скрейпа. Уходят в `@receiver: webhook-receiver` — это тоже **наш** sub-route для critical.

Итого: Karma подключена к Alertmanager, наши маршруты видны «в бою», UI полностью функционален. Наши собственные алерты `ApiHighErrorRate` / `ApiHighLatencyP95` / `ApiDown` тут пока не появились — они не firing, и мы их зажжём в Шаге 4.4.

![](img/33_karma_ui.png)

---

### Шаг 4.4. Симуляция инцидентов и проверка состояния Firing

Проверил алерты в бою: триггернул их через эндпоинты, для которых они написаны.

### Ошибки и латентность

Два цикла в отдельных вкладках:

```bash
while true; do curl -s -o /dev/null http://localhost:8090/fail; sleep 0.1; done
while true; do curl -s -o /dev/null http://localhost:8090/slow; done
```

Дашборд RED ожил: RPS ~7.3, 5xx ~93%, p95 ~1.6s.

![](img/36_grafana_red_under_load.png)

Через 2.5 минуты Prometheus перевёл `ApiHighErrorRate` и `ApiHighLatencyP95` в `FIRING`.

![](img/37_prometheus_alerts_firing.png)

Alertmanager сгруппировал по sub-routes: оба алерта → `webhook-receiver`, Watchdog → `null`.

![](img/38_alertmanager_firing.png)

Karma показала карточки с нашими кастомными аннотациями.

![](img/39_karma_firing.png)

### Сервис упал

Погасил поды:

```bash
kubectl scale deployment api-deployment -n monitoring --replicas=0
```

Первые два алерта разрешились (метрик нет), `ApiDown` перешёл в `FIRING`.

![](img/40_karma_apdown.png)

Вернул поды обратно:

```bash
kubectl scale deployment api-deployment -n monitoring --replicas=2
```

Все три алерта прошли полный путь: Prometheus → Alertmanager → Karma. Задание закрыто.

---
