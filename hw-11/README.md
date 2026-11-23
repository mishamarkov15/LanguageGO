# ДЗ 11. Запуск Gateway в Docker

Gateway принимает расходы и бюджеты по HTTP. Библиотека Ledger сохраняет данные в PostgreSQL и кеширует отчёты в Redis. Для Gateway и Ledger есть отдельные двухэтапные Dockerfile. В Compose запускаются три сервиса — `db`, `redis` и `gateway`.

## Запуск

Понадобятся Docker с Compose и утилита Goose (`brew install goose` на macOS). Все команды выполняются **из корня LanguageGO**, где лежит [docker-compose.yml](../docker-compose.yml).

1. Запустите PostgreSQL и дождитесь статуса `healthy`. Если порт 5432 занят, сначала выполните `export DB_PORT=55432` в том же терминале.

   ```sh
   docker compose up -d --wait db
   ```

2. Примените миграции с хоста через опубликованный порт.

   ```sh
   export DATABASE_URL="postgres://cashcraft:cashcraft@127.0.0.1:${DB_PORT:-5432}/cashcraft?sslmode=disable"
   goose -dir hw-11/ledger/migrations postgres "$DATABASE_URL" up
   ```

3. Соберите и запустите Gateway. Compose запустит Redis и дождётся готовности обеих зависимостей. Внутри сети Gateway обращается к `db:5432` и `redis:6379`.

   ```sh
   docker compose up -d --build --wait gateway
   docker compose ps
   ```

## Проверка HTTP

С хоста и из контейнера оба запроса должны вернуть `pong`.

```sh
curl -fsS http://localhost:8080/ping
docker compose exec -T gateway wget -qO- http://127.0.0.1:8080/ping
```

Пример для пустой базы. Последний запрос вернёт `{"еда":150,"транспорт":25}`.

```sh
api=http://localhost:8080/api
curl -fsS "$api/budgets" -H 'Content-Type: application/json' -d '{"category":"еда","limit":500}'
curl -fsS "$api/budgets"

curl -fsS "$api/transactions" -H 'Content-Type: application/json' \
  -d '{"amount":150,"category":"еда","date":"2026-11-01"}'
curl -fsS "$api/transactions/bulk?workers=2" -H 'Content-Type: application/json' \
  -d '[{"amount":25,"category":"транспорт","date":"2026-11-02"}]'
curl -fsS "$api/transactions"
curl -fsS "$api/reports/summary?from=2026-11-01&to=2026-11-30"
```

## Сборка образов вручную

Gateway включает локальный модуль Ledger, поэтому контекст сборки Gateway — вся папка `hw-11`. При работающем стенде можно запустить второй экземпляр Gateway на порту 8081. Для проверки из другого терминала выполните `curl -fsS http://localhost:8081/ping`.

```sh
docker build -t hw11-ledger hw-11/ledger
docker build -t hw11-gateway -f hw-11/gateway/Dockerfile hw-11
docker run --rm --network languagego-hw11_default -p 127.0.0.1:8081:8080 \
  -e 'DATABASE_URL=postgres://cashcraft:cashcraft@db:5432/cashcraft?sslmode=disable' \
  -e REDIS_ADDR=redis:6379 hw11-gateway
```

Чтобы остановить второй экземпляр, нажмите `Ctrl+C`. Команда `docker compose down` остановит стенд. Данные PostgreSQL сохраняются в именованном томе `pgdata`.
