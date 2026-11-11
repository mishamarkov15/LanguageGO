package pg_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-9/ledger/internal/domain"
	"hw-9/ledger/internal/repository/pg"
	"hw-9/ledger/internal/service"
)

// Каждый тест получает собственную схему; существующие таблицы не очищаются.
func setupStore(t *testing.T) *pg.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задана: тест с PostgreSQL пропущен")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("hw9_pg_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := admin.ExecContext(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE")
		assert.NoError(t, err)
	})

	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.RuntimeParams["search_path"] = schema
	database := stdlib.OpenDB(*cfg)
	database.SetMaxOpenConns(10)
	t.Cleanup(func() { database.Close() })

	files, err := filepath.Glob("../../../migrations/*.sql")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, name := range files {
		data, err := os.ReadFile(name)
		require.NoError(t, err)
		up := strings.SplitN(string(data), "-- +goose Down", 2)[0]
		_, err = database.ExecContext(ctx, up)
		require.NoError(t, err)
	}
	return pg.New(database)
}

func TestBudgetRepository(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()
	repo := store.Budgets()
	_, err := repo.GetByCategory(ctx, "missing")
	require.ErrorIs(t, err, domain.ErrNotFound)

	require.NoError(t, repo.Set(ctx, domain.Budget{Category: "транспорт", Limit: 500}))
	require.NoError(t, repo.Set(ctx, domain.Budget{Category: "еда", Limit: 100}))
	require.NoError(t, repo.Set(ctx, domain.Budget{Category: "еда", Limit: 120.35}))
	budget, err := repo.GetByCategory(ctx, "еда")
	require.NoError(t, err)
	assert.Equal(t, domain.Budget{Category: "еда", Limit: 120.35}, budget)
	budgets, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, []domain.Budget{{Category: "еда", Limit: 120.35}, {Category: "транспорт", Limit: 500}}, budgets)
}

func TestExpenseRepository(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()
	repo := store.Expenses()
	items, err := repo.List(ctx)
	require.NoError(t, err)
	assert.NotNil(t, items)
	assert.Empty(t, items)
	total, err := repo.SumByCategory(ctx, "missing")
	require.NoError(t, err)
	assert.Zero(t, total)

	inputs := []domain.Transaction{
		{Amount: 0.10, Category: "еда", Date: "2026-01-01", Description: "первый день"},
		{Amount: 0.20, Category: "еда", Date: "2026-01-31"},
		{Amount: 1.50, Category: "транспорт", Date: "2026-01-31"},
		{Amount: 50, Category: "еда", Date: "2026-02-01"},
	}
	var saved []domain.Transaction
	for _, input := range inputs {
		tx, err := repo.Add(ctx, input)
		require.NoError(t, err)
		assert.Positive(t, tx.ID)
		saved = append(saved, tx)
	}
	items, err = repo.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, []domain.Transaction{saved[3], saved[2], saved[1], saved[0]}, items)
	total, err = repo.SumByCategory(ctx, "еда")
	require.NoError(t, err)
	assert.Equal(t, 50.30, total)
	svc := service.New(store.Budgets(), repo, store, nil)
	report, err := svc.GetReportSummary(ctx, "2026-01-01", "2026-01-31")
	require.NoError(t, err)
	assert.Equal(t, domain.ReportSummary{
		From: "2026-01-01", To: "2026-01-31", Total: 1.80,
		Categories: []domain.CategoryTotal{{Category: "еда", Total: 0.30}, {Category: "транспорт", Total: 1.50}},
	}, report)
	empty, err := svc.GetReportSummary(ctx, "2025-01-01", "2025-12-31")
	require.NoError(t, err)
	assert.NotNil(t, empty.Categories)
	assert.Empty(t, empty.Categories)
	assert.Zero(t, empty.Total)
}

func TestWithinTransaction(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()
	writeBoth := func(budgets domain.BudgetRepository, expenses domain.ExpenseRepository) error {
		if err := budgets.Set(ctx, domain.Budget{Category: "еда", Limit: 100}); err != nil {
			return err
		}
		_, err := expenses.Add(ctx, domain.Transaction{Amount: 10, Category: "еда", Date: "2026-01-01"})
		return err
	}
	sentinel := errors.New("callback failed")
	err := store.WithinTransaction(ctx, func(budgets domain.BudgetRepository, expenses domain.ExpenseRepository) error {
		if err := writeBoth(budgets, expenses); err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	budgets, err := store.Budgets().List(ctx)
	require.NoError(t, err)
	assert.Empty(t, budgets)
	items, err := store.Expenses().List(ctx)
	require.NoError(t, err)
	assert.Empty(t, items)

	require.NoError(t, store.WithinTransaction(ctx, writeBoth))
	budgets, err = store.Budgets().List(ctx)
	require.NoError(t, err)
	assert.Len(t, budgets, 1)
	items, err = store.Expenses().List(ctx)
	require.NoError(t, err)
	assert.Len(t, items, 1)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	err = store.WithinTransaction(canceledCtx, func(domain.BudgetRepository, domain.ExpenseRepository) error {
		t.Fatal("callback не должен вызываться после отмены контекста")
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
}

type noCache struct{}

func (noCache) GetJSON(context.Context, string, any) (bool, error)        { return false, nil }
func (noCache) SetJSON(context.Context, string, any, time.Duration) error { return nil }
func (noCache) Delete(context.Context, ...string) error                   { return nil }
func (noCache) DeleteByPrefix(context.Context, string) error              { return nil }

func TestConcurrentTransactionsRespectBudget(t *testing.T) {
	store := setupStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	svc := service.New(store.Budgets(), store.Expenses(), store, noCache{})
	require.NoError(t, svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 100}))

	const requests = 20
	start := make(chan struct{})
	results := make(chan error, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 10, Category: "еда", Date: "2026-01-01"})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	accepted, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, domain.ErrBudgetExceeded):
			rejected++
		default:
			t.Errorf("неожиданная ошибка: %v", err)
		}
	}
	assert.Equal(t, 10, accepted)
	assert.Equal(t, 10, rejected)
	items, err := store.Expenses().List(ctx)
	require.NoError(t, err)
	assert.Len(t, items, 10)
	total, err := store.Expenses().SumByCategory(ctx, "еда")
	require.NoError(t, err)
	assert.Equal(t, 100.0, total)
}
