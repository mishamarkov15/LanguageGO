# ДЗ 10. Пакетный импорт транзакций

Gateway принимает расходы одним запросом. В Ledger пул воркеров обрабатывает их параллельно. Каждый расход проходит валидацию и проверку бюджета и сохраняется в отдельной транзакции PostgreSQL.

## Запуск

Нужны Go 1.23+, PostgreSQL, Redis и goose. [Подготовка PostgreSQL и Redis](../hw-7/ledger/README.md#postgresql-и-redis).

Из корня репозитория:

```sh
cd hw-10
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable'
export REDIS_ADDR=localhost:6379
goose -dir ledger/migrations postgres "$DATABASE_URL" up
cd gateway
go run .
```

## Импорт

`POST /api/transactions/bulk` принимает JSON-массив. У каждого расхода обязательны `amount` (положительная сумма с точностью до копеек), `category` и `date` в формате `YYYY-MM-DD`. Поле `description` необязательно. Параметр `workers` задаёт число воркеров от 1 до 32, по умолчанию 4. При общем бюджете порядок обработки может влиять на то, какие расходы пройдут проверку.

Пример на пустой базе — установить бюджет 500 рублей и отправить три расхода:

```sh
curl -sS localhost:8080/api/budgets -H 'Content-Type: application/json' \
  -d '{"category":"еда","limit":500}'

curl -sS 'localhost:8080/api/transactions/bulk?workers=4' -H 'Content-Type: application/json' \
  -d '[
    {"amount":150,"category":"еда","date":"2026-11-01","description":"обед"},
    {"amount":600,"category":"еда","date":"2026-11-01"},
    {"amount":25,"category":"транспорт","date":"2026-11-02"}
  ]'
```

Ответ `200 OK`. Поля `accepted` и `rejected` показывают количество сохранённых и отклонённых записей. Ошибки упорядочены по индексу во входном массиве, начиная с нуля.

```json
{"accepted":2,"rejected":1,"errors":[{"index":1,"error":"budget exceeded"}]}
```

Пустой массив возвращает `{"accepted":0,"rejected":0,"errors":[]}`. Некорректное тело запроса или значение `workers` даёт `400`. При истечении `REQUEST_TIMEOUT` (по умолчанию 2 секунды) Gateway возвращает `504` с `{"error":"request canceled or timed out"}`. Уже сохранённые расходы остаются в БД, их можно проверить командой `curl -sS localhost:8080/api/transactions`. Повторная отправка создаст их ещё раз.

## Тесты

Из корня репозитория выполните:

```sh
(cd hw-10/ledger && go test -race ./...)
(cd hw-10/gateway && go test -race ./...)
```

Чтобы включить интеграционные тесты бюджета при параллельном импорте, задайте `TEST_DATABASE_URL` для тестовой базы PostgreSQL. Тесты создают и удаляют собственные схемы.
