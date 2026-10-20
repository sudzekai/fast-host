#!/usr/bin/env bash
# FastHost — запуск сервера хостинг-панели
#
#   ./run.sh                       # драйвер json (без внешних зависимостей)
#   ./run.sh --port 9090           # свой порт
#   ./run.sh --db sqlite           # SQLite (сборка с -tags db_sql)
#   ./run.sh --db mysql --dsn "user:pass@tcp(127.0.0.1:3306)/fasthost"
set -euo pipefail
cd "$(dirname "$0")"

mkdir -p data users bin

DB_REQ="$(echo " $* " | sed -n 's/.*--db[ =]\{1,\}\([a-z]*\).*/\1/p')"
[ -z "$DB_REQ" ] && DB_REQ="${DB_DRIVER:-}"
TAGS=""
case "$DB_REQ" in
  sqlite|mysql) TAGS="-tags db_sql" ;;
esac

echo "==> Сборка FastHost (${TAGS:-драйвер json})..."
go build $TAGS -o bin/fasthost .

echo "==> Запуск FastHost"
echo "    Панель:   http://localhost:${PORT:-8080}"
echo "    Данные:   $(pwd)/users/{username}/projects/{project}"
echo "    Драйвер:  ${DB_REQ:-json} (по умолчанию json; sqlite/mysql — с -tags db_sql)"
exec ./bin/fasthost "$@"