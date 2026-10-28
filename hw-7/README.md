# Домашнее задание 7

Ledger больше не хранит данные в памяти: транзакции и бюджеты лежат в PostgreSQL
(таблицы `expenses` и `budgets`, схема создаётся миграциями goose), а отчёты
кешируются в Redis на 30 секунд. Бизнес-правила прежние: валидация данных и
запрет превышения бюджета.

Gateway обращается к тем же HTTP-эндпоинтам, что и в ДЗ 5–6, и с теми же
контрактами — меняется только место хранения данных внутри Ledger. Добавлен
эндпоинт `GET /api/reports/summary` для отчёта по тратам за период.

Ledger — Go-библиотека, которую импортирует Gateway, поэтому оба работают в одном
процессе: при старте Gateway вызывает `ledger.Init`, Ledger подключается к
PostgreSQL и Redis и пишет об этом в лог. «Перезапустить Ledger» — значит
перезапустить Gateway; данные при этом сохраняются в базе.

Требуется Go 1.23 или новее, PostgreSQL и Redis.

## Порядок запуска

1. Запустите PostgreSQL и Redis, создайте базу `cashapp` —
   [ledger/README.md](ledger/README.md#postgresql-и-redis).
2. Примените миграции goose — [ledger/README.md](ledger/README.md#миграции).
3. Запустите Gateway вместе с Ledger — [gateway/README.md](gateway/README.md#запуск).
4. Проверьте API через cURL — [gateway/README.md](gateway/README.md#проверка-через-curl).

Кратко, из корня репозитория:

```sh
cd hw-7
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable"
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
cd gateway
REDIS_ADDR=localhost:6379 go run .
```

## Структура

```
hw-7/
├── ledger/
│   ├── migrations/        SQL-миграции goose: budgets и expenses
│   ├── internal/db/       подключение к PostgreSQL: DSN из env, пул, Ping
│   ├── internal/cache/    кеш в Redis: JSON-значения с TTL
│   ├── ledger.go          Init, Close, Reset
│   ├── budget.go          SetBudget (UPSERT), ListBudgets (кеш budgets:all)
│   ├── transaction.go     AddTransaction (проверка лимита), ListTransactions
│   └── report.go          GetReportSummary (кеш report:summary:<from>:<to>)
└── gateway/
    ├── main.go            запуск Ledger и HTTP-сервера на :8080
    └── internal/api/      HTTP-слой из ДЗ 5 + отчёт
```

## Тесты

Тесты с PostgreSQL запускаются на отдельной базе из `TEST_DATABASE_URL` (без
неё они пропускаются), вместо настоящего Redis используется
[miniredis](https://github.com/alicebob/miniredis). Команды — в README сервисов.

Покрытие тестами ~89% по обоим модулям: `ledger` — 94.2%,
`gateway` — 80.3% (`internal/api` — 99%, `main.go` тестами не покрыт).
