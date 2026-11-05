package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-8/gateway/internal/api"
	"hw-8/ledger"
)

type stubService struct {
	setBudget        func(context.Context, ledger.Budget) error
	listBudgets      func(context.Context) ([]ledger.Budget, error)
	addTransaction   func(context.Context, ledger.Transaction) (ledger.Transaction, error)
	listTransactions func(context.Context) ([]ledger.Transaction, error)
	getReportSummary func(context.Context, string, string) (ledger.ReportSummary, error)
}

func (s stubService) SetBudget(ctx context.Context, b ledger.Budget) error {
	return s.setBudget(ctx, b)
}

func (s stubService) ListBudgets(ctx context.Context) ([]ledger.Budget, error) {
	return s.listBudgets(ctx)
}

func (s stubService) AddTransaction(ctx context.Context, tx ledger.Transaction) (ledger.Transaction, error) {
	return s.addTransaction(ctx, tx)
}

func (s stubService) ListTransactions(ctx context.Context) ([]ledger.Transaction, error) {
	return s.listTransactions(ctx)
}

func (s stubService) GetReportSummary(ctx context.Context, from, to string) (ledger.ReportSummary, error) {
	return s.getReportSummary(ctx, from, to)
}

var _ ledger.Service = stubService{}

func send(service ledger.Service, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	api.NewRouter(service).ServeHTTP(res, req)
	return res
}

func assertJSON[T any](t *testing.T, res *httptest.ResponseRecorder, status int) T {
	t.Helper()
	assert.Equal(t, status, res.Code, "тело ответа: %s", res.Body)
	assert.Equal(t, "application/json; charset=utf-8", res.Header().Get("Content-Type"))
	var body T
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	return body
}

func TestBudgetsAPI(t *testing.T) {
	t.Run("create_or_update", func(t *testing.T) {
		called := false
		service := stubService{setBudget: func(ctx context.Context, b ledger.Budget) error {
			called = true
			assert.Equal(t, ledger.Budget{Category: "еда", Limit: 5000.5}, b)
			return nil
		}}

		res := send(service, http.MethodPost, "/api/budgets", `{"category":"еда","limit":5000.5}`)

		assert.True(t, called)
		assert.Equal(t, api.BudgetResponse{Category: "еда", Limit: 5000.5},
			assertJSON[api.BudgetResponse](t, res, http.StatusCreated))
		assert.JSONEq(t, `{"category":"еда","limit":5000.5}`, res.Body.String())
	})

	t.Run("list", func(t *testing.T) {
		service := stubService{listBudgets: func(context.Context) ([]ledger.Budget, error) {
			return []ledger.Budget{{Category: "еда", Limit: 5000}, {Category: "книги", Limit: 250}}, nil
		}}

		res := send(service, http.MethodGet, "/api/budgets", "")

		assert.Equal(t, []api.BudgetResponse{{Category: "еда", Limit: 5000}, {Category: "книги", Limit: 250}},
			assertJSON[[]api.BudgetResponse](t, res, http.StatusOK))
	})

	t.Run("empty_list", func(t *testing.T) {
		service := stubService{listBudgets: func(context.Context) ([]ledger.Budget, error) { return nil, nil }}

		res := send(service, http.MethodGet, "/api/budgets", "")

		assertJSON[[]api.BudgetResponse](t, res, http.StatusOK)
		assert.JSONEq(t, `[]`, res.Body.String())
	})
}

func TestTransactionsAPI(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		called := false
		service := stubService{addTransaction: func(ctx context.Context, tx ledger.Transaction) (ledger.Transaction, error) {
			called = true
			assert.Equal(t, ledger.Transaction{Amount: 450.5, Category: "еда", Description: "ланч", Date: "2025-09-10"}, tx)
			tx.ID = 42
			return tx, nil
		}}

		res := send(service, http.MethodPost, "/api/transactions",
			`{"amount":450.5,"category":"еда","description":"ланч","date":"2025-09-10"}`)

		assert.True(t, called)
		assert.Equal(t, api.TransactionResponse{ID: 42, Amount: 450.5, Category: "еда", Description: "ланч", Date: "2025-09-10"},
			assertJSON[api.TransactionResponse](t, res, http.StatusCreated))
		assert.JSONEq(t, `{"id":42,"amount":450.5,"category":"еда","description":"ланч","date":"2025-09-10"}`, res.Body.String())
	})

	t.Run("list_preserves_service_order", func(t *testing.T) {
		service := stubService{listTransactions: func(context.Context) ([]ledger.Transaction, error) {
			return []ledger.Transaction{
				{ID: 8, Amount: 20, Category: "еда", Date: "2025-09-20"},
				{ID: 3, Amount: 10, Category: "книги", Date: "2025-09-01"},
			}, nil
		}}

		res := send(service, http.MethodGet, "/api/transactions", "")

		assert.Equal(t, []api.TransactionResponse{
			{ID: 8, Amount: 20, Category: "еда", Date: "2025-09-20"},
			{ID: 3, Amount: 10, Category: "книги", Date: "2025-09-01"},
		}, assertJSON[[]api.TransactionResponse](t, res, http.StatusOK))
	})

	t.Run("empty_list", func(t *testing.T) {
		service := stubService{listTransactions: func(context.Context) ([]ledger.Transaction, error) { return nil, nil }}

		res := send(service, http.MethodGet, "/api/transactions", "")

		assertJSON[[]api.TransactionResponse](t, res, http.StatusOK)
		assert.JSONEq(t, `[]`, res.Body.String())
	})
}

func TestReportSummaryAPI(t *testing.T) {
	const path = "/api/reports/summary?from=2025-09-01&to=2025-09-30"
	t.Run("ok", func(t *testing.T) {
		called := false
		service := stubService{getReportSummary: func(ctx context.Context, from, to string) (ledger.ReportSummary, error) {
			called = true
			assert.Equal(t, "2025-09-01", from)
			assert.Equal(t, "2025-09-30", to)
			return ledger.ReportSummary{From: from, To: to, Total: 515.5,
				Categories: []ledger.CategoryTotal{{Category: "еда", Total: 450}, {Category: "транспорт", Total: 65.5}}}, nil
		}}

		res := send(service, http.MethodGet, path, "")

		assert.True(t, called)
		assertJSON[api.ReportSummaryResponse](t, res, http.StatusOK)
		assert.JSONEq(t, `{"from":"2025-09-01","to":"2025-09-30","total":515.5,"categories":[{"category":"еда","total":450},{"category":"транспорт","total":65.5}]}`, res.Body.String())
	})

	t.Run("empty_categories", func(t *testing.T) {
		service := stubService{getReportSummary: func(ctx context.Context, from, to string) (ledger.ReportSummary, error) {
			return ledger.ReportSummary{From: from, To: to}, nil
		}}

		res := send(service, http.MethodGet, path, "")

		assertJSON[api.ReportSummaryResponse](t, res, http.StatusOK)
		assert.JSONEq(t, `{"from":"2025-09-01","to":"2025-09-30","total":0,"categories":[]}`, res.Body.String())
	})
}

func TestInvalidJSONDoesNotCallService(t *testing.T) {
	tests := []struct{ name, path, body string }{
		{"budget_bad_json", "/api/budgets", `{"category":"еда",`},
		{"budget_empty_body", "/api/budgets", ``},
		{"budget_wrong_type", "/api/budgets", `{"category":"еда","limit":"5000"}`},
		{"budget_unknown_field", "/api/budgets", `{"category":"еда","limit":5000,"currency":"RUB"}`},
		{"budget_two_objects", "/api/budgets", `{"category":"еда","limit":5000}{"category":"кафе","limit":1}`},
		{"transaction_bad_json", "/api/transactions", `{"amount":450,"category":`},
		{"transaction_empty_body", "/api/transactions", ``},
		{"transaction_wrong_type", "/api/transactions", `{"amount":"450","category":"еда","date":"2025-09-10"}`},
		{"transaction_unknown_field", "/api/transactions", `{"amount":450,"categry":"еда","date":"2025-09-10"}`},
		{"transaction_too_large_body", "/api/transactions", `{"description":"` + strings.Repeat("x", 1<<20) + `"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Все методы заглушки nil: обращение к сервису завершит тест ошибкой.
			res := send(stubService{}, http.MethodPost, tc.path, tc.body)

			body := assertJSON[api.ErrorResponse](t, res, http.StatusBadRequest)
			assert.NotEmpty(t, body.Error)
		})
	}
}

func TestValidationBelongsToService(t *testing.T) {
	tests := []struct {
		name, path, body, message string
		service                   stubService
	}{
		{name: "transaction", path: "/api/transactions", body: `{"amount":-1,"category":"","date":"invalid"}`,
			message: "invalid transaction", service: stubService{addTransaction: func(ctx context.Context, tx ledger.Transaction) (ledger.Transaction, error) {
				assert.Equal(t, ledger.Transaction{Amount: -1, Date: "invalid"}, tx)
				return ledger.Transaction{}, &ledger.ValidationError{Err: errors.New("invalid transaction")}
			}}},
		{name: "budget", path: "/api/budgets", body: `{"category":"","limit":0}`,
			message: "invalid budget", service: stubService{setBudget: func(ctx context.Context, b ledger.Budget) error {
				assert.Equal(t, ledger.Budget{}, b)
				return &ledger.ValidationError{Err: errors.New("invalid budget")}
			}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := send(tc.service, http.MethodPost, tc.path, tc.body)

			assert.Equal(t, tc.message, assertJSON[api.ErrorResponse](t, res, http.StatusBadRequest).Error)
		})
	}

	t.Run("period", func(t *testing.T) {
		service := stubService{getReportSummary: func(ctx context.Context, from, to string) (ledger.ReportSummary, error) {
			assert.Equal(t, "invalid", from)
			assert.Empty(t, to)
			return ledger.ReportSummary{}, &ledger.ValidationError{Err: errors.New("invalid period")}
		}}
		res := send(service, http.MethodGet, "/api/reports/summary?from=invalid", "")

		assert.Equal(t, "invalid period", assertJSON[api.ErrorResponse](t, res, http.StatusBadRequest).Error)
	})
}

func errorService(err error) stubService {
	return stubService{
		setBudget:   func(context.Context, ledger.Budget) error { return err },
		listBudgets: func(context.Context) ([]ledger.Budget, error) { return nil, err },
		addTransaction: func(context.Context, ledger.Transaction) (ledger.Transaction, error) {
			return ledger.Transaction{}, err
		},
		listTransactions: func(context.Context) ([]ledger.Transaction, error) { return nil, err },
		getReportSummary: func(context.Context, string, string) (ledger.ReportSummary, error) {
			return ledger.ReportSummary{}, err
		},
	}
}

func TestServiceErrors(t *testing.T) {
	t.Run("wrapped_validation", func(t *testing.T) {
		err := fmt.Errorf("operation: %w", &ledger.ValidationError{Err: errors.New("invalid amount")})

		res := send(errorService(err), http.MethodPost, "/api/transactions", `{}`)

		assert.Equal(t, "invalid amount", assertJSON[api.ErrorResponse](t, res, http.StatusBadRequest).Error)
	})

	t.Run("wrapped_budget_exceeded", func(t *testing.T) {
		res := send(errorService(fmt.Errorf("category: %w", ledger.ErrBudgetExceeded)),
			http.MethodPost, "/api/transactions", `{"amount":10,"category":"еда","date":"2025-09-10"}`)

		assertJSON[api.ErrorResponse](t, res, http.StatusConflict)
		assert.JSONEq(t, `{"error":"budget exceeded"}`, res.Body.String())
	})

	requests := []struct{ name, method, path, body string }{
		{"create_transaction", http.MethodPost, "/api/transactions", `{"amount":1,"category":"еда","date":"2025-09-10"}`},
		{"list_transactions", http.MethodGet, "/api/transactions", ""},
		{"create_budget", http.MethodPost, "/api/budgets", `{"category":"еда","limit":5000}`},
		{"list_budgets", http.MethodGet, "/api/budgets", ""},
		{"report", http.MethodGet, "/api/reports/summary?from=2025-09-01&to=2025-09-30", ""},
	}
	for _, tc := range requests {
		t.Run(tc.name+"_internal_error", func(t *testing.T) {
			res := send(errorService(errors.New("private storage details")), tc.method, tc.path, tc.body)

			assertJSON[api.ErrorResponse](t, res, http.StatusInternalServerError)
			assert.JSONEq(t, `{"error":"internal server error"}`, res.Body.String())
		})
	}
}

func TestRequestContext(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "request"))
	cancel()
	called := 0
	check := func(got context.Context) {
		called++
		assert.Equal(t, "request", got.Value(contextKey{}))
		assert.ErrorIs(t, got.Err(), context.Canceled)
	}
	service := stubService{
		setBudget:   func(ctx context.Context, b ledger.Budget) error { check(ctx); return nil },
		listBudgets: func(ctx context.Context) ([]ledger.Budget, error) { check(ctx); return nil, nil },
		addTransaction: func(ctx context.Context, tx ledger.Transaction) (ledger.Transaction, error) {
			check(ctx)
			return tx, nil
		},
		listTransactions: func(ctx context.Context) ([]ledger.Transaction, error) { check(ctx); return nil, nil },
		getReportSummary: func(ctx context.Context, from, to string) (ledger.ReportSummary, error) {
			check(ctx)
			return ledger.ReportSummary{}, nil
		},
	}
	router := api.NewRouter(service)
	for _, path := range []string{"/api/budgets", "/api/transactions", "/api/reports/summary"} {
		req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}
	for _, path := range []string{"/api/budgets", "/api/transactions"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)).WithContext(ctx)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}
	assert.Equal(t, 5, called)
}

func TestRouting(t *testing.T) {
	tests := []struct {
		name, method, path string
		status             int
		allow              string
	}{
		{"unknown_api_path", http.MethodGet, "/api/unknown", http.StatusNotFound, ""},
		{"trailing_slash", http.MethodGet, "/api/budgets/", http.StatusNotFound, ""},
		{"outside_api", http.MethodGet, "/transactions", http.StatusNotFound, ""},
		{"delete_transactions", http.MethodDelete, "/api/transactions", http.StatusMethodNotAllowed, "GET, POST"},
		{"put_budgets", http.MethodPut, "/api/budgets", http.StatusMethodNotAllowed, "GET, POST"},
		{"post_report", http.MethodPost, "/api/reports/summary", http.StatusMethodNotAllowed, "GET"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := send(stubService{}, tc.method, tc.path, "")

			assert.NotEmpty(t, assertJSON[api.ErrorResponse](t, res, tc.status).Error)
			assert.Equal(t, tc.allow, res.Header().Get("Allow"))
		})
	}
}
