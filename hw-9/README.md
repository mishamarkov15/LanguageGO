# ДЗ 9. Параллельный отчёт по расходам

Gateway принимает HTTP-запросы, а Ledger хранит расходы и бюджеты в PostgreSQL. При первом запросе отчёта Ledger параллельно считает сумму для каждой категории. Redis кеширует результат на 30 секунд. Добавление расхода сбрасывает кеш отчётов.

## Запуск

Нужны Go 1.23+, PostgreSQL, Redis и goose. [Подготовка базы и Redis](../hw-7/ledger/README.md#postgresql-и-redis).

Из корня репозитория выполните команды в одном терминале.

```sh
cd hw-9
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable'
export REDIS_ADDR=localhost:6379
goose -dir ledger/migrations postgres "$DATABASE_URL" up
cd gateway
go run .
```

## Отчёт за период

`GET /api/reports/summary` принимает обязательные даты `from` и `to` в формате `YYYY-MM-DD`. Обе границы входят в период. На пустой базе выполните в другом терминале:

```sh
curl -sS localhost:8080/api/transactions -H 'Content-Type: application/json' \
  -d '{"amount":450,"category":"еда","date":"2026-09-01"}'
curl -sS localhost:8080/api/transactions -H 'Content-Type: application/json' \
  -d '{"amount":65.5,"category":"транспорт","date":"2026-09-30"}'
curl -sS 'localhost:8080/api/reports/summary?from=2026-09-01&to=2026-09-30'
```

Ответ отчёта (`200 OK`):

```json
{"еда":450,"транспорт":65.5}
```

Для периода без расходов ответом будет `{}`. Если дата пропущена, имеет неверный формат или начало позже конца, Gateway возвращает `400 Bad Request` с описанием ошибки.

## Таймауты запросов

По умолчанию на каждый запрос отводится **2 секунды**. Чтобы проверить другой таймаут, остановите Gateway и запустите его из `hw-9/gateway` командой `REQUEST_TIMEOUT=50ms go run .`. Следующий запуск обычной командой `go run .` вернёт значение по умолчанию.

При превышении таймаута Gateway возвращает `504 Gateway Timeout` и `{"error":"request canceled or timed out"}`. Ledger отменяет SQL-запросы. Пока идёт расчёт, каждые 400 мс в журнале появляется сообщение о ходе работы, а при завершении или отмене — об остановке heartbeat.

Чтобы воспроизвести таймаут на тестовой базе, откройте `psql "$DATABASE_URL"`, выполните `BEGIN;` и `LOCK TABLE expenses IN ACCESS EXCLUSIVE MODE;`. Оставив транзакцию открытой, запросите отчёт за период, которого ещё нет в кеше. После ответа `504` выполните в psql `ROLLBACK;` и повторите тот же запрос.

## Тесты

Из корня репозитория:

```sh
(cd hw-9/ledger && go test -race ./...)
(cd hw-9/gateway && go test -race ./...)
```

Для интеграционных тестов PostgreSQL укажите адрес тестовой базы в `TEST_DATABASE_URL`. Тесты создают и удаляют свои схемы.
