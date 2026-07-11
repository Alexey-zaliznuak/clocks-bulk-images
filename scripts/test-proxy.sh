#!/usr/bin/env bash
# Простой тест стабильности прокси по нейтральным хостам (не OpenRouter).
# Запускать на прод-сервере: bash scripts/test-proxy.sh
#
# Цель — понять, держит ли прокси соединение вообще:
#   1) какой выходной IP/регион,
#   2) серия быстрых HTTPS-запросов (сколько OK / FAIL),
#   3) скачивание файла на несколько МБ (ловим обрывы посреди передачи = EOF).

set -u

# HTTP-порт прокси = 50100 (порт 50101 — это SOCKS5, для него схема socks5://)
PROXY_URL="${OPENROUTER_PROXY_URL:-http://zaliznuak20:oT9QirSPKm@5.22.207.240:50100}"
RUNS="${RUNS:-10}"
# Небольшой файл для стресс-теста соединения (можно переопределить)
DL_URL="${DL_URL:-https://speed.cloudflare.com/__down?bytes=5000000}"

echo "=== Тест прокси (нейтральные хосты) ==="
echo "Прокси  : $PROXY_URL"
echo "Повторов: $RUNS"
echo

echo "--- Выходной IP/регион через прокси ---"
curl -sS -x "$PROXY_URL" --max-time 30 https://ipinfo.io/json 2>&1 || echo "(не удалось получить ipinfo)"
echo
echo

ok=0
fail=0
echo "--- $RUNS быстрых HTTPS-запросов через прокси ---"
for i in $(seq 1 "$RUNS"); do
  out=$(curl -sS -x "$PROXY_URL" --max-time 30 \
    -o /dev/null \
    -w "code=%{http_code} time=%{time_total}s" \
    https://www.cloudflare.com/cdn-cgi/trace 2>&1)
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
echo "Итог быстрых запросов: OK=$ok  FAIL=$fail  из $RUNS"
echo

echo "--- Стресс: 3 скачивания ~5МБ через прокси (ловим обрывы) ---"
dok=0
dfail=0
for i in 1 2 3; do
  out=$(curl -sS -x "$PROXY_URL" --max-time 120 \
    -o /dev/null \
    -w "code=%{http_code} size=%{size_download}B speed=%{speed_download}B/s time=%{time_total}s" \
    "$DL_URL" 2>&1)
  status=$?
  if [ $status -eq 0 ] && echo "$out" | grep -q "code=200"; then
    dok=$((dok+1))
    echo "  [$i] OK   $out"
  else
    dfail=$((dfail+1))
    echo "  [$i] FAIL exit=$status $out"
  fi
done
echo
echo "Итог скачиваний: OK=$dok  FAIL=$dfail  из 3"
echo
echo "Если много FAIL / 'Recv failure' / 'transfer closed' / малый speed — прокси нестабилен."
