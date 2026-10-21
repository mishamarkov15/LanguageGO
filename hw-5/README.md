# Домашнее задание 5

REST API в сервисе Gateway. Gateway импортирует модуль `ledger` из этой же папки
(через `require` и `replace ../ledger` в `gateway/go.mod`) и вызывает его функции.
Данные, как и раньше, хранятся только в памяти и пропадают после остановки сервера.

- `ledger` — библиотека: транзакции, бюджеты, валидация, `ListBudgets()` и ошибка
  `ErrBudgetExceeded` для превышения лимита.
- `gateway/internal/api` — HTTP-слой: DTO и преобразователи в доменные модели,
  обработчики, маршруты с префиксом `/api` и middleware.

Требуется Go 1.22 или новее. Запуск из корня репозитория:

```sh
cd hw-5/gateway
go run .
```

Сервер слушает порт `8080`. Для остановки нажмите `Ctrl+C`. В консоль для каждого
запроса пишется метод, путь и длительность обработки.

## Эндпоинты

| Метод  | Путь                | Описание                                   | Успех           |
|--------|---------------------|--------------------------------------------|-----------------|
| `POST` | `/api/transactions` | Добавить транзакцию                        | `201 Created`   |
| `GET`  | `/api/transactions` | Список транзакций (`[]`, если их нет)      | `200 OK`        |
| `POST` | `/api/budgets`      | Создать или обновить бюджет категории      | `201 Created`   |
| `GET`  | `/api/budgets`      | Список бюджетов (`[]`, если их нет)        | `200 OK`        |

Тело транзакции: `amount` (число > 0), `category` (непустая строка),
`description` (строка), `date` (`ГГГГ-ММ-ДД`). Тело бюджета: `category`
(непустая строка), `limit` (число > 0). Транзакция в категории без бюджета
принимается без ограничений.

Все ответы имеют заголовок `Content-Type: application/json; charset=utf-8`.
Ошибки возвращаются в формате `{"error":"описание"}`:

| Статус | Когда                                                              |
|--------|--------------------------------------------------------------------|
| `400`  | Некорректный JSON, неизвестное поле или ошибка валидации           |
| `404`  | Неизвестный путь                                                   |
| `405`  | Неподдерживаемый метод                                             |
| `409`  | Превышение бюджета: `{"error":"budget exceeded"}`                  |
| `500`  | Прочие внутренние ошибки                                           |

## Примеры cURL

Команды выполняются в другом терминале при запущенном сервере, по порядку.

Создать бюджет — `201 Created`:

```sh
curl -i -X POST http://localhost:8080/api/budgets \
  -H "Content-Type: application/json" \
  -d '{"category":"еда","limit":1000}'
```

Добавить транзакцию — `201 Created`, в ответе транзакция с `id`:

```sh
curl -i -X POST http://localhost:8080/api/transactions \
  -H "Content-Type: application/json" \
  -d '{"amount":450,"category":"еда","description":"ланч","date":"2025-09-10"}'
```

Превысить бюджет (450 + 600 > 1000) — `409 Conflict`:

```sh
curl -i -X POST http://localhost:8080/api/transactions \
  -H "Content-Type: application/json" \
  -d '{"amount":600,"category":"еда","description":"ужин","date":"2025-09-10"}'
```

Ошибка валидации — `400 Bad Request`:

```sh
curl -i -X POST http://localhost:8080/api/transactions \
  -H "Content-Type: application/json" \
  -d '{"amount":-5,"category":"еда","description":"возврат","date":"2025-09-10"}'
```

Списки транзакций и бюджетов — `200 OK`:

```sh
curl -i http://localhost:8080/api/transactions
curl -i http://localhost:8080/api/budgets
```
