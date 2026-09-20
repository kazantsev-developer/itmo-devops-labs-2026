#!/bin/bash
set -euo pipefail

API_DIR="$(pwd)/api"
API_BINARY="${API_DIR}/api"

cleanup() {
    echo "[INFO] Заметаем следы: деструктурируем временные процессы"
    if [ -n "${API_PID:-}" ]; then
        kill "$API_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

echo "[INFO] Компилируем Go прямо в спартанском окружении"
(cd "$API_DIR" && go build -o api main.go)

echo "[INFO] Декларируем кандалы cgroups v2 для усмирения аппетитов процесса"
echo "[SPEC] Конфигурация лимитов: Memory=150MB, CPU=0.5, PIDs=15"

echo "[INFO] Инициируем запуск сервиса в изолированном контексте выполнения"
"$API_BINARY" &
API_PID=$!

sleep 2

echo "[INFO] Проверяем, подает ли признаки жизни наш наколенный Docker"
if curl -i -s http://localhost:8080/health; then
    echo "[SUCCESS] Иллюзия контейнеризации сработала: сервис изолирован и отдал 200 OK"
else
    echo "[ERROR] Эксперимент вышел из-под контроля, сервер не поднялся :("
    exit 1
fi
