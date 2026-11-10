# FastHost — панель управления хостингом

Корпоративный хостинг-контроллер: клиенты загружают ZIP с сайтом (`index.html`
или `index.php`), указывают домен — панель сама подключает сайт к nginx, выпускает
SSL-сертификат (Let's Encrypt через certbot или локальный CA для `*.localhost`/`*.test`)
и включает редирект 80 → 443. Администратор управляет пользователями, квотами и видит
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

Для PHP-сайтов дополнительно установите PHP-FPM:

```bash
sudo apt install php-fpm                            # Debian/Ubuntu
# сокет обычно /run/php/php8.x-fpm.sock — панель найдёт его автоматически
```

```bash
./api/run.sh --local         # панель сама запускает nginx со своим конфигом, порт панели 8080
# или
./api/run.sh --port 9090
./api/run.sh --db sqlite     # SQLite (сборка с -tags db_sql)
```

Открыть API: **http://localhost:8080/api/health**

Вебка — отдельный проект: папку `web/` раздаёт nginx, все запросы идут на
`https://api.sudzekai.ru/fast-host` (префикс срезает nginx). Чтобы браузер
передавал cookie между доменами, разрешите origin вебки на API:

```bash
./api/run.sh --cors-origins "https://panel.sudzekai.ru,http://localhost:8081"
```

Первый вход: логин **admin**, пароль **admin** (сменить в «Настройки»).
Регистрация новых клиентов включена (страница «Регистрация»). Её можно выключить
прямо из админки (Пользователи → «Регистрация новых клиентов») — настройка
хранится в БД. Флаг запуска `--allow-registration=false` жёстко запрещает
регистрацию и блокирует переключатель в панели.

## Структура файлов

```
fast-host/
├── api/                  # самостоятельный Go-модуль сервера
│   ├── go.mod, go.sum     # зависимости API
│   ├── main.go            # точка входа API
│   ├── run.sh             # сборка и запуск API из корня проекта
│   ├── metricsd.sh        # сборка и запуск отдельного сборщика метрик
│   ├── cmd/metricsd/      # точка входа сборщика метрик
│   └── internal/
│       ├── config/        # флаги и переменные окружения
│       ├── store/         # Storage: JSON / SQLite / MySQL
│       ├── auth/          # JWT, bcrypt, middleware
│       ├── nginx/         # управление nginx и SSL
│       ├── metrics/       # сбор метрик
│       ├── httpx/         # HTTP/JSON-хелперы
│       └── handlers/      # REST API
├── web/                  # самостоятельный статический фронтенд
│   ├── index.html
│   ├── css/
│   └── js/
├── data/                 # служебные данные API (не в Git)
├── users/                # сайты пользователей (не в Git)
└── bin/                  # собранные исполняемые файлы (не в Git)
```

Go-команды выполняются в `api/` или из корня с `go -C api ...`:

```bash
go -C api test ./...
go -C api test -tags db_sql ./...
go -C api vet ./...
go -C api vet -tags db_sql ./...
```

Скрипты `api/run.sh` и `api/metricsd.sh` можно вызывать из любого каталога:
они используют корень репозитория как рабочий каталог. Поэтому существующие
`data/`, `users/` и относительные пути конфигурации сохраняются. При ручном
запуске бинарного файла используйте корень проекта как рабочий каталог.
Фронтенд развёртывается отдельно: document root nginx указывает на `web/`.

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

**Локально (macOS / разработка)** — `./api/run.sh --local`: панель генерирует
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

**PHP в старых проектах** — если php-fpm установлен, панель находит стандартный сокет
(`/run/php/php*-fpm.sock`) и добавляет в конфиги сайтов `fastcgi_pass`, `index.php`
и обработку `.php`. Нужен свой адрес — `--php-fpm unix:/путь/к/php-fpm.sock` или
`--php-fpm 127.0.0.1:9000`; отключить — `--php-fpm off`. После загрузки ZIP с
`index.php` сайт начинает выполняться без дополнительных настроек. Для новых
проектов выберите тип `PHP-FPM`: он использует отдельный пул, описанный ниже.

## PHP-проекты и каталоги данных

При создании проекта выберите `nginx` (статика) или `PHP-FPM` (PHP).
В карточке существующего проекта тип можно изменить. У PHP-проектов собственные
системный пользователь и пул; nginx обслуживает HTTP/HTTPS, SSL и access.log,
поэтому загрузка ZIP, деплой, метрики и переключатель сайта работают одинаково.
Старые проекты без поля `provider` сохраняют прежнюю общую настройку `--php-fpm`.

Управляемые пулы рассчитаны на Linux с systemd и расположением PHP как в Debian.
Установите PHP CLI, FPM и SQLite-модуль одной версии (не ниже 8.2):

```bash
sudo apt install php8.4-cli php8.4-fpm php8.4-sqlite3
sudo systemctl enable --now php8.4-fpm
sudo ./api/run.sh --php-version 8.4
```

API нужны права root для создания пользователей, назначения владельца каталогов
и управления пулами. PHP выполняется от отдельного пользователя проекта.
Сокеты пулов доступны nginx-пользователю `www-data`. Версия по умолчанию — 8.4;
её можно изменить через `--php-version` / `PHP_VERSION`.
Перед созданием пула проверяются PHP 8.2+, `pdo_sqlite` и работоспособность Argon2id.

В карточке PHP-проекта откройте **Каталоги данных → Создать каталог**.
Задайте имя папки и переменной окружения, например `database` и `APP_DATA_DIR`.
Панель создаст `data/project-data/{ключ проекта}/volumes/database` вне корня сайта,
назначит пользователя пула владельцем и установит права `0700`.
Абсолютный путь передаётся через `env[APP_DATA_DIR]` в конфигурации пула.
Произвольные серверные пути из браузера не принимаются.

В каталоге можно создавать, загружать и редактировать текстовые конфигурационные
файлы размером до 1 МБ (права `0600`, владелец — пользователь пула).
Базы SQLite изменяет приложение, а не редактор конфигурации.
Для `brosayukurit.online` создайте переменную `BROSAYU_DATA_DIR`, затем файл
`config.json` с содержимым `{"origin":"https://brosayukurit.online"}`.
Это настройки приложения, они не зашиты в код панели.

Данные учитываются в размере проекта и дисковой квоте, сохраняются при ZIP-деплое
и выключении сайта, удаляются при удалении проекта. Скачивание ZIP содержит
только файлы сайта. При выключении проекта пул убирается; при включении создаётся
повторно с прежними каталогами и переменными.

Отдельный проверочный сценарий запускает временные API/nginx на портах
18081/18080/18443 и проверяет реальный PHP-FPM, SQLite, Argon2id, права,
переменные, SSL, метрики и сохранность базы при деплое:

```bash
sudo python3 api/tests/phpfpm_integration.py /absolute/path/to/fasthost 8.4
```

## База данных: единый контракт для JSON / SQLite / MySQL

Весь доступ к данным — через интерфейс `store.Storage` (`api/internal/store/store.go`).
Драйверы (`api/internal/store/`):

| Драйвер | Файл | Когда |
|---|---|---|
| `json` | `json.go` | по умолчанию; JSON-файлы в каталоге DSN |
| `sqlite` | `sql.go` | `go build -tags db_sql` |
| `mysql` | `sql.go` | `go build -tags db_sql` |

SQL-драйверы компилируются только с тегом `db_sql` (иначе не тянется
`modernc.org/sqlite`). `api/run.sh` автоматически добавляет тег, если запрошен
`--db sqlite|mysql`.

```bash
# JSON (по умолчанию)
./api/run.sh

# SQLite
./api/run.sh --db sqlite
# или вручную:
go -C api build -tags db_sql -o ../bin/fasthost . && ./bin/fasthost --db sqlite

# MySQL (создайте базу заранее: CREATE DATABASE fasthost CHARACTER SET utf8mb4)
./api/run.sh --db mysql --dsn "user:password@tcp(127.0.0.1:3306)/fasthost?parseTime=true&charset=utf8mb4"
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
| `--nginx-conf` | `NGINX_CONF` | — | свой главный конфиг: панель создаёт его и сама запускает nginx (`api/run.sh --local`) |
| `--nginx-sites` | `NGINX_SITES_DIR` | `data/nginx/sites` | каталог конфигов сайтов (его подключает `include`) |
| `--nginx-reload-cmd` | `NGINX_RELOAD_CMD` | `nginx -s reload` | своя команда перезагрузки |
| `--nginx-http-port` / `--nginx-https-port` | `NGINX_HTTP_PORT` / `NGINX_HTTPS_PORT` | `80` / `443` | порты сайтов |
| `--nginx-default-server` | `NGINX_DEFAULT_SERVER` | вкл | заглушка для неизвестных/выключенных доменов |
| `--ssl-provider` | `SSL_PROVIDER` | `auto` | `auto` (локальные домены — свой CA, остальные — Let's Encrypt), `letsencrypt`, `selfsigned` |
| `--ssl-staging` | `SSL_STAGING` | выкл | тестовый сервер Let's Encrypt |
| `--certbot-bin` | `CERTBOT_BIN` | `certbot` | исполняемый файл certbot |
| `--certbot-email` | `CERTBOT_EMAIL` | `admin@example.com` | email для Let's Encrypt |
| `--certbot-dir`, `--ssl-dir`, `--acme-dir` | `CERTBOT_DIR`, `SSL_DIR`, `ACME_DIR` | `data/…` | каталоги сертификатов и ACME |
| `--php-fpm` | `PHP_FPM` | автоопределение | адрес php-fpm (`unix:/run/php/php-fpm.sock`, `127.0.0.1:9000`); `off` — отключить PHP |

## Метрики

В карточке проекта показаны состояние сборщика, время последнего замера и ошибки.
Экран обновляется каждые 10 секунд; «Обновить замер» запускает сбор немедленно.
На главной странице и в списке проектов можно пересчитать использование диска.
Скорость запросов нормализуется к минуте по фактическим временам двух замеров.
График хранилища отображает размер в подходящих единицах, без округления малых
сайтов до нуля гигабайт. Служебные логи показываются отдельно и не входят в квоту;
в квоту входят файлы сайта и данные PHP. Ошибка чтения файлов сохраняет предыдущий
замер и отображается в панели. Счётчики и позиция чтения access.log сохраняются
вместе, чтобы повторный сбор после ошибки не терял запросы.

На главной странице администратора отображается RAM всего сервера из Linux
`/proc/meminfo`: использовано, доступно, всего и swap, с временем замера.
Эти показатели относятся ко всем приложениям на сервере, не к отдельному проекту.

- Встроенный сборщик каждые 60 с пишет в БД: размер папки проекта, число запросов
  и трафик из `.conf/access.log` (инкрементально, по смещению).
- Графики «Диск, ГБ» и «Запросы/мин» — в карточке проекта.
- Можно вынести в отдельный сервис: `./api/metricsd.sh` (тот же контракт БД,
  запускать на том же каталоге `users`).

## API

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| POST | `/api/auth/login` | — | вход (`remember: true` — сессия на 30 дней), установка cookie |
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
| PUT | `/api/projects/{id}/provider` | владелец/admin | `{provider: "nginx" или "php-fpm"}` — тип проекта |
| GET | `/api/projects/{id}/data-directories` | владелец/admin | каталоги данных, переменные, абсолютные пути, имена файлов |
| POST | `/api/projects/{id}/data-directories` | владелец/admin | `{name, env}` — создать каталог и подключить к PHP-пулу |
| GET | `/api/projects/{id}/data-directories/{directory}/files/{file}` | владелец/admin | прочитать текстовый файл до 1 МБ |
| PUT | `/api/projects/{id}/data-directories/{directory}/files/{file}` | владелец/admin | `{content}` — сохранить текстовый файл |
| POST | `/api/projects/{id}/apply-ssl` | владелец/admin | выпустить сертификат, включить HTTPS и редирект 80 → 443 |
| GET | `/api/projects/{id}/nginx-config` | admin | сгенерированный конфиг сайта |
| GET | `/api/admin/users` | admin | список пользователей с квотами |
| GET | `/api/admin/users/{id}` | admin | пользователь + проекты + пути |
| PUT | `/api/admin/users/{id}/password` | admin | смена пароля пользователя |
| PUT | `/api/admin/users/{id}/quota` | admin | лимит хранения (2–100 ГБ) |
| PUT | `/api/admin/registration` | admin | включить/выключить регистрацию |
| GET | `/api/admin/stats` | admin | общая статистика (+ `registration_enabled`) |
| GET | `/api/users/me/api-key` | user | свой API-ключ деплоя |
| POST | `/api/users/me/api-key/regenerate` | user | перевыпустить ключ (старый перестаёт работать) |
| POST | `/api/deploy/{id}` | API-ключ | загрузить zip с файлами проекта (полностью заменяет файлы сайта) |

## Деплой из GitHub Actions

В разделе «Настройки» находится пример для PHP-сайта: выбор проекта подставляет
его ID в YAML, кнопка «Копировать YAML» копирует весь workflow. Там же доступны
пояснения по секрету `FASTHOST_API_KEY`, составу архива и полной замене файлов.
Пример проверяет PHP-файлы перед упаковкой и исключает `.env` и базы SQLite.

У каждого пользователя есть уникальный API-ключ (Настройки → «API-ключ деплоя»).
Workflow собирает сайт, пакует в zip и отправляет его на деплой:

```yaml
name: Deploy
on:
  push:
    branches: [main]
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20
      - run: npm ci && npm run build
      - name: Package
        run: |
          cd dist
          zip -r ../site.zip .
      - name: Deploy
        run: |
          curl -f -X POST \
            -H "X-Api-Key: ${{ secrets.FASTHOST_API_KEY }}" \
            -H "Content-Type: application/zip" \
            --data-binary @site.zip \
            https://api.sudzekai.ru/fast-host/api/deploy/1
```

`1` — id проекта (в URL проекта в панели), `FASTHOST_API_KEY` — секрет репозитория
с API-ключом пользователя. Вместо `dist` подставьте папку сборки. В zip должен
быть `index.html` (или `index.php`) в корне (как при загрузке через панель).
**Деплой полностью заменяет файлы сайта:** всё, чего нет в новом архиве, удаляется;
служебный каталог `.conf` (логи) не затрагивается.

## Ограничения

- Имя проекта: `a-z0-9 _-`, 2–64 символа.
- Домен: валидное hostname с точкой (или `localhost` для теста).
- ZIP должен содержать `index.html`, `index.htm` или `index.php` в корне; путь-трэверсал отвергается.
- Квота: суммарно все проекты пользователя ≤ лимита (проверяется при загрузке), по умолчанию 5 ГБ, настраивается админом.
