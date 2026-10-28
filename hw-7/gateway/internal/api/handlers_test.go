package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-7/gateway/internal/api"
	"hw-7/ledger"
)

const jsonContentType = "application/json; charset=utf-8"

func TestMain(m *testing.M) {
	// Логи middleware и ledger не нужны в отчёте тестов.
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// newServer поднимает тестовый HTTP-сервер на роутере Gateway. Ledger
// подключается к тестовой базе из TEST_DATABASE_URL и к Redis в памяти
// (miniredis), данные очищаются через ledger.Reset(). Все серверы работают
// с одной базой, поэтому интеграционные тесты не помечаются t.Parallel().
func newServer(t *testing.T) (*httptest.Server, *miniredis.Miniredis) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задана: тест с PostgreSQL пропущен")
	}
	mr := miniredis.RunT(t)
	ctx := context.Background()

	require.NoError(t, ledger.Init(ctx, ledger.Config{DatabaseURL: dsn, RedisAddr: mr.Addr()}))
	t.Cleanup(func() { ledger.Close() })
	require.NoError(t, ledger.Reset(ctx))

	server := httptest.NewServer(api.NewRouter())
	t.Cleanup(server.Close)
	return server, mr
}

// newRouterServer поднимает сервер без подключения ledger — для запросов,
// которые отклоняются до обращения к базе (маршрутизация, параметры).
func newRouterServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(api.NewRouter())
	t.Cleanup(server.Close)
	return server
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func send(t *testing.T, server *httptest.Server, method, path, body string) response {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return response{status: resp.StatusCode, header: resp.Header, body: data}
}

// assertJSON проверяет статус и заголовок Content-Type, затем разбирает тело в T.
func assertJSON[T any](t *testing.T, res response, wantStatus int) T {
	t.Helper()
	assert.Equal(t, wantStatus, res.status, "тело ответа: %s", res.body)
	assert.Equal(t, jsonContentType, res.header.Get("Content-Type"))

	var v T
	require.NoError(t, json.Unmarshal(res.body, &v), "тело ответа: %s", res.body)
	return v
}

func assertError(t *testing.T, res response, wantStatus int) string {
	t.Helper()
	body := assertJSON[api.ErrorResponse](t, res, wantStatus)
	assert.NotEmpty(t, body.Error)
	return body.Error
}

func createBudget(t *testing.T, server *httptest.Server, body string) {
	t.Helper()
	assertJSON[api.BudgetResponse](t, send(t, server, http.MethodPost, "/api/budgets", body), http.StatusCreated)
}

func createTransaction(t *testing.T, server *httptest.Server, body string) api.TransactionResponse {
	t.Helper()
	return assertJSON[api.TransactionResponse](t,
		send(t, server, http.MethodPost, "/api/transactions", body), http.StatusCreated)
}

func TestBudgetsAPI(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		server, _ := newServer(t)

		res := send(t, server, http.MethodPost, "/api/budgets", `{"category":"еда","limit":5000}`)

		got := assertJSON[api.BudgetResponse](t, res, http.StatusCreated)
		assert.Equal(t, api.BudgetResponse{Category: "еда", Limit: 5000}, got)
		assert.JSONEq(t, `{"category":"еда","limit":5000}`, string(res.body))
	})

	t.Run("list_contains_created", func(t *testing.T) {
		server, _ := newServer(t)
		createBudget(t, server, `{"category":"еда","limit":5000}`)

		res := send(t, server, http.MethodGet, "/api/budgets", "")

		got := assertJSON[[]api.BudgetResponse](t, res, http.StatusOK)
		assert.Equal(t, []api.BudgetResponse{{Category: "еда", Limit: 5000}}, got)
	})

	t.Run("list_empty", func(t *testing.T) {
		server, _ := newServer(t)

		res := send(t, server, http.MethodGet, "/api/budgets", "")

		assertJSON[[]api.BudgetResponse](t, res, http.StatusOK)
		assert.JSONEq(t, `[]`, string(res.body))
	})

	t.Run("update", func(t *testing.T) {
		server, _ := newServer(t)
		createBudget(t, server, `{"category":"еда","limit":5000}`)
		send(t, server, http.MethodGet, "/api/budgets", "")

		createBudget(t, server, `{"category":"еда","limit":7000.5}`)

		got := assertJSON[[]api.BudgetResponse](t, send(t, server, http.MethodGet, "/api/budgets", ""), http.StatusOK)
		assert.Equal(t, []api.BudgetResponse{{Category: "еда", Limit: 7000.5}}, got,
			"обновление бюджета должно сбрасывать кеш списка")
	})

	badRequests := []struct {
		name string
		body string
	}{
		{name: "bad_json", body: `{"category":"еда",`},
		{name: "empty_body", body: ``},
		{name: "wrong_type", body: `{"category":"еда","limit":"5000"}`},
		{name: "unknown_field", body: `{"category":"еда","limit":5000,"currency":"RUB"}`},
		{name: "two_objects", body: `{"category":"еда","limit":5000}{"category":"кафе","limit":1}`},
		{name: "zero_limit", body: `{"category":"еда","limit":0}`},
		{name: "negative_limit", body: `{"category":"еда","limit":-100}`},
		{name: "three_decimals", body: `{"category":"еда","limit":10.555}`},
		{name: "empty_category", body: `{"category":"","limit":5000}`},
	}
	for _, tc := range badRequests {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newServer(t)

			res := send(t, server, http.MethodPost, "/api/budgets", tc.body)

			assertError(t, res, http.StatusBadRequest)
			got := assertJSON[[]api.BudgetResponse](t, send(t, server, http.MethodGet, "/api/budgets", ""), http.StatusOK)
			assert.Empty(t, got, "некорректный бюджет не должен сохраняться")
		})
	}
}

func TestTransactionsAPI(t *testing.T) {
	const lunch = `{"amount":450,"category":"еда","description":"ланч","date":"2025-09-10"}`

	t.Run("ok", func(t *testing.T) {
		server, _ := newServer(t)
		createBudget(t, server, `{"category":"еда","limit":5000}`)

		res := send(t, server, http.MethodPost, "/api/transactions", lunch)

		created := assertJSON[api.TransactionResponse](t, res, http.StatusCreated)
		assert.Equal(t, api.TransactionResponse{
			ID: 1, Amount: 450, Category: "еда", Description: "ланч", Date: "2025-09-10",
		}, created)

		list := assertJSON[[]api.TransactionResponse](t,
			send(t, server, http.MethodGet, "/api/transactions", ""), http.StatusOK)
		assert.Equal(t, []api.TransactionResponse{created}, list)
	})

	t.Run("list_newest_first", func(t *testing.T) {
		server, _ := newServer(t)
		older := createTransaction(t, server, `{"amount":10,"category":"еда","date":"2025-09-01"}`)
		newer := createTransaction(t, server, `{"amount":20,"category":"еда","date":"2025-09-20"}`)

		list := assertJSON[[]api.TransactionResponse](t,
			send(t, server, http.MethodGet, "/api/transactions", ""), http.StatusOK)

		assert.Equal(t, []api.TransactionResponse{newer, older}, list)
	})

	t.Run("list_empty", func(t *testing.T) {
		server, _ := newServer(t)

		res := send(t, server, http.MethodGet, "/api/transactions", "")

		assertJSON[[]api.TransactionResponse](t, res, http.StatusOK)
		assert.JSONEq(t, `[]`, string(res.body))
	})

	t.Run("no_budget", func(t *testing.T) {
		server, _ := newServer(t)

		res := send(t, server, http.MethodPost, "/api/transactions",
			`{"amount":100000,"category":"книги","description":"без бюджета","date":"2025-09-10"}`)

		assertJSON[api.TransactionResponse](t, res, http.StatusCreated)
	})

	t.Run("exceeded", func(t *testing.T) {
		server, _ := newServer(t)
		createBudget(t, server, `{"category":"еда","limit":500}`)
		createTransaction(t, server, lunch)

		res := send(t, server, http.MethodPost, "/api/transactions",
			`{"amount":50.01,"category":"еда","description":"десерт","date":"2025-09-10"}`)

		assertJSON[api.ErrorResponse](t, res, http.StatusConflict)
		assert.JSONEq(t, `{"error":"budget exceeded"}`, string(res.body))
		list := assertJSON[[]api.TransactionResponse](t,
			send(t, server, http.MethodGet, "/api/transactions", ""), http.StatusOK)
		assert.Len(t, list, 1, "отклонённая транзакция не должна сохраняться")
	})

	badRequests := []struct {
		name string
		body string
	}{
		{name: "bad_json", body: `{"amount":450,"category":`},
		{name: "empty_body", body: ``},
		{name: "wrong_type", body: `{"amount":"450","category":"еда","date":"2025-09-10"}`},
		{name: "unknown_field", body: `{"amount":450,"categry":"еда","date":"2025-09-10"}`},
		{name: "zero_amount", body: `{"amount":0,"category":"еда","date":"2025-09-10"}`},
		{name: "negative_amount", body: `{"amount":-450,"category":"еда","date":"2025-09-10"}`},
		{name: "too_large_amount", body: `{"amount":1e15,"category":"еда","date":"2025-09-10"}`},
		{name: "three_decimals", body: `{"amount":1.005,"category":"еда","date":"2025-09-10"}`},
		{name: "empty_category", body: `{"amount":450,"category":"","date":"2025-09-10"}`},
		{name: "bad_date", body: `{"amount":450,"category":"еда","date":"10.09.2025"}`},
		{name: "missing_date", body: `{"amount":450,"category":"еда"}`},
	}
	for _, tc := range badRequests {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newServer(t)

			res := send(t, server, http.MethodPost, "/api/transactions", tc.body)

			assertError(t, res, http.StatusBadRequest)
			list := assertJSON[[]api.TransactionResponse](t,
				send(t, server, http.MethodGet, "/api/transactions", ""), http.StatusOK)
			assert.Empty(t, list, "некорректная транзакция не должна сохраняться")
		})
	}
}

func TestReportSummaryAPI(t *testing.T) {
	const path = "/api/reports/summary?from=2025-09-01&to=2025-09-30"

	t.Run("ok", func(t *testing.T) {
		server, _ := newServer(t)
		createTransaction(t, server, `{"amount":450,"category":"еда","date":"2025-09-10"}`)
		createTransaction(t, server, `{"amount":65.5,"category":"транспорт","date":"2025-09-30"}`)
		createTransaction(t, server, `{"amount":1000,"category":"еда","date":"2025-10-01"}`)

		res := send(t, server, http.MethodGet, path, "")

		got := assertJSON[api.ReportSummaryResponse](t, res, http.StatusOK)
		assert.Equal(t, api.ReportSummaryResponse{
			From:  "2025-09-01",
			To:    "2025-09-30",
			Total: 515.5,
			Categories: []api.CategoryTotalResponse{
				{Category: "еда", Total: 450},
				{Category: "транспорт", Total: 65.5},
			},
		}, got)
	})

	t.Run("empty", func(t *testing.T) {
		server, _ := newServer(t)

		res := send(t, server, http.MethodGet, path, "")

		assertJSON[api.ReportSummaryResponse](t, res, http.StatusOK)
		assert.JSONEq(t, `{"from":"2025-09-01","to":"2025-09-30","total":0,"categories":[]}`, string(res.body))
	})

	t.Run("cached", func(t *testing.T) {
		server, mr := newServer(t)
		createTransaction(t, server, `{"amount":450,"category":"еда","date":"2025-09-10"}`)

		first := send(t, server, http.MethodGet, path, "")
		second := send(t, server, http.MethodGet, path, "")

		assertJSON[api.ReportSummaryResponse](t, second, http.StatusOK)
		assert.Equal(t, string(first.body), string(second.body))
		assert.Equal(t, 30*time.Second, mr.TTL("report:summary:2025-09-01:2025-09-30"))
	})

	t.Run("updated_after_transaction", func(t *testing.T) {
		server, _ := newServer(t)
		createTransaction(t, server, `{"amount":450,"category":"еда","date":"2025-09-10"}`)
		send(t, server, http.MethodGet, path, "")

		createTransaction(t, server, `{"amount":50,"category":"еда","date":"2025-09-11"}`)

		got := assertJSON[api.ReportSummaryResponse](t, send(t, server, http.MethodGet, path, ""), http.StatusOK)
		assert.Equal(t, 500.0, got.Total)
	})

	badRequests := []struct {
		name  string
		query string
	}{
		{name: "no_params", query: ""},
		{name: "missing_to", query: "?from=2025-09-01"},
		{name: "missing_from", query: "?to=2025-09-30"},
		{name: "bad_format", query: "?from=01.09.2025&to=2025-09-30"},
		{name: "nonexistent_date", query: "?from=2025-09-01&to=2025-09-31"},
		{name: "to_before_from", query: "?from=2025-09-30&to=2025-09-01"},
	}
	for _, tc := range badRequests {
		t.Run(tc.name, func(t *testing.T) {
			server := newRouterServer(t)

			res := send(t, server, http.MethodGet, "/api/reports/summary"+tc.query, "")

			assertError(t, res, http.StatusBadRequest)
		})
	}
}

func TestInternalError(t *testing.T) {
	requests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "list_transactions", method: http.MethodGet, path: "/api/transactions"},
		{name: "create_transaction", method: http.MethodPost, path: "/api/transactions",
			body: `{"amount":450,"category":"еда","date":"2025-09-10"}`},
		{name: "list_budgets", method: http.MethodGet, path: "/api/budgets"},
		{name: "create_budget", method: http.MethodPost, path: "/api/budgets", body: `{"category":"еда","limit":5000}`},
		{name: "report", method: http.MethodGet, path: "/api/reports/summary?from=2025-09-01&to=2025-09-30"},
	}

	for _, tc := range requests {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newServer(t)
			require.NoError(t, ledger.Close())

			res := send(t, server, tc.method, tc.path, tc.body)

			assertJSON[api.ErrorResponse](t, res, http.StatusInternalServerError)
			assert.JSONEq(t, `{"error":"internal server error"}`, string(res.body),
				"детали ошибки БД не должны уходить клиенту")
		})
	}
}

func TestRouting(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantAllow  string
	}{
		{name: "unknown_api_path", method: http.MethodGet, path: "/api/unknown", wantStatus: http.StatusNotFound},
		{name: "trailing_slash", method: http.MethodGet, path: "/api/budgets/", wantStatus: http.StatusNotFound},
		{name: "outside_api", method: http.MethodGet, path: "/transactions", wantStatus: http.StatusNotFound},
		{name: "delete_transactions", method: http.MethodDelete, path: "/api/transactions",
			wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, POST"},
		{name: "put_budgets", method: http.MethodPut, path: "/api/budgets",
			wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, POST"},
		{name: "post_report", method: http.MethodPost, path: "/api/reports/summary",
			wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newRouterServer(t)

			res := send(t, server, tc.method, tc.path, "")

			assertError(t, res, tc.wantStatus)
			assert.Equal(t, tc.wantAllow, res.header.Get("Allow"))
		})
	}
}
