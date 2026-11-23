package service_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"hw-10/ledger/internal/domain"
	"hw-10/ledger/internal/service"
)

func TestImportCancellationKeepsCompletedResults(t *testing.T) {
	const workers = 3
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls, active atomic.Int64
	started := make(chan struct{}, workers)
	manager := transactionFunc(func(ctx context.Context, _ func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
		if calls.Add(1) == 1 {
			return nil // Первая валидная транзакция уже зафиксирована.
		}
		active.Add(1)
		defer active.Add(-1)
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	})
	svc := service.New(nil, nil, manager, nil)
	items := make([]domain.Transaction, 20)
	for i := 1; i < len(items); i++ {
		items[i] = validTransaction()
	}
	done := make(chan struct{})
	var summary domain.ImportSummary
	var err error
	go func() {
		defer close(done)
		summary, err = svc.ImportTransactions(ctx, items, workers)
	}()
	for range workers {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("три воркера не начали работу параллельно")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("импорт не завершился после отмены")
	}
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 1, summary.Accepted)
	require.EqualValues(t, 1, summary.Rejected)
	require.Len(t, summary.Errors, 1)
	require.Equal(t, 0, summary.Errors[0].Index)
	require.Zero(t, active.Load())
	require.EqualValues(t, workers+1, calls.Load())
}

func TestImportResults(t *testing.T) {
	failure := errors.New("database offline")
	manager := transactionFunc(func(context.Context, func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
		return failure
	})
	svc := service.New(nil, nil, manager, nil)
	ctx := context.Background()
	empty, err := svc.ImportTransactions(ctx, nil, 4)
	require.NoError(t, err)
	require.Zero(t, empty.Accepted)
	require.Zero(t, empty.Rejected)
	for _, workers := range []int{-1, 0, 33} {
		_, err := svc.ImportTransactions(ctx, nil, workers)
		require.ErrorAs(t, err, new(*domain.ValidationError))
	}
	items := []domain.Transaction{{}, validTransaction(), {}}
	result, err := svc.ImportTransactions(ctx, items, 4)
	require.NoError(t, err)
	require.EqualValues(t, 0, result.Accepted)
	require.EqualValues(t, 3, result.Rejected)
	require.Len(t, result.Errors, 3)
	for i, problem := range result.Errors {
		require.Equal(t, i, problem.Index)
	}
	require.ErrorIs(t, result.Errors[1].Err, failure)

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	result, err = svc.ImportTransactions(canceled, items, 4)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, result.Accepted)
	require.Zero(t, result.Rejected)
}
