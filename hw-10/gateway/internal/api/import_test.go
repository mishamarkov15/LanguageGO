package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-10/gateway/internal/api"
	"hw-10/ledger"
)

const bulkPath = "/api/transactions/bulk"

func TestBulkAPI(t *testing.T) {
	svc := stubService{importTransactions: func(ctx context.Context, items []ledger.Transaction, workers int) (ledger.ImportSummary, error) {
		assert.Equal(t, 3, workers)
		assert.Len(t, items, 4)
		assert.Equal(t, ledger.Transaction{Amount: 12.5, Category: "еда", Date: "2026-05-01", Description: "обед"}, items[0])
		return ledger.ImportSummary{Accepted: 1, Rejected: 3, Errors: []ledger.ImportResultError{
			{Index: 1, Err: ledger.ErrBudgetExceeded},
			{Index: 2, Err: &ledger.ValidationError{Err: errors.New("invalid amount")}},
			{Index: 3, Err: errors.New("database details")},
		}}, nil
	}}
	res := send(svc, "POST", bulkPath+"?workers=3", `[{"amount":12.5,"category":"еда","date":"2026-05-01","description":"обед"},{},{},{}]`)
	require.Equal(t, 200, res.Code)
	assert.JSONEq(t, `{"accepted":1,"rejected":3,"errors":[{"index":1,"error":"budget exceeded"},{"index":2,"error":"invalid amount"},{"index":3,"error":"internal server error"}]}`, res.Body.String())

	svc.importTransactions = func(_ context.Context, items []ledger.Transaction, workers int) (ledger.ImportSummary, error) {
		assert.Equal(t, 4, workers)
		assert.Empty(t, items)
		return ledger.ImportSummary{}, nil
	}
	res = send(svc, "POST", bulkPath, "[]")
	require.Equal(t, 200, res.Code)
	assert.JSONEq(t, `{"accepted":0,"rejected":0,"errors":[]}`, res.Body.String())
}

func TestBulkAPIInvalidRequest(t *testing.T) {
	for _, body := range []string{"", "null", "{}", "[", "[] []", `[{"unexpected":1}]`} {
		res := send(stubService{}, "POST", bulkPath, body)
		assert.NotEmpty(t, assertJSON[api.ErrorResponse](t, res, 400).Error)
	}
	for _, query := range []string{"?workers=", "?workers=two", "?workers=1&workers=2"} {
		res := send(stubService{}, "POST", bulkPath+query, "[]")
		assert.NotEmpty(t, assertJSON[api.ErrorResponse](t, res, 400).Error)
	}
	res := send(stubService{}, "GET", bulkPath, "")
	assert.Equal(t, 405, res.Code)
	assert.Equal(t, "POST", res.Header().Get("Allow"))
}

func TestBulkAPITimeout(t *testing.T) {
	workerDone := make(chan struct{})
	svc := stubService{importTransactions: func(ctx context.Context, _ []ledger.Transaction, _ int) (ledger.ImportSummary, error) {
		defer close(workerDone)
		<-ctx.Done()
		return ledger.ImportSummary{Accepted: 1}, ctx.Err()
	}}
	req := httptest.NewRequest(http.MethodPost, bulkPath, strings.NewReader("[{},{}]"))
	res := httptest.NewRecorder()
	api.NewRouter(svc, 50*time.Millisecond).ServeHTTP(res, req)
	assert.Equal(t, "request canceled or timed out", assertJSON[api.ErrorResponse](t, res, 504).Error)
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("Ledger не получил отмену запроса")
	}
}
