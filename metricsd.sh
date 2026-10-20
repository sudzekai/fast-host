#!/usr/bin/env bash
# FastHost — отдельный сервис сбора метрик (альтернатива встроенному сборщику).
# По умолчанию основной сервер уже собирает метрики сам (флаг -no-metrics отключает).
#
#   ./metricsd.sh                  # драйвер json
#   ./metricsd.sh --db sqlite      # SQLite (сборка с -tags db_sql)
set -euo pipefail
cd "$(dirname "$0")"

mkdir -p data users bin

DB_REQ="$(echo " $* " | sed -n 's/.*--db[ =]\{1,\}\([a-z]*\).*/\1/p')"
[ -z "$DB_REQ" ] && DB_REQ="${DB_DRIVER:-}"
TAGS=""
case "$DB_REQ" in
  sqlite|mysql) TAGS="-tags db_sql" ;;
esac

echo "==> Сборка metricsd (${TAGS:-драйвер json})..."
go build $TAGS -o bin/metricsd ./cmd/metricsd

echo "==> Запуск metricsd"
exec ./bin/metricsd "$@"