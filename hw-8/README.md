# Домашнее задание 8

Ledger разделён на слои, Gateway получает готовый сервис через интерфейс.
HTTP API, PostgreSQL, кеш Redis и правила проверки бюджетов сохранены из ДЗ 7.

## Структура

```text
ledger/
  ledger.go                 публичные типы, интерфейс Service и фабрика New
  internal/domain/          сущности, валидация, интерфейсы репозиториев и кеша
  internal/service/         бизнес-правила и кеширование через интерфейсы
  internal/repository/pg/   SQL-запросы и транзакции PostgreSQL
  internal/db/              настройки и подключение PostgreSQL
  internal/cache/           реализация кеша Redis
  internal/app/             чтение окружения и сборка зависимостей
  migrations/               миграции goose из ДЗ 7
gateway/
  main.go                   инициализация Ledger и запуск HTTP-сервера
  internal/api/             обработчики с внедрённым Service, DTO и middleware
```

Домен не зависит от хранения и HTTP. Сервис использует доменные интерфейсы,
реализации репозиториев зависят от домена, а фабрика связывает их вместе.
Проверка бюджета и добавление расхода выполняются в одной транзакции БД
с блокировкой строки бюджета, как в ДЗ 7.

В Gateway вызов `ledger.New(ctx)` возвращает `(service, closeFn, err)`.
После проверки ошибки вызывается `defer closeFn()`, а сервис передаётся
в `api.NewRouter(service)`. Подключения проверяются с переданным контекстом;
Gateway не импортирует SQL-драйверы или репозитории. Фабрика не запускает миграции.

## Запуск

Требуются Go 1.23+, PostgreSQL, Redis и goose. Подготовка PostgreSQL и Redis
описана в [ДЗ 7](../hw-7/ledger/README.md#postgresql-и-redis).
Из корня репозитория:

```sh
cd hw-8
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable'
export REDIS_ADDR=localhost:6379 REDIS_DB=0
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
cd gateway
go run .
```

Сервер доступен на `http://localhost:8080`, остановка — `Ctrl+C`.
Вместо `DATABASE_URL` можно задать `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`,
`DB_NAME`; пароль Redis задаётся через `REDIS_PASSWORD`.

## Проверка API

В другом терминале, на пустой базе, по порядку:

```sh
curl -i -X POST localhost:8080/api/budgets -H 'Content-Type: application/json' \
  -d '{"category":"еда","limit":5000}'
curl -i -X POST localhost:8080/api/transactions -H 'Content-Type: application/json' \
  -d '{"amount":450,"category":"еда","description":"ланч","date":"2025-09-10"}'
curl -i -X POST localhost:8080/api/transactions -H 'Content-Type: application/json' \
  -d '{"amount":4600,"category":"еда","date":"2025-09-12"}'
curl -i localhost:8080/api/budgets
curl -i localhost:8080/api/transactions
curl -i 'localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30'
```

Первые два запроса возвращают `201`, третий — `409` с
`{"error":"budget exceeded"}`, остальные — `200`. Неверные данные дают `400`,
ошибки хранения — `500` с `{"error":"internal server error"}`.

## Тесты

Из корня репозитория:

```sh
(cd hw-8/ledger && go test -race ./...)
(cd hw-8/gateway && go test -race ./...)
```

Домен, сервис и HTTP-обработчики проверяются без PostgreSQL. Для интеграционных
тестов PostgreSQL задайте `TEST_DATABASE_URL` отдельной тестовой базы;
тесты создают и удаляют собственные схемы. Redis в тестах заменён на miniredis.
