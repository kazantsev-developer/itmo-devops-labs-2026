РАПОРТ БЮРО GOPHER A.I.D.D.
Ночной инцидент, namespace shop, окно 03:12–03:15 UTC (2026-10-05)

УЛИКИ:
1. metrics.json: pod api — CPU 94.2%, память 248 МБ, горутины 1420, error rate 93%, p95 1.6 c, 4 рестарта за час, статус CrashLoopBackOff. Pod worker — CPU 41.5%, память 132 МБ, горутины 310, error rate 0.4%, статус Running.
2. values.yaml: лимиты api — CPU 200m, память 256Mi, реплики 3; лимиты worker — CPU 200m, память 256Mi, реплики 2.
3. Расчёт отклонений: память api 248/256 МБ = 96.875% лимита (остаток всего 3.125%); память worker 132/256 МБ = 51.5625% лимита; CPU api 94.2% (запас 5.8%), CPU worker 41.5% (запас 58.5%).
4. errors.log: повторяющиеся HTTP 500 на GET /fail (3 записи), upstream timeout 1600ms на route=/orders, WARN о горутинах 1420 при пороге 500, "superfluous response.WriteHeader call", "context deadline exceeded waiting for postgres", финальная запись "pod restarting, reason=CrashLoopBackOff restarts=4".

ДИАГНОЗ (три предложения):
Pod api находится в CrashLoopBackOff из-за исчерпания ресурсов: память 248 МБ составляет 96.875% лимита 256Mi, а CPU 94.2% упирается в потолок, что приводит к OOM-давлению и 4 рестартам за час. Лог показывает корневую причину — зависание/дедлайн на ожидание postgres ("context deadline exceeded waiting for postgres") и таймауты upstream на /orders, из-за чего запросы копятся, горутины раздулись до 1420 при пороге 500, а error rate достиг 93%. Подсистема worker при этом здорова (память 51.5625% лимита, 0 рестартов), значит проблема локализована в api и/или в связке api с базой данных.

РЕКОМЕНДАЦИЯ ДЕЖУРНОМУ:
Немедленно проверить доступность и нагрузку postgres (shop-postgres-rw.shop.svc.cluster.local:5432) и пул соединений api; временно поднять лимит памяти api до 512Mi и CPU до 400m, чтобы остановить цикл CrashLoopBackOff, затем откатить/проверить образ shop-api:1.0.2 и добавить алерт на порог горутины 500.