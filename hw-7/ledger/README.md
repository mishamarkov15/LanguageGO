# Ledger

Бизнес-логика учёта трат. Транзакции и бюджеты хранятся в PostgreSQL, отчёты и
список бюджетов кешируются в Redis на короткий TTL. Ledger — библиотека: её
импортирует Gateway и при старте вызывает `ledger.Init`.

## PostgreSQL и Redis

Через Docker:

```sh
docker run --name cashcraft-postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=cashapp -p 5432:5432 -d postgres:16-alpine
docker run --name cashcraft-redis -p 6379:6379 -d redis:7-alpine
```

Или локально (macOS, Homebrew):

```sh
brew install postgresql@16 redis
brew services start postgresql@16
brew services start redis
createdb cashapp
```

## Миграции

Схема создаётся утилитой [goose](https://github.com/pressly/goose). Установка:

```sh
go install github.com/pressly/goose/v3/cmd/goose@v3.26.0
```

(или `brew install goose`). Миграции лежат в `ledger/migrations/`, у каждой есть
секции `-- +goose Up` и `-- +goose Down`:

- `00001_create_budgets.sql` — `budgets`: `id`, уникальная `category`,
  `limit_amount NUMERIC(14,2) > 0`;
- `00002_create_expenses.sql` — `expenses`: `id`, `amount NUMERIC(14,2) <> 0`,
  `category`, `description`, `date`, индекс по `(category, date)`.

Применяйте миграции **до** первого запуска и после каждого изменения схемы.
Команды из папки `hw-7`:

```sh
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable"
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
goose -dir ./ledger/migrations postgres "$DATABASE_URL" status
```

Откат последней миграции и всех миграций:

```sh
goose -dir ./ledger/migrations postgres "$DATABASE_URL" down
goose -dir ./ledger/migrations postgres "$DATABASE_URL" reset
```

## Подключение

Настройки читаются из переменных окружения:

| Переменная       | По умолчанию     | Описание                                   |
|------------------|------------------|--------------------------------------------|
| `DATABASE_URL`   | —                | Строка подключения к PostgreSQL            |
| `DB_HOST`        | `localhost`      | Хост PostgreSQL                            |
| `DB_PORT`        | `5432`           | Порт PostgreSQL                            |
| `DB_USER`        | `postgres`       | Пользователь PostgreSQL                    |
| `DB_PASS`        | `postgres`       | Пароль PostgreSQL                          |
| `DB_NAME`        | `cashapp`        | Имя базы данных                            |
| `REDIS_ADDR`     | `localhost:6379` | Адрес Redis                                |
| `REDIS_DB`       | `0`              | Номер базы Redis                           |
| `REDIS_PASSWORD` | —                | Пароль Redis, необязательный               |

`DATABASE_URL` имеет приоритет. Если она не задана, строка подключения
собирается из `DB_*`: `postgres://DB_USER:DB_PASS@DB_HOST:DB_PORT/DB_NAME?sslmode=disable`.

`Init` открывает пул через `sql.Open("pgx", dsn)` (`MaxOpenConns` 10,
`MaxIdleConns` 5, `ConnMaxLifetime` 30 минут), проверяет его через `Ping`, затем
подключается к Redis и проверяет его через `PING`. При успехе в лог пишется:

```
PostgreSQL: подключение установлено (postgres@localhost:5432/cashapp)
Redis: подключение установлено (localhost:6379, db 0)
```

Если PostgreSQL или Redis недоступны, Gateway выводит причину и завершается с кодом 1.

## Бизнес-правила

- `SetBudget` — `INSERT … ON CONFLICT (category) DO UPDATE`: бюджет категории
  создаётся или обновляется без дублей.
- `ListBudgets` — бюджеты, отсортированные по категории.
- `AddTransaction` — в транзакции БД: читает лимит категории
  `SELECT … FOR UPDATE`, считает уже потраченное `SUM(amount)`, и если
  `потрачено + сумма > лимит`, возвращает `ErrBudgetExceeded` без вставки.
  Иначе выполняет `INSERT … RETURNING id`. Блокировка строки бюджета не даёт
  параллельным запросам вместе превысить лимит. Категории без бюджета не
  ограничены, как и раньше.
- `ListTransactions` — траты от новых к старым (`ORDER BY date DESC, id DESC`).
- Перед записью всегда вызывается `Validate()`. Суммы и лимиты должны быть
  положительными, не больше `999999999999.99` и не больше чем с двумя знаками
  после запятой (иначе их нельзя сохранить в `NUMERIC(14,2)` без округления).
  Сравнение с лимитом идёт в копейках, поэтому `0.1 + 0.2` укладывается в
  лимит `0.3`.

## Кеш в Redis

| Ключ                              | TTL | Что хранит                       | Когда сбрасывается          |
|-----------------------------------|-----|----------------------------------|-----------------------------|
| `report:summary:<from>:<to>`      | 30с | JSON отчёта `GetReportSummary`   | при `AddTransaction`        |
| `budgets:all`                     | 30с | JSON списка `ListBudgets`        | при `SetBudget` (`DEL`)     |

`GetReportSummary(ctx, from, to)` сначала ищет отчёт в Redis (cache hit), а если
его нет — считает `SUM(amount) … GROUP BY category` за период в БД и сохраняет
результат с TTL (cache miss → set). Оба случая видны в логе:

```
ledger: report:summary:2025-09-01:2025-09-30 — cache miss, отчёт посчитан в БД и сохранён на 30s
ledger: report:summary:2025-09-01:2025-09-30 — cache hit
```

Кеш прозрачен для Gateway. Если Redis перестанет отвечать во время работы,
ошибки кеша только пишутся в лог, а данные читаются из PostgreSQL.

## Тесты

Тесты, которые обращаются к PostgreSQL, очищают таблицы, поэтому им нужна
отдельная база. Создайте её и примените миграции:

```sh
createdb cashapp_test
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/cashapp_test?sslmode=disable"
goose -dir ./ledger/migrations postgres "$TEST_DATABASE_URL" up
```

Запуск из папки `hw-7/ledger`:

```sh
go test ./... -cover -coverprofile=cover.out
go tool cover -html=cover.out -o cover.html
```

Без `TEST_DATABASE_URL` выполняются только тесты валидации, DSN и кеша, а тесты
с БД пропускаются. Redis в тестах заменён на miniredis, отдельный сервер не нужен.
Покрытие пакетов `ledger` ~94%: `ledger` — 92.5%, `internal/db` — 96%,
`internal/cache` — 100%.
