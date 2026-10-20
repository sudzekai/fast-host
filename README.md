# FastHost — панель управления хостингом

Корпоративный хостинг-контроллер: клиенты загружают ZIP с сайтом (`index.html`),
указывают домен — панель сама подключает сайт к nginx, выпускает SSL-сертификат
(Let's Encrypt через certbot или локальный CA для `*.localhost`/`*.test`) и
включает редирект 80 → 443. Администратор управляет пользователями, квотами и видит
пути к папкам проектов (клиенту путь не показывается).

## Стек

- **Backend**: Go (net/http, без фреймворков), JWT в HttpOnly-cookie, bcrypt
- **База данных**: единый контракт `store.Storage` с тремя драйверами:
  - `json` — JSON-файлы (по умолчанию, без внешних зависимостей, работает сразу)
  - `sqlite` — SQLite (сборка `-tags db_sql`)
  - `mysql` — MySQL (сборка `-tags db_sql`)
- **Web**: отдельный статический проект в `web/` (HTML + CSS + JS, vanilla),
  хостится через nginx и обращается к API по `API_BASE` (см. `web/js/api.js`)
- **nginx / certbot**: панель пишет конфиги сайтов, проверяет `nginx -t`, перезагружает nginx и выпускает сертификаты сама

## Быстрый старт

Нужны Go, nginx и (для публичных доменов) certbot:

```bash
brew install go nginx certbot                       # macOS
sudo apt install golang nginx certbot               # Debian/Ubuntu
```

```bash
./run.sh --local         # панель сама запускает nginx со своим конфигом, порт панели 8080
# или
./run.sh --port 9090
./run.sh --db sqlite     # SQLite (сборка с -tags db_sql)
```

Открыть API: **http://localhost:8080/api/health**

Вебка — отдельный проект: папку `web/` раздаёт nginx, все запросы идут на
`https://api.sudzekai.ru/fast-host` (префикс срезает nginx). Чтобы браузер
передавал cookie между доменами, разрешите origin вебки на API:

```bash
./run.sh --cors-origins "https://panel.sudzekai.ru,http://localhost:8081"
```

Первый вход: логин **admin**, пароль **admin** (сменить в «Настройки»).
Регистрация новых клиентов включена (страница «Регистрация»). Её можно выключить
прямо из админки (Пользователи → «Регистрация новых клиентов») — настройка
хранится в БД. Флаг запуска `--allow-registration=false` жёстко запрещает
регистрацию и блокирует переключатель в панели.

## Структура файлов

```
hosting/
├── main.go               # точка входа (только API, статику не отдаёт)
├── run.sh                # сборка и запуск сервера
├── metricsd.sh           # отдельный сервис метрик (альтернатива встроенному)
├── internal/
│   ├── config/           # флаги и переменные окружения
│   ├── store/            # контракт Storage + драйверы: json / sqlite / mysql
│   ├── auth/             # JWT, bcrypt, middleware
│   ├── nginx/            # генерация nginx-конфига и certbot-скриптов
│   ├── metrics/          # сборщик метрик (диск, access.log)
│   └── handlers/         # REST API + статика
├── cmd/metricsd/         # отдельный сервис метрик
└── web/                  # отдельный фронтенд-проект (хостится через nginx, не через API)
```

### Проекты клиентов и служебные данные

```
users/
└── {username}/projects/{projectname}/
    ├── index.html ...      # файлы сайта (из загруженного ZIP)
    └── .conf/              # служебное, закрыто от веба
        ├── access.log      # пишет nginx (для метрик)
        └── error.log
data/
├── nginx/
│   ├── nginx.conf          # только в режиме --local / --nginx-conf
│   └── sites/              # конфиги сайтов: {домен}.conf (+ 00-default.conf)
├── ssl/                    # локальный CA (ca.pem) и самоподписанные сертификаты
├── letsencrypt/            # certbot --config-dir (работает без root)
└── acme/                   # webroot для проверки Let's Encrypt
```

Включённый проект = файл `data/nginx/sites/{домен}.conf`. Выключение удаляет его и
перезагружает nginx — домен перестаёт обслуживаться (заглушка `default_server`
закрывает соединение, запрос не попадает на чужой сайт). Удаление проекта убирает
конфиг, сертификаты, метрики и папку проекта с диска.

## nginx и SSL

Панель всё делает сама: при создании проекта пишет конфиг, проверяет `nginx -t`
(при ошибке откатывает файл, чтобы не сломать остальные сайты) и перезагружает nginx.
В конфигах только рабочие директивы, без комментариев.

**Локально (macOS / разработка)** — `./run.sh --local`: панель генерирует
`data/nginx/nginx.conf` и запускает свой экземпляр nginx на 80/443 (на macOS root не
нужен). Для доменов `*.localhost`, `*.test`, `*.local` и т.п. Let's Encrypt невозможен,
поэтому сертификат подписывается локальным CA панели (`data/ssl/ca.pem`). Проверка:

```bash
curl http://demo.localhost/                                  # *.localhost резолвится в 127.0.0.1
curl -I http://demo.localhost/                               # после SSL: 301 → https
curl --cacert data/ssl/ca.pem https://demo.localhost/
```

Чтобы браузер не ругался, добавьте `data/ssl/ca.pem` в доверенные (Связка ключей → «Всегда доверять»).

**Сервер с системным nginx** — без `--local`. В `http {}` основного конфига nginx добавьте

```nginx
include /путь/к/fast-host/data/nginx/sites/*.conf;
```

и уберите стандартный сайт с `default_server` (панель ставит свою заглушку; отключить —
`--nginx-default-server=false`). Панель должна иметь право перезагружать nginx: запуск от
root или `--nginx-reload-cmd "sudo systemctl reload nginx"`. Сертификаты Let's Encrypt
выпускаются через `certbot certonly --webroot` (домен должен указывать на этот сервер,
порт 80 открыт), продлеваются панелью автоматически за 30 дней до истечения. Для проверки
без лимитов Let's Encrypt — `--ssl-staging`.

## База данных: единый контракт для JSON / SQLite / MySQL

Весь доступ к данным — через интерфейс `store.Storage` (`internal/store/store.go`).
Драйверы (`internal/store/`):

| Драйвер | Файл | Когда |
|---|---|---|
| `json` | `json.go` | по умолчанию; JSON-файлы в каталоге DSN |
| `sqlite` | `sql.go` | `go build -tags db_sql` |
| `mysql` | `sql.go` | `go build -tags db_sql` |

SQL-драйверы компилируются только с тегом `db_sql` (иначе не тянется
`modernc.org/sqlite`). `run.sh` автоматически добавляет тег, если запрошен
`--db sqlite|mysql`.

```bash
# JSON (по умолчанию)
./run.sh

# SQLite
./run.sh --db sqlite
# или вручную:
go build -tags db_sql -o bin/fasthost . && ./bin/fasthost --db sqlite

# MySQL (создайте базу заранее: CREATE DATABASE fasthost CHARACTER SET utf8mb4)
./run.sh --db mysql --dsn "user:password@tcp(127.0.0.1:3306)/fasthost?parseTime=true&charset=utf8mb4"
```

## Параметры запуска

| Флаг | Env | По умолчанию | Описание |
|---|---|---|---|
| `--port` | `PORT` | `8080` | HTTP-порт панели |
| `--db` | `DB_DRIVER` | `json` | `json`, `sqlite` (нужен `-tags db_sql`), `mysql` |
| `--dsn` | `DB_DSN` | `data/fasthost-json` | каталог для json / путь к файлу SQLite / DSN MySQL |
| `--data` | `DATA_DIR` | `.` | корень приложения; внутри создаётся `users/{login}/projects/{имя}` |
| `--secret` | `JWT_SECRET` | случайный, хранится в БД | секрет подписи JWT |
| `--admin-user` | `ADMIN_USERNAME` | `admin` | первый администратор |
| `--admin-pass` | `ADMIN_PASSWORD` | `admin` | пароль первого администратора |
| `--allow-registration` | `ALLOW_REGISTRATION` | вкл | самостоятельная регистрация клиентов |
| `--no-metrics` | — | вкл (сборщик) | отключить встроенный сборщик метрик |
| `--metrics-interval` | — | `60` | интервал сбора метрик, сек |
| `--cors-origins` | `CORS_ORIGINS` | пусто | разрешённые origin вебки через запятую (иначе только same-origin) |
| `--nginx-bin` | `NGINX_BIN` | `nginx` | исполняемый файл nginx |
| `--nginx-conf` | `NGINX_CONF` | — | свой главный конфиг: панель создаёт его и сама запускает nginx (`run.sh --local`) |
| `--nginx-sites` | `NGINX_SITES_DIR` | `data/nginx/sites` | каталог конфигов сайтов (его подключает `include`) |
| `--nginx-reload-cmd` | `NGINX_RELOAD_CMD` | `nginx -s reload` | своя команда перезагрузки |
| `--nginx-http-port` / `--nginx-https-port` | `NGINX_HTTP_PORT` / `NGINX_HTTPS_PORT` | `80` / `443` | порты сайтов |
| `--nginx-default-server` | `NGINX_DEFAULT_SERVER` | вкл | заглушка для неизвестных/выключенных доменов |
| `--ssl-provider` | `SSL_PROVIDER` | `auto` | `auto` (локальные домены — свой CA, остальные — Let's Encrypt), `letsencrypt`, `selfsigned` |
| `--ssl-staging` | `SSL_STAGING` | выкл | тестовый сервер Let's Encrypt |
| `--certbot-bin` | `CERTBOT_BIN` | `certbot` | исполняемый файл certbot |
| `--certbot-email` | `CERTBOT_EMAIL` | `admin@example.com` | email для Let's Encrypt |
| `--certbot-dir`, `--ssl-dir`, `--acme-dir` | `CERTBOT_DIR`, `SSL_DIR`, `ACME_DIR` | `data/…` | каталоги сертификатов и ACME |

## Метрики

- Встроенный сборщик каждые 60 с пишет в БД: размер папки проекта, число запросов
  и трафик из `.conf/access.log` (инкрементально, по смещению).
- Графики «Диск, ГБ» и «Запросы/мин» — в карточке проекта.
- Можно вынести в отдельный сервис: `./metricsd.sh` (тот же контракт БД,
  запускать на том же каталоге `users`).

## API

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| POST | `/api/auth/login` | — | вход, установка cookie |
| POST | `/api/auth/register` | — | регистрация клиента |
| POST | `/api/auth/logout` | — | выход |
| GET | `/api/me` | user | текущий пользователь + занято |
| PUT | `/api/users/me/password` | user | смена своего пароля |
| GET | `/api/projects` | user | свои проекты |
| POST | `/api/projects` | user | создать проект (домен) |
| GET | `/api/projects/{id}` | владелец/admin | детали |
| DELETE | `/api/projects/{id}` | владелец/admin | удалить проект (конфиг, сертификат, файлы с диска) |
| POST | `/api/projects/{id}/upload` | владелец/admin | загрузить ZIP с сайтом |
| GET | `/api/projects/{id}/download` | владелец/admin | скачать файлы сайта |
| GET | `/api/projects/{id}/files` | владелец/admin | список файлов |
| GET | `/api/projects/{id}/metrics` | владелец/admin | метрики и ряд точек |
| PUT | `/api/projects/{id}/enabled` | владелец/admin | `{enabled}` — включить/выключить сайт в nginx |
| POST | `/api/projects/{id}/apply-ssl` | владелец/admin | выпустить сертификат, включить HTTPS и редирект 80 → 443 |
| GET | `/api/projects/{id}/nginx-config` | admin | сгенерированный конфиг сайта |
| GET | `/api/admin/users` | admin | список пользователей с квотами |
| GET | `/api/admin/users/{id}` | admin | пользователь + проекты + пути |
| PUT | `/api/admin/users/{id}/password` | admin | смена пароля пользователя |
| PUT | `/api/admin/users/{id}/quota` | admin | лимит хранения (2–100 ГБ) |
| PUT | `/api/admin/registration` | admin | включить/выключить регистрацию |
| GET | `/api/admin/stats` | admin | общая статистика (+ `registration_enabled`) |

## Ограничения

- Имя проекта: `a-z0-9 _-`, 2–64 символа.
- Домен: валидное hostname с точкой (или `localhost` для теста).
- ZIP должен содержать `index.html` (или `index.htm`) в корне; путь-трэверсал отвергается.
- Квота: суммарно все проекты пользователя ≤ лимита (проверяется при загрузке), по умолчанию 5 ГБ, настраивается админом.