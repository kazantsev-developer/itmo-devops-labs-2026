## Обычный запуск

**Проверка, что поды живы:**

```bash
kubectl get pods -n monitoring
```

**Пробросить порты в локалку:**

**Вкладка 1 - Grafana** http://localhost:3000

```bash
kubectl port-forward -n monitoring svc/kube-prom-grafana 3000:80
```

**Вкладка 2 - Prometheus** http://localhost:9090

```bash
kubectl port-forward -n monitoring svc/kube-prom-kube-prometheus-prometheus 9090:9090
```

**Вкладка 3 - Alertmanager** http://localhost:9093

```bash
kubectl port-forward -n monitoring svc/kube-prom-kube-prometheus-alertmanager 9093:9093
```

**Вкладка 4 - Karma** http://localhost:8080

```bash
kubectl port-forward -n monitoring svc/karma 8080:8080
```

**Вкладка 5 - Jaeger** http://localhost:16686

```bash
kubectl port-forward -n monitoring svc/jaeger 16686:16686
```

**Достать пароль от Grafana (логин: admin):**

```bash
kubectl get secret -n monitoring kube-prom-grafana -o jsonpath="{.data.admin-password}" | base64 -d; echo
```

---

## Если кластер пустой (реанимация)

Если ставлю стек с нуля на чистый Kubernetes в OrbStack. Все команды выполняю из корня `lab-02-monitoring/`.

**1. Подготовка репозиториев и неймспейса:**

```bash
kubectl create namespace monitoring

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm repo add zekker6 https://zekker6.github.io/helm-charts/
helm repo update
```

**2. Установка 5 Helm-релизов:**

```bash
helm install kube-prom prometheus-community/kube-prometheus-stack -n monitoring -f deploy/values/kube-prom-stack.yaml
helm install loki grafana/loki -n monitoring -f deploy/values/loki.yaml
helm install alloy grafana/alloy -n monitoring -f deploy/values/alloy.yaml
helm install jaeger jaegertracing/jaeger -n monitoring -f deploy/values/jaeger.yaml
helm install karma zekker6/karma -n monitoring -f deploy/values/karma.yaml
```

**3. Сборка Docker-образа:**

```bash
docker build -t api-slim:1.0.0 .
```

**4. Применение манифестов и правил:**

```bash
kubectl apply -f deploy/webhook-echo.yaml
kubectl apply -f deploy/api.yaml
kubectl apply -f deploy/service-monitor.yaml
kubectl apply -f deploy/prometheus-rules.yaml
kubectl apply -f deploy/grafana-datasource-loki.yaml
kubectl apply -f deploy/grafana-dashboard-red.yaml
```

**5. Проверка статуса:**
Все поды должны быть в статусе `Running`. Если что-то в `Pending` / `CrashLoopBackOff`, подождать 30 секунд и проверить снова!

```bash
kubectl get pods -n monitoring
kubectl get pvc -n monitoring
```
