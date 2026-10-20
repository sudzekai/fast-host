#!/usr/bin/env bash
# FastHost — запуск сервера хостинг-панели
#
#   ./run.sh                       # драйвер json, системный nginx
#   ./run.sh --local               # панель сама запускает nginx со своим конфигом (data/nginx)
#   ./run.sh --port 9090           # свой порт
#   ./run.sh --db sqlite           # SQLite (сборка с -tags db_sql)
#   ./run.sh --db mysql --dsn "user:pass@tcp(127.0.0.1:3306)/fasthost"
set -euo pipefail
cd "$(dirname "$0")"

mkdir -p data users bin

ARGS=()
LOCAL=0
for a in "$@"; do
  if [ "$a" = "--local" ]; then LOCAL=1; else ARGS+=("$a"); fi
done
if [ "$LOCAL" = 1 ]; then
  ARGS+=(--nginx-conf "$(pwd)/data/nginx/nginx.conf")
fi

DB_REQ="$(echo " $* " | sed -n 's/.*--db[ =]\{1,\}\([a-z]*\).*/\1/p')"
[ -z "$DB_REQ" ] && DB_REQ="${DB_DRIVER:-}"
TAGS=""
case "$DB_REQ" in
  sqlite|mysql) TAGS="-tags db_sql" ;;
esac

echo "==> Сборка FastHost (${TAGS:-драйвер json})..."
go build $TAGS -o bin/fasthost .

echo "==> Запуск FastHost"
echo "    API:      http://localhost:${PORT:-8080}/api/health (вебка хостится отдельно через nginx)"
echo "    Данные:   $(pwd)/users/{username}/projects/{project}"
echo "    Драйвер:  ${DB_REQ:-json} (по умолчанию json; sqlite/mysql — с -tags db_sql)"
[ "$LOCAL" = 1 ] && echo "    nginx:    свой экземпляр, конфиг $(pwd)/data/nginx/nginx.conf"
exec ./bin/fasthost ${ARGS[@]+"${ARGS[@]}"}
