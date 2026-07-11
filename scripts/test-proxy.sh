#!/usr/bin/env bash
# Простой тест стабильности прокси до OpenRouter.
# Запускать на прод-сервере: bash scripts/test-proxy.sh
#
# Креды берутся из окружения или из .env (если лежит рядом), иначе — из дефолтов ниже.

set -u

# --- Конфиг (можно переопределить через переменные окружения) ---
PROXY_URL="${OPENROUTER_PROXY_URL:-http://zaliznuak20:oT9QirSPKm@5.22.207.240:50101}"
API_KEY="${OPENROUTER_API_KEY:-}"
BASE_URL="${OPENROUTER_BASE_URL:-https://openrouter.ai/api/v1}"
RUNS="${RUNS:-10}"

# Подхватить .env, если ключ не задан в окружении
if [ -z "$API_KEY" ] && [ -f "$(dirname "$0")/../.env" ]; then
  API_KEY="$(grep -E '^OPENROUTER_API_KEY=' "$(dirname "$0")/../.env" | head -n1 | cut -d= -f2-)"
fi

MODELS_URL="$BASE_URL/videos/models"

echo "=== Тест прокси до OpenRouter ==="
echo "Прокси : $PROXY_URL"
echo "URL    : $MODELS_URL"
echo "Повторов: $RUNS"
echo

# Куда «виден» сервер (IP/регион) — через прокси
echo "--- IP через прокси ---"
curl -sS -x "$PROXY_URL" --max-time 30 https://ipinfo.io/json || echo "(не удалось получить ipinfo)"
echo
echo

ok=0
fail=0
echo "--- $RUNS запросов GET /videos/models через прокси ---"
for i in $(seq 1 "$RUNS"); do
  # %{http_code} — код ответа, %{time_total} — общее время
  out=$(curl -sS -x "$PROXY_URL" --max-time 60 \
    -o /dev/null \
    -w "code=%{http_code} time=%{time_total}s" \
    -H "Authorization: Bearer $API_KEY" \
    "$MODELS_URL" 2>&1)
  status=$?
  if [ $status -eq 0 ] && echo "$out" | grep -q "code=200"; then
    ok=$((ok+1))
    echo "  [$i] OK   $out"
  else
    fail=$((fail+1))
    echo "  [$i] FAIL exit=$status $out"
  fi
done
echo
echo "Итог через прокси: OK=$ok  FAIL=$fail  из $RUNS"
echo

echo "--- Для сравнения: 1 прямой запрос (без прокси) ---"
curl -sS --max-time 60 -o /dev/null \
  -w "code=%{http_code} time=%{time_total}s\n" \
  -H "Authorization: Bearer $API_KEY" \
  "$MODELS_URL" 2>&1 || echo "(прямой запрос не прошёл — вероятно гео-блок, это ожидаемо)"
