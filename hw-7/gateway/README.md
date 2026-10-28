# Gateway

HTTP API на порту `8080`. Эндпоинты и формат JSON те же, что в ДЗ 5–6, но
теперь Ledger хранит данные в PostgreSQL, а отчёты кеширует в Redis. Gateway
об этом не знает: он вызывает те же функции Ledger.

## Запуск

Перед запуском поднимите PostgreSQL и Redis и примените миграции — см.
[ledger/README.md](../ledger/README.md). Затем из корня репозитория:

```sh
cd hw-7/gateway
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable"
export REDIS_ADDR=localhost:6379 REDIS_DB=0
go run .
```

`REDIS_PASSWORD` задаётся, если у Redis есть пароль. Вместо `DATABASE_URL` можно
задать `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`. В логе при старте:

```
PostgreSQL: подключение установлено (postgres@localhost:5432/cashapp)
Redis: подключение установлено (localhost:6379, db 0)
Ledger started
Gateway listening on :8080
```

`Ctrl+C` останавливает сервер и закрывает соединения с базой и Redis.

## Эндпоинты

| Метод  | Путь                                         | Описание                                  | Успех         |
|--------|----------------------------------------------|-------------------------------------------|---------------|
| `POST` | `/api/budgets`                               | Создать или обновить бюджет категории     | `201 Created` |
| `GET`  | `/api/budgets`                               | Список бюджетов                           | `200 OK`      |
| `POST` | `/api/transactions`                          | Добавить трату                            | `201 Created` |
| `GET`  | `/api/transactions`                          | Список трат, сначала новые                | `200 OK`      |
| `GET`  | `/api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD` | Траты по категориям за период (включительно) | `200 OK` |

Ошибки — `{"error":"..."}` с заголовком `Content-Type: application/json; charset=utf-8`:
`400` — некорректный JSON, данные или период, `404` — неизвестный путь,
`405` — неподдерживаемый метод, `409` — `{"error":"budget exceeded"}`,
`500` — ошибка базы данных (детали только в логе сервера).

## Проверка через cURL

Команды выполняются в другом терминале при запущенном сервере, по порядку.
`psql` и `redis-cli` показывают, что данные действительно лежат в PostgreSQL и Redis.

Бюджет — `201`, запись в таблице `budgets`:

```sh
curl -i -X POST http://localhost:8080/api/budgets \
  -H "Content-Type: application/json" \
  -d '{"category":"еда","limit":5000}'
psql "$DATABASE_URL" -c "SELECT * FROM budgets"
```

Трата в пределах лимита — `201`, запись в таблице `expenses`:

```sh
curl -i -X POST http://localhost:8080/api/transactions \
  -H "Content-Type: application/json" \
  -d '{"amount":450,"category":"еда","description":"ланч","date":"2025-09-10"}'
psql "$DATABASE_URL" -c "SELECT * FROM expenses"
```

Превышение лимита (450 + 4600 > 5000) — `409`, в `expenses` новой строки нет:

```sh
curl -i -X POST http://localhost:8080/api/transactions \
  -H "Content-Type: application/json" \
  -d '{"amount":4600,"category":"еда","description":"техника","date":"2025-09-12"}'
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM expenses"
```

Списки из БД — `200`:

```sh
curl -i http://localhost:8080/api/transactions
curl -i http://localhost:8080/api/budgets
```

Отчёт дважды подряд — оба ответа одинаковые, но в логе сервера первый запрос
отмечен как `cache miss` (отчёт посчитан в БД), второй — как `cache hit`
(взят из Redis). Ключ живёт 30 секунд:

```sh
curl -i "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30"
curl -i "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30"
redis-cli TTL report:summary:2025-09-01:2025-09-30
```

Ответ:

```json
{"from":"2025-09-01","to":"2025-09-30","total":450,"categories":[{"category":"еда","total":450}]}
```

Данные переживают перезапуск: остановите сервер (`Ctrl+C`), снова выполните
`go run .` и повторите `GET /api/transactions` и `GET /api/budgets` — в ответах
те же записи.

## Тесты

Интеграционные тесты в `internal/api/handlers_test.go` поднимают роутер через
`httptest.NewServer` и подключают Ledger к тестовой базе (как её подготовить —
в [ledger/README.md](../ledger/README.md#тесты)) и к miniredis. Из папки `hw-7/gateway`:

```sh
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/cashapp_test?sslmode=disable"
go test ./... -cover -coverprofile=cover.out
go tool cover -html=cover.out -o cover.html
```

Проверяются все эндпоинты, сохранение в БД, коды 400, 409 и 500, кеш отчёта и
списка бюджетов. Без `TEST_DATABASE_URL` выполняются только тесты маршрутизации
и параметров отчёта. Покрытие `internal/api` — 99%, модуля вместе с `main.go` — 80.3%.
