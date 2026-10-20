# FastHost — панель управления хостингом

Корпоративный хостинг-контроллер: клиенты загружают ZIP с сайтом (`index.html`),
указывают домен — панель сама генерирует nginx-конфиг и скрипты для SSL (certbot).
Администратор управляет пользователями, квотами и видит путь к папкам проектов.

## Стек

- **Backend**: Go (net/http, без фреймворков), JWT в HttpOnly-cookie, bcrypt
- **База данных**: единый контракт `store.Storage` с тремя драйверами:
  - `json` — JSON-файлы (по умолчанию, без внешних зависимостей, работает сразу)
  - `sqlite` — SQLite (сборка `-tags db_sql`)
  - `mysql` — MySQL (сборка `-tags db_sql`)
- **Web**: HTML + CSS + JS (vanilla), стиль Enterprise (IBM Carbon-подобный)
- **nginx / certbot**: генерируются конфиги и скрипты; реальные команды **закомментированы/выключены** до установки на домашнем сервере

## Быстрый старт

```bash
cd hosting
./run.sh                 # драйвер json, порт 8080
# или
./run.sh --port 9090
./run.sh --db sqlite     # SQLite (сборка с -tags db_sql)
```

Открыть: **http://localhost:8080**

Первый вход: логин **admin**, пароль **admin** (сменить в «Настройки»).
Регистрация новых клиентов включена (страница «Регистрация»). Её можно выключить
прямо из админки (Пользователи → «Регистрация новых клиентов») — настройка
хранится в БД. Флаг запуска `--allow-registration=false` жёстко запрещает
регистрацию и блокирует переключатель в панели.

## Структура файлов

```
hosting/
├── main.go               # точка входа, встраивает web/ в бинарь
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
└── web/                  # панель (index.html, css, js)
```

### Проекты клиентов

Всё лежит внутри каталога приложения:

```
users/
└── {username}/
    └── projects/
        └── {projectname}/
            ├── index.html ...   # файлы сайта (из загруженного ZIP)
            └── .conf/           # системные файлы проекта
                ├── nginx.conf   # конфиг nginx для домена
                ├── install.sh   # установка конфига в /etc/nginx
                ├── certbot.sh   # получение SSL-сертификата
                ├── access.log   # access-лог nginx (для метрик)
                └── error.log
```

## Тестирование nginx + SSL дома (nginx/certbot НЕ установлены здесь)

Панель **не трогает систему**: при создании проекта она пишет только `.conf/` внутри
папки проекта. Чтобы включить сайт на домашнем сервере (где nginx установлен):

```bash
# 1) Убедиться, что домен указывает на IP этого сервера (A-запись)
#    или для теста добавьте в /etc/hosts:  127.0.0.1  mydomain.example

# 2) Установить конфиг в nginx (из папки проекта):
sudo bash users/{username}/projects/{projectname}/.conf/install.sh

# 3) Получить SSL (кнопка «Применить SSL» в панели или вручную):
sudo bash users/{username}/projects/{projectname}/.conf/certbot.sh
#    Требуется: apt install certbot python3-certbot-nginx

# 4) Проверить: nginx -t && systemctl reload nginx
```

Для кнопки «Применить SSL», чтобы она **реально** выполняла certbot, запустите сервер:

```bash
./run.sh --live-ssl                 # или env ENABLE_LIVE_SSL=1
```

Без `--live-ssl` кнопка показывает инструкцию и сгенерированные скрипты (безопасный тестовый режим).

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
| `--port` | `PORT` | `8080` | HTTP-порт |
| `--db` | `DB_DRIVER` | `json` | `json`, `sqlite` (нужен `-tags db_sql`), `mysql` |
| `--dsn` | `DB_DSN` | `data/fasthost-json` | каталог для json / путь к файлу SQLite / DSN MySQL |
| `--data` | `DATA_DIR` | `.` | корень приложения; внутри создаётся `users/{login}/projects/{имя}` |
| `--secret` | `JWT_SECRET` | dev-значение | секрет подписи JWT |
| `--admin-user` | `ADMIN_USERNAME` | `admin` | первый администратор |
| `--admin-pass` | `ADMIN_PASSWORD` | `admin` | пароль первого администратора |
| `--certbot-email` | `CERTBOT_EMAIL` | `admin@example.com` | email для Let's Encrypt |
| `--live-ssl` | `ENABLE_LIVE_SSL` | выкл | реально запускать certbot |
| `--allow-registration` | `ALLOW_REGISTRATION` | вкл | самостоятельная регистрация клиентов |
| `--no-metrics` | — | вкл (сборщик) | отключить встроенный сборщик метрик |
| `--metrics-interval` | — | `60` | интервал сбора метрик, сек |

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
| DELETE | `/api/projects/{id}` | владелец/admin | удалить проект |
| POST | `/api/projects/{id}/upload` | владелец/admin | загрузить ZIP с сайтом |
| GET | `/api/projects/{id}/download` | владелец/admin | скачать файлы сайта |
| GET | `/api/projects/{id}/files` | владелец/admin | список файлов |
| GET | `/api/projects/{id}/metrics` | владелец/admin | метрики и ряд точек |
| GET | `/api/projects/{id}/nginx-config` | владелец/admin | сгенерированный конфиг |
| POST | `/api/projects/{id}/apply-ssl` | владелец/admin | certbot (или инструкция) |
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