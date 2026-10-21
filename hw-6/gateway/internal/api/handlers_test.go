package api_test

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-6/gateway/internal/api"
	"hw-6/ledger"
)

const jsonContentType = "application/json; charset=utf-8"

func TestMain(m *testing.M) {
	// Логи middleware не нужны в отчёте тестов.
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// newServer поднимает тестовый HTTP-сервер на роутере Gateway с чистым ledger.
// Все сервера работают с общим состоянием ledger, поэтому интеграционные
// тесты не помечаются t.Parallel().
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	ledger.Reset()
	server := httptest.NewServer(api.NewRouter())
	t.Cleanup(func() {
		server.Close()
		ledger.Reset()
	})
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

func TestBudgetsAPI(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		server := newServer(t)

		res := send(t, server, http.MethodPost, "/api/budgets", `{"category":"еда","limit":5000}`)

		got := assertJSON[api.BudgetResponse](t, res, http.StatusCreated)
		assert.Equal(t, api.BudgetResponse{Category: "еда", Limit: 5000}, got)
		assert.JSONEq(t, `{"category":"еда","limit":5000}`, string(res.body))
	})

	t.Run("list_contains_created", func(t *testing.T) {
		server := newServer(t)
		assertJSON[api.BudgetResponse](t,
			send(t, server, http.MethodPost, "/api/budgets", `{"category":"еда","limit":5000}`), http.StatusCreated)

		res := send(t, server, http.MethodGet, "/api/budgets", "")

		got := assertJSON[[]api.BudgetResponse](t, res, http.StatusOK)
		assert.Equal(t, []api.BudgetResponse{{Category: "еда", Limit: 5000}}, got)
	})

	t.Run("list_empty", func(t *testing.T) {
		server := newServer(t)

		res := send(t, server, http.MethodGet, "/api/budgets", "")

		assertJSON[[]api.BudgetResponse](t, res, http.StatusOK)
		assert.JSONEq(t, `[]`, string(res.body))
	})

	t.Run("update", func(t *testing.T) {
		server := newServer(t)
		send(t, server, http.MethodPost, "/api/budgets", `{"category":"еда","limit":5000}`)

		res := send(t, server, http.MethodPost, "/api/budgets", `{"category":"еда","limit":7000}`)
		assertJSON[api.BudgetResponse](t, res, http.StatusCreated)

		got := assertJSON[[]api.BudgetResponse](t, send(t, server, http.MethodGet, "/api/budgets", ""), http.StatusOK)
		assert.Equal(t, []api.BudgetResponse{{Category: "еда", Limit: 7000}}, got)
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
		{name: "empty_category", body: `{"category":"","limit":5000}`},
	}
	for _, tc := range badRequests {
		t.Run(tc.name, func(t *testing.T) {
			server := newServer(t)

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
		server := newServer(t)
		assertJSON[api.BudgetResponse](t,
			send(t, server, http.MethodPost, "/api/budgets", `{"category":"еда","limit":5000}`), http.StatusCreated)

		res := send(t, server, http.MethodPost, "/api/transactions", lunch)

		created := assertJSON[api.TransactionResponse](t, res, http.StatusCreated)
		assert.Equal(t, api.TransactionResponse{
			ID: 1, Amount: 450, Category: "еда", Description: "ланч", Date: "2025-09-10",
		}, created)

		list := assertJSON[[]api.TransactionResponse](t,
			send(t, server, http.MethodGet, "/api/transactions", ""), http.StatusOK)
		assert.Equal(t, []api.TransactionResponse{created}, list)
	})

	t.Run("list_empty", func(t *testing.T) {
		server := newServer(t)

		res := send(t, server, http.MethodGet, "/api/transactions", "")

		assertJSON[[]api.TransactionResponse](t, res, http.StatusOK)
		assert.JSONEq(t, `[]`, string(res.body))
	})

	t.Run("no_budget", func(t *testing.T) {
		server := newServer(t)

		res := send(t, server, http.MethodPost, "/api/transactions",
			`{"amount":100000,"category":"книги","description":"без бюджета","date":"2025-09-10"}`)

		assertJSON[api.TransactionResponse](t, res, http.StatusCreated)
	})

	t.Run("exceeded", func(t *testing.T) {
		server := newServer(t)
		send(t, server, http.MethodPost, "/api/budgets", `{"category":"еда","limit":500}`)
		assertJSON[api.TransactionResponse](t, send(t, server, http.MethodPost, "/api/transactions", lunch), http.StatusCreated)

		res := send(t, server, http.MethodPost, "/api/transactions",
			`{"amount":51,"category":"еда","description":"десерт","date":"2025-09-10"}`)

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
		{name: "empty_category", body: `{"amount":450,"category":"","date":"2025-09-10"}`},
		{name: "bad_date", body: `{"amount":450,"category":"еда","date":"10.09.2025"}`},
		{name: "missing_date", body: `{"amount":450,"category":"еда"}`},
	}
	for _, tc := range badRequests {
		t.Run(tc.name, func(t *testing.T) {
			server := newServer(t)

			res := send(t, server, http.MethodPost, "/api/transactions", tc.body)

			assertError(t, res, http.StatusBadRequest)
			list := assertJSON[[]api.TransactionResponse](t,
				send(t, server, http.MethodGet, "/api/transactions", ""), http.StatusOK)
			assert.Empty(t, list, "некорректная транзакция не должна сохраняться")
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newServer(t)

			res := send(t, server, tc.method, tc.path, "")

			assertError(t, res, tc.wantStatus)
			assert.Equal(t, tc.wantAllow, res.header.Get("Allow"))
		})
	}
}
