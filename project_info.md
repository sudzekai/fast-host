# FastHost — project_info.md

Онбординг-документ для другого ИИ / разработчика. Здесь собрано всё необходимое,
чтобы быстро понять проект и начать вносить изменения: архитектура, контракты,
структура кода, API, сценарии работы, особенности и подводные камни.

---

## 1. Что это

Корпоративный хостинг-контроллер (аналог «панели хостинга» в стиле FastPanel):

- **Клиент**: логин/пароль → панель управления → создаёт проект, указывает домен,
  загружает ZIP с сайтом (обязателен `index.html`). При создании проекта панель
  сразу генерирует **nginx-конфиг** и скрипты для **SSL (certbot)**.
- **Админ**: список пользователей, их проекты, лимиты хранения (квоты), смена
  паролей, пути к папкам проектов, вкл/выкл регистрации.
- **Метрики**: сбор (размер на диске + запросы/трафик из nginx access.log) и
  графики в карточке проекта.

nginx и certbot на машине разработки **не установлены**: панель только пишет
конфиги/скрипты в `.conf/` проекта, реальные команды выполняются опционально
(флаг `--live-ssl`) на домашнем/боевом сервере.

---

## 2. Технологии

| Слой | Технология |
|---|---|
| Backend | Go 1.26, только stdlib (`net/http`), без фреймворков |
| Авторизация | JWT (golang-jwt/v5) в **HttpOnly-cookie**, bcrypt (x/crypto) |
| БД | единый контракт `store.Storage`: драйверы **json** (по умолчанию), **sqlite** (modernc.org/sqlite), **mysql** (go-sql-driver/mysql) |
| Web | HTML + CSS + JS (vanilla), без сборщиков и фреймворков, встроено в бинарь через `//go:embed` |
| Шрифт | IBM Plex Sans (+ IBM Plex Mono для кода) |

---

## 3. Быстрый старт

```bash
cd /home/sudzekai/Desktop/hosting

./run.sh                        # драйвер json, порт 8080
./run.sh --port 9090
./run.sh --db sqlite            # SQLite (автосборка с -tags db_sql)
./run.sh --db mysql --dsn "user:pass@tcp(127.0.0.1:3306)/fasthost"
```

- Панель: **http://localhost:8080**
- Первый вход: **admin / admin** (администратор создаётся автоматически при первом
  запуске; логин/пароль задаются флагами `--admin-user`/`--admin-pass`).
- Регистрация клиентов включена (страница «Регистрация»), отключается из админки.

### Тестовый сценарий (curl)

```bash
# 1) войти как админ, сохранить cookie
curl -c c.txt -X POST localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin"}'

# 2) создать проект (сразу генерирует nginx-конфиг в .conf/)
curl -b c.txt -X POST localhost:8080/api/projects \
  -H 'Content-Type: application/json' -d '{"name":"site","domain":"example.com"}'

# 3) загрузить zip с index.html
printf '<h1>hi</h1>' > index.html && zip site.zip index.html
curl -b c.txt -X POST localhost:8080/api/projects/1/upload -F "file=@site.zip"

# 4) посмотреть метрики / конфиг / файлы
curl -b c.txt localhost:8080/api/projects/1/metrics
curl -b c.txt localhost:8080/api/projects/1/nginx-config
curl -b c.txt localhost:8080/api/projects/1/files
```

---

## 4. Конфигурация

Флаги CLI (приоритет: флаг > env > дефолт):

| Флаг | Env | Дефолт | Описание |
|---|---|---|---|
| `--port` | `PORT` | `8080` | HTTP-порт |
| `--db` | `DB_DRIVER` | `json` | `json` \| `sqlite` \| `mysql` (sql* — нужна сборка `-tags db_sql`) |
| `--dsn` | `DB_DSN` | `data/fasthost-json` | json: каталог; sqlite: путь к файлу; mysql: DSN |
| `--data` | `DATA_DIR` | `.` | корень приложения; внутри создаётся `users/` |
| `--secret` | `JWT_SECRET` | dev-значение | секрет подписи JWT |
| `--admin-user` | `ADMIN_USERNAME` | `admin` | первый администратор |
| `--admin-pass` | `ADMIN_PASSWORD` | `admin` | его пароль |
| `--certbot-email` | `CERTBOT_EMAIL` | `admin@example.com` | email для Let's Encrypt |
| `--live-ssl` | `ENABLE_LIVE_SSL` | выкл | реально выполнять certbot/nginx при «Применить SSL» |
| `--allow-registration` | `ALLOW_REGISTRATION` | вкл | жёсткий флаг регистрации (блокирует переключатель в админке) |
| `--no-metrics` | — | выкл | отключить встроенный сборщик метрик |
| `--metrics-interval` | — | `60` | интервал сбора метрик, сек |

**Сборка с SQL-драйверами**: файл `internal/store/sql.go` и регистратор
`sql_registry_sql.go` компилируются только с `//go:build db_sql`:

```bash
go build -tags db_sql -o bin/fasthost .
```

Без тега работает только драйвер `json`; `NewStorage("sqlite", ...)` вернёт ошибку
с подсказкой. `run.sh` сам добавляет тег, если в аргументах есть `--db sqlite|mysql`.

---

## 5. Структура проекта

```
hosting/
├── main.go                  # точка входа: конфиг, БД, сид админа, сборщик метрик, http-сервер
├── go.mod / go.sum          # module hosting
├── run.sh                   # сборка+запуск сервера (автотег db_sql при --db sqlite/mysql)
├── metricsd.sh              # сборка+запуск отдельного сервиса метрик
├── README.md                # пользовательская документация
├── project_info.md          # ЭТОТ документ
├── cmd/
│   └── metricsd/main.go     # отдельный сервис метрик (альтернатива встроенному)
├── internal/
│   ├── config/config.go     # флаги/env/дефолты → Config
│   ├── store/
│   │   ├── models.go        # User, UserDetailed, Project, Metric, константы ролей/статусов/квот, sentinel-ошибки
│   │   ├── store.go         # ИНТЕРФЕЙС Storage (контракт БД)
│   │   ├── factory.go       # NewStorage(driver, dsn) — выбор драйвера
│   │   ├── json.go          # JSON-драйвер (файлы, мутабельность через мьютекс)
│   │   ├── sql.go           # SQL-драйвер (sqlite+mysql, build tag db_sql)
│   │   └── sql_registry_{sql,nosql}.go  # подключение/заглушка SQL по build-тегу
│   ├── auth/auth.go         # JWT (подпись/парс), bcrypt, middleware (cookie/Bearer, роли), cookie-set/clear
│   ├── httpx/httpx.go       # WriteJSON/ReadJSON/Error/ErrStatus, UserFrom(r) (контекст)
│   ├── nginx/nginx.go       # ValidateDomain, генерация nginx.conf + certbot.sh + install.sh, ApplyCertbot
│   ├── metrics/sampler.go   # сбор метрик (размер папки, инкрементальный парс access.log)
│   └── handlers/
│       ├── handlers.go      # struct Handlers{cfg, store, sampler, web}, валидаторы, пути проектов, loadProject
│       ├── router.go        # ServeMux: все маршруты + withLogging + staticHandler (SPA)
│       ├── auth.go          # login/register/logout/me/changeOwnPassword + registrationEnabled
│       ├── projects.go      # CRUD проектов, upload zip, download, files, nginx-config, apply-ssl
│       ├── zip.go           # безопасная распаковка zip, hasIndex, mergeDir
│       ├── metrics.go       # GET /api/projects/{id}/metrics
│       └── admin.go         # пользователи, квоты, пароли, регистрация, stats
└── web/                     # фронтенд (embed в бинарь)
    ├── index.html
    ├── css/app.css
    └── js/{api.js, ui.js, app.js}
```

---

## 6. Архитектура

### Поток запроса

```
Browser ── HTTP ──> ServeMux (router.go)
                     ├─ /api/* → auth.Middleware → handler
                     │          (JWT из cookie fasthost_token или Authorization: Bearer,
                     │           user перезагружается из БД, роли проверяются)
                     └─ /      → staticHandler (web/, SPA fallback на index.html)
```

- Ключевое состояние сессии — `store.User` в `context` (через `httpx.SetUser/UserFrom`).
- Права: владелец проекта или admin (`Handlers.canAccess`), роли в middleware.

### Контракт БД `store.Storage`

Единый интерфейс для всех драйверов (`internal/store/store.go`):

```go
type Storage interface {
    Close() error
    CreateUser/GetUserByID/GetUserByUsername/GetUserDetailed/ListUsersDetailed
    UpdateUserPassword/UpdateUserQuota/CountUsers
    CreateProject/GetProjectByID/GetProjectByDomain/ListProjectsByUser/ListAllProjects
    UpdateProject/UpdateProjectDisk/DeleteProject
    AddMetric/LastMetric/ListMetrics
    GetSetting/SetSetting
}
```

Sentinel-ошибки (`store/models.go`): `ErrNotFound`, `ErrDuplicate`,
`ErrQuotaExceeded`; маппинг в HTTP — `httpx.ErrStatus` (404/409/413/500).

При добавлении метода в контракт нужно реализовать его **во всех драйверах**
(json + sql) — иначе компиляция сломается. Это защита от расхождения драйверов.

### Драйверы

| Драйвер | Файл | Как хранит | Особенности |
|---|---|---|---|
| `json` | `json.go` | `data/…/users.json`, `projects.json`, `settings.json`, `metrics/{pid}.json` | in-memory + write-through; мьютекс; метрики обрезаются до 500/проект; `next_id` хранится в файлах; `created_at` заполняется при создании, если пуст |
| `sqlite` | `sql.go` | файл БД | `SetMaxOpenConns(1)`, WAL + busy_timeout в DSN-прагмах |
| `mysql` | `sql.go` | сервер | `SetMaxOpenConns(20)`, DDL с `ENGINE=InnoDB utf8mb4` |

Диалектные различия (DDL, UPSERT в `SetSetting`, autoincrement) локализованы в
`sql.go` (`dialect`); вся бизнес-логика хендлеров от драйвера не зависит.

---

## 7. Модели данных

```go
type User struct {           // users
    ID, Username, Role, QuotaBytes, CreatedAt
    PasswordHash            // json:"-" — никогда не попадает в API-ответы
}
type UserDetailed struct {   // User + UsedBytes (сумма disk_bytes проектов), ProjectCount
    User
}
type Project struct {        // projects
    ID, UserID, Username (подставляется при листах), Name, Domain,
    Status ("active"|"error"), SSLEnabled, Path (абсолютный), DiskBytes, CreatedAt
}
type Metric struct {         // metrics
    ID, ProjectID, TS ("2006-01-02 15:04:05"), DiskBytes, RequestsCum, BandwidthCum
}
```

- `settings` — пары key/value (сейчас используется `allow_registration` и
  `default_quota` при регистрации).
- Квоты: `DefaultQuotaBytes = 5 ГБ`; валидный диапазон админа — `MinQuotaBytes`
  (100 МБ) … `MaxQuotaBytes` (100 ГБ).

---

## 8. HTTP API

Все ответы — JSON; ошибки — `{"error": "..."}`. Авторизация — cookie
`fasthost_token` (HttpOnly, SameSite=Lax, TTL 12 ч) или `Authorization: Bearer`.

| Метод | Путь | Доступ | Назначение |
|---|---|---|---|
| GET | `/api/health` | — | `ok` |
| POST | `/api/auth/login` | — | `{username,password}` → cookie + `{user}` |
| POST | `/api/auth/register` | — | `{username,password}` → создаёт user-а, cookie. 403, если регистрация отключена |
| POST | `/api/auth/logout` | любой | сброс cookie |
| GET | `/api/me` | любой | `{user, used_bytes, project_count}` |
| PUT | `/api/users/me/password` | любой | `{current_password,new_password}` |
| GET | `/api/projects` | любой | `{projects[], used_bytes, quota}` |
| POST | `/api/projects` | любой | `{name,domain}` → создаёт папку + `.conf/` + nginx/certbot |
| GET | `/api/projects/{id}` | владелец/admin | `{project}` |
| DELETE | `/api/projects/{id}` | владелец/admin | удаляет файлы + записи + метрики |
| POST | `/api/projects/{id}/upload` | владелец/admin | multipart `file` (zip). Квота + проверка `index.html` |
| GET | `/api/projects/{id}/download` | владелец/admin | zip файлов сайта (без `.conf`) |
| GET | `/api/projects/{id}/files` | владелец/admin | `{files:[{path,dir,size}]}` без `.conf` |
| GET | `/api/projects/{id}/metrics` | владелец/admin | `{disk_bytes, requests_cum, bandwidth_cum, series[]}` (последние 120) |
| GET | `/api/projects/{id}/nginx-config` | владелец/admin | `{config}` (текст `.conf/nginx.conf`) |
| POST | `/api/projects/{id}/apply-ssl` | владелец/admin | выполняет certbot (или инструкцию в тестовом режиме) |
| GET | `/api/admin/users` | admin | `{users: UserDetailed[]}` |
| GET | `/api/admin/users/{id}` | admin | `{user, projects[], data_dir}` |
| PUT | `/api/admin/users/{id}/password` | admin | `{new_password}` |
| PUT | `/api/admin/users/{id}/quota` | admin | `{quota_bytes}` (100 МБ–100 ГБ) |
| PUT | `/api/admin/registration` | admin | `{enabled}` — вкл/выкл регистрацию (сохраняется в БД) |
| GET | `/api/admin/stats` | admin | `{users, projects, total_disk, data_dir, registration_enabled, registration_flag_off}` |

Пути `{id}` в Go 1.22+ ServeMux: `r.PathValue("id")`.

---

## 9. Ключевые сценарии

### 9.1 Создание проекта (`POST /api/projects`)

1. Валидация: `name` — `^[a-z0-9][a-z0-9_-]{1,63}$` (латиница нижнего регистра,
   приводится `lowerASCII`); `domain` — `nginx.ValidateDomain` (hostname с точкой
   или `localhost`).
2. Проверка, что домен не занят (`GetProjectByDomain`).
3. Создание каталога `{dataDir}/users/{username}/projects/{name}/` и внутри
   `.conf/`:
   - `nginx.conf` — конфиг сервера (listen 80, `server_name`, root, try_files,
     access/error log в `.conf/`, блоки с комментариями по подключению и SSL);
   - `certbot.sh` — получение сертификата Let's Encrypt;
   - `install.sh` — копирование конфига в `/etc/nginx/sites-available|enabled`.
4. `CreateProject` (статус `active`).
5. Путь в БД — **абсолютный** (`dataDir` резолвится через `filepath.Abs`).

### 9.2 Загрузка ZIP (`POST /api/projects/{id}/upload`)

- Multipart-поле `file`; тело ограничено `http.MaxBytesReader` (2 ГБ).
- Квота: `used = Σ disk_bytes проектов пользователя`; `quotaLeft = quota − used`;
  распаковка не может превысить остаток.
- Извлечение **в staging** (`{проект}/.conf/.upload-staging`): проверка пути
  (`..`, абсолютные пути), лимит размера, затем `hasIndex` — обязателен
  `index.html` или `index.htm` в корне.
- Успех → `mergeDir` (перемещение файлов в корень проекта), пересчёт `dirSize`,
  `UpdateProjectDisk`. Ошибка → staging удаляется, файлы не трогаются.

### 9.3 Метрики

- **Сборщик**: встроенный goroutine в `main.go` (по умолчанию) или отдельный
  сервис `cmd/metricsd` (тот же контракт БД). Каждые `--metrics-interval` сек:
  - `dirSize` папки проекта (включая `.conf`);
  - инкрементальный парс `{проект}/.conf/access.log` (offset в памяти `Sampler`),
    извлечение `$body_bytes_sent` (regexp `"\s+\d{3}\s+(\d+)\s+"`); при
    ротации/усечении лога offset сбрасывается;
  - в БД пишется `Metric{disk_bytes, requests_cum, bandwidth_cum}` (кумулятивные
    счётчики берутся из `LastMetric`).
- **API**: `GET /api/projects/{id}/metrics` → последние 120 точек; фронтенд рисует
  «Диск, ГБ» и «Запросы/мин» (дельты между точками) canvas-чартами без библиотек.

### 9.4 SSL / certbot

- Кнопка «Применить SSL» → `POST .../apply-ssl` → `nginx.ApplyCertbot`.
- Без `--live-ssl`: возвращает инструкцию и пути к `.conf/certbot.sh` и
  `.conf/install.sh` (тестовый режим; `ssl_enabled` не ставится).
- С `--live-ssl`: выполняет `bash {проект}/.conf/certbot.sh` (nginx -t → reload →
  certbot --nginx). Ошибка → статус проекта `error`.

### 9.5 Регистрация (вкл/выкл)

Эффективное значение: `cfg.AllowRegistration && (setting отсутствует → true | иначе setting == "1")`.
Админ переключает `PUT /api/admin/registration` (пишет `allow_registration` в БД).
Флаг `--allow-registration=false` жёстко блокирует (`registration_flag_off` → в
админке переключатель заменяется на бейдж).

### 9.6 Права доступа

- `auth.Middleware(secret, store, roles...)`: валидный JWT + user существует.
- `Handlers.loadProject` → `canAccess`: `admin` или владелец; иначе 403.

---

## 10. Хранение файлов

```
{dataDir}/users/{username}/projects/{projectname}/
├── index.html …           ← файлы сайта (из zip)
└── .conf/                 ← системные файлы проекта
    ├── nginx.conf         ← конфиг nginx (генерируется при создании)
    ├── certbot.sh         ← SSL-скрипт (генерируется)
    ├── install.sh         ← установка конфига в систему (генерируется)
    ├── access.log         ← пишет nginx (для метрик)
    └── error.log
```

`.conf` скрыт от листинга файлов и от download-архива; в nginx-конфиге закрыт
`location ~ /\.conf { deny all; }`.

---

## 11. Фронтенд

SPA без сборщика; все файлы встраиваются в бинарь (`//go:embed all:web`).

| Файл | Роль |
|---|---|
| `web/index.html` | каркас: `#app`, `#modal-root`, `#toast-root`, подключение css/js, шрифты Google |
| `web/css/app.css` | дизайн-система (токены CSS), лейаут, таблицы, формы, модалки, чарты, тумблер |
| `web/js/api.js` | `api.get/post/put/del/upload` — fetch-обёртка (same-origin credentials, JSON, FormData, 401→throw) |
| `web/js/ui.js` | `h()` (DOM-хелпер), `ICONS` (inline SVG), `toast`, `openModal/confirmDialog`, `badge`, `progressBar`, `chart` (canvas), `breadcrumbs`, `formatBytes/formatDate/escapeHtml` |
| `web/js/app.js` | состояние `state{me,used,projectCount,view}`, `boot()`→`render()`+`refreshMe()`, hash-роутинг, все страницы |

### Маршруты (hash)

`#/` (дашборд) · `#/projects` · `#/projects/{id}` · `#/settings` ·
`#/admin/users` · `#/admin/users/{id}` · `#/register`

### Структура экрана

```
topbar (бренд, avatar, роль, «Выйти»)
sidebar (Панель / Проекты / Настройки / [admin] Пользователи)
content = breadcrumbs + страница
```

### ВАЖНО про `h()` (ui.js)

- Аттрибуты: `class`, `html` (innerHTML), `style`, `on*` (addEventListener),
  `dataset`, прочие → `setAttribute`. `false/null/undefined` пропускаются.
- Дети: массивы конкатенируются (1 уровень); **строки, начинающиеся с `<`, после
  `trimStart` вставляются как HTML** (так работают inline-SVG-иконки и `<span
  class="sort-arrow">`), остальное — текст.
- Не передавайте SVG-иконку конкатенацией с текстом в одну строку, если строка
  начинается с текста, а не с `<` — иконка станет текстом. Иконки начинаются с
  `<svg`, поэтому `ICONS.x + ' текст'` безопасно.

### Сортировка/фильтр таблиц

`state.view = {data, sortKey, sortDir, filter}` + перерисовка tbody. Заголовки
кликабельны (стрелка ▲/▼).

### Стиль (Enterprise)

- Палитра: фон `#F5F7FA`, поверхности `#FFFFFF`, текст `#1F2A37`, акцент
  `#2F6FED`, статусы `#12B76A`/`#F79009`/`#D92D20`.
- Сетка с шагом 8px; IBM Plex Sans; плотные таблицы, бейджи статусов, хлебные
  крошки, тосты, модалки.

---

## 12. Тестирование

```bash
# сборки (обе вариации)
go build ./...
go build -tags db_sql ./...

# статика
go vet ./... && go vet -tags db_sql ./...
node --check web/js/*.js

# ручной API-сценарий — см. «Тестовый сценарий» в разделе 3.
```

DOM-смоук: при разработке использовался node-харнес со стабами DOM/fetch
(прогон всех страниц, проверка отсутствия исключений и непустоты `main`).
Аналог можно воссоздать в `tmp`; в репозиторий он не закоммичен.

---

## 13. Подводные камни и особенности

1. **Build tag `db_sql`**: `sql.go` и `sql_registry_sql.go` собираются только с
   тегом. Без тега драйверы `sqlite`/`mysql` вернут ошибку с подсказкой. Добавляя
   зависимости SQL — проверяйте обе сборки.
2. **JSON-драйвер**: без транзакций, одиночный процесс, метрики обрезаются до 500
   точек на проект, файлы перезаписываются атомарно (tmp+rename). Мьютекс
   сериализует все операции. Не использовать на кластере.
3. **SQLite**: `SetMaxOpenConns(1)`; WAL и busy_timeout задаются в DSN.
   `SetSetting` ветвится по диалекту (`ON CONFLICT` vs `ON DUPLICATE KEY`).
4. **Пароли**: `PasswordHash` имеет `json:"-"` — в API никогда не утекает; в
   JSON-драйвере используется отдельная структура записи (`jsonUserRecord`) —
   поэтому хеши сохраняются на диск.
5. **SPA-статика**: `staticHandler` читает файл и отдаёт `http.ServeContent`;
   неизвестный путь → `index.html`. **Не менять на `http.FileServer`** — он делает
   301 для путей, оканчивающихся на `/index.html`.
6. **Время**: `ts`/`created_at` — строки `"2006-01-02 15:04:05"` (общие для всех
   драйверов). Драйверы заполняют `CreatedAt`, если пуст.
7. **Квоты**: `disk_bytes` проектов обновляет сборщик метрик; при загрузке
   используется сумма по БД (может отставать на один цикл сбора). Удаление
   проекта — `os.RemoveAll(path)` + удаление метрик.
8. **Секреты по умолчанию**: `admin/admin`, dev JWT-секрет — только для
   разработки; перед продом сменить (`--admin-pass`, `--secret`).
9. **nginx/certbot не установлены здесь**: генерация конфигов безопасна, но
   «Применить SSL» выполняет команды только с `--live-ssl`. Для домашнего теста —
   `install.sh` + `certbot.sh` из `.conf/` проекта (README раздел «Тестирование
   nginx + SSL дома»).
10. **Ограничения валидации**: имя проекта 2–64 символа (a-z0-9_-), логин 3–32
    (латиница/цифры/_ ), пароль ≥6, домен — hostname с точкой или `localhost`,
    квота 100 МБ–100 ГБ.

---

## 14. Идеи для развития

- CPU/RAM-метрики (требуется запуск процессов или node_exporter).
- Резервное копирование/снапшоты проектов, ротация access.log.
- Прогресс-бар загрузки ZIP, докачка/рестарт при больших файлах.
- CSRF-токен (сейчас митигируется SameSite=Lax), refresh-токены, 2FA.
- Изменение роли пользователя из админки, удаление пользователя (каскад по проектам).
- Лимит количества проектов, лимит на один файл в ZIP.
- Экспорт метрик (CSV/JSON), ретеншн настраиваемый.
- Международная версия (i18n: сейчас интерфейс на русском).