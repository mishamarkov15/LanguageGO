package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-9/ledger/internal/domain"
	"hw-9/ledger/internal/service"
)

type budgetStub struct {
	domain.BudgetRepository
	set  func(context.Context, domain.Budget) error
	get  func(context.Context, string) (domain.Budget, error)
	list func(context.Context) ([]domain.Budget, error)
}

func (b *budgetStub) Set(ctx context.Context, value domain.Budget) error { return b.set(ctx, value) }
func (b *budgetStub) GetByCategory(ctx context.Context, category string) (domain.Budget, error) {
	return b.get(ctx, category)
}
func (b *budgetStub) List(ctx context.Context) ([]domain.Budget, error) { return b.list(ctx) }

type expenseStub struct {
	domain.ExpenseRepository
	add        func(context.Context, domain.Transaction) (domain.Transaction, error)
	list       func(context.Context) ([]domain.Transaction, error)
	sum        func(context.Context, string) (float64, error)
	categories func(context.Context, domain.Period) ([]string, error)
	periodSum  func(context.Context, string, domain.Period) (float64, error)
}

func (e *expenseStub) Add(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	return e.add(ctx, tx)
}
func (e *expenseStub) List(ctx context.Context) ([]domain.Transaction, error) { return e.list(ctx) }
func (e *expenseStub) SumByCategory(ctx context.Context, category string) (float64, error) {
	return e.sum(ctx, category)
}
func (e *expenseStub) CategoriesInPeriod(ctx context.Context, period domain.Period) ([]string, error) {
	return e.categories(ctx, period)
}
func (e *expenseStub) SumInPeriod(ctx context.Context, category string, period domain.Period) (float64, error) {
	return e.periodSum(ctx, category, period)
}

type transactionFunc func(context.Context, func(domain.BudgetRepository, domain.ExpenseRepository) error) error

func (f transactionFunc) WithinTransaction(ctx context.Context, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
	return f(ctx, fn)
}

type cacheStub struct {
	data         map[string][]byte
	ttls         map[string]time.Duration
	getErr       error
	setErr       error
	deleteErr    error
	deleted      []string
	prefixes     []string
	beforeDelete func()
}

func newCache() *cacheStub {
	return &cacheStub{data: make(map[string][]byte), ttls: make(map[string]time.Duration)}
}

func (c *cacheStub) GetJSON(_ context.Context, key string, dest any) (bool, error) {
	if c.getErr != nil {
		return false, c.getErr
	}
	data, ok := c.data[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(data, dest)
}

func (c *cacheStub) SetJSON(_ context.Context, key string, value any, ttl time.Duration) error {
	if c.setErr != nil {
		return c.setErr
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.data[key], c.ttls[key] = data, ttl
	return nil
}

func (c *cacheStub) Delete(_ context.Context, keys ...string) error {
	if c.beforeDelete != nil {
		c.beforeDelete()
	}
	c.deleted = append(c.deleted, keys...)
	if c.deleteErr != nil {
		return c.deleteErr
	}
	for _, key := range keys {
		delete(c.data, key)
	}
	return nil
}

func (c *cacheStub) DeleteByPrefix(_ context.Context, prefix string) error {
	if c.beforeDelete != nil {
		c.beforeDelete()
	}
	c.prefixes = append(c.prefixes, prefix)
	if c.deleteErr != nil {
		return c.deleteErr
	}
	for key := range c.data {
		if strings.HasPrefix(key, prefix) {
			delete(c.data, key)
		}
	}
	return nil
}

func validTransaction() domain.Transaction {
	return domain.Transaction{Amount: 10, Category: "еда", Description: "ланч", Date: "2025-09-10"}
}

func TestAddTransaction(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		limit, spent, amount float64
		noBudget, rejected   bool
	}{
		{name: "within_limit", limit: 5000, spent: 1000, amount: 3000},
		{name: "exactly_limit", limit: 5000, spent: 3000, amount: 2000},
		{name: "exceeded", limit: 5000, spent: 4500, amount: 600, rejected: true},
		{name: "cents_exactly_limit", limit: 0.3, spent: 0.1, amount: 0.2},
		{name: "one_cent_over_limit", limit: 0.3, spent: 0.3, amount: 0.01, rejected: true},
		{name: "no_budget", amount: 1_000_000, noBudget: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var events []string
			var persisted []domain.Transaction
			budgets := &budgetStub{get: func(_ context.Context, category string) (domain.Budget, error) {
				events = append(events, "budget")
				assert.Equal(t, "еда", category)
				if tc.noBudget {
					return domain.Budget{}, domain.ErrNotFound
				}
				return domain.Budget{Category: category, Limit: tc.limit}, nil
			}}
			expenses := &expenseStub{
				sum: func(_ context.Context, category string) (float64, error) {
					events = append(events, "sum")
					assert.Equal(t, "еда", category)
					return tc.spent, nil
				},
				add: func(_ context.Context, tx domain.Transaction) (domain.Transaction, error) {
					events = append(events, "add")
					tx.ID = 42
					persisted = append(persisted, tx)
					return tx, nil
				},
			}
			manager := transactionFunc(func(_ context.Context, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
				events = append(events, "begin")
				if err := fn(budgets, expenses); err != nil {
					return err
				}
				events = append(events, "commit")
				return nil
			})
			cache := newCache()
			cache.beforeDelete = func() { events = append(events, "invalidate") }
			// Nil outside repositories ensure all budget checks and writes use the transaction's repositories.
			svc := service.New(nil, nil, manager, cache)
			input := validTransaction()
			input.Amount = tc.amount
			saved, err := svc.AddTransaction(ctx, input)
			if tc.rejected {
				require.ErrorIs(t, err, domain.ErrBudgetExceeded)
				assert.Empty(t, persisted)
				assert.Empty(t, cache.prefixes)
				assert.Zero(t, saved)
				return
			}
			require.NoError(t, err)
			input.ID = 42
			assert.Equal(t, input, saved)
			assert.Equal(t, []domain.Transaction{input}, persisted)
			wantEvents := []string{"begin", "budget", "sum", "add", "commit", "invalidate"}
			if tc.noBudget {
				wantEvents = []string{"begin", "budget", "add", "commit", "invalidate"}
			}
			assert.Equal(t, wantEvents, events)
			assert.Equal(t, []string{"report:summary:"}, cache.prefixes)
		})
	}
}

func TestValidationBeforeDependencies(t *testing.T) {
	svc := service.New(nil, nil, nil, nil)
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"transaction": func() error { _, err := svc.AddTransaction(ctx, domain.Transaction{}); return err },
		"budget":      func() error { return svc.SetBudget(ctx, domain.Budget{}) },
		"period":      func() error { _, err := svc.GetReportSummary(ctx, "2025-09-30", "2025-09-01"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			var validationErr *domain.ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Equal(t, validationErr.Err.Error(), err.Error())
			assert.NotErrorIs(t, err, domain.ErrBudgetExceeded)
		})
	}
}

func TestAddTransactionDependencyErrors(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("dependency failed")
	for _, stage := range []string{"begin", "budget", "sum", "add", "commit"} {
		t.Run(stage, func(t *testing.T) {
			added := false
			budgets := &budgetStub{get: func(context.Context, string) (domain.Budget, error) {
				if stage == "budget" {
					return domain.Budget{}, failure
				}
				return domain.Budget{Category: "еда", Limit: 100}, nil
			}}
			expenses := &expenseStub{
				sum: func(context.Context, string) (float64, error) {
					if stage == "sum" {
						return 0, failure
					}
					return 0, nil
				},
				add: func(_ context.Context, tx domain.Transaction) (domain.Transaction, error) {
					added = true
					if stage == "add" {
						return domain.Transaction{}, failure
					}
					tx.ID = 1
					return tx, nil
				},
			}
			manager := transactionFunc(func(_ context.Context, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
				if stage == "begin" {
					return failure
				}
				if err := fn(budgets, expenses); err != nil {
					return err
				}
				return failure // commit failed
			})
			cache := newCache()
			svc := service.New(nil, nil, manager, cache)
			saved, err := svc.AddTransaction(ctx, validTransaction())
			assert.ErrorIs(t, err, failure)
			assert.Zero(t, saved)
			assert.Empty(t, cache.prefixes, "cache must survive failed writes or commits")
			assert.Equal(t, stage == "add" || stage == "commit", added)
		})
	}
}

func TestBudgetCachingAndUpdate(t *testing.T) {
	ctx := context.Background()
	stored := []domain.Budget{{Category: "еда", Limit: 5000}}
	reads := 0
	budgets := &budgetStub{
		list: func(context.Context) ([]domain.Budget, error) { reads++; return stored, nil },
		set:  func(_ context.Context, b domain.Budget) error { stored[0] = b; return nil },
	}
	cache := newCache()
	svc := service.New(budgets, nil, nil, cache)
	first, err := svc.ListBudgets(ctx)
	require.NoError(t, err)
	assert.Equal(t, stored, first)
	assert.Equal(t, 30*time.Second, cache.ttls["budgets:all"])
	cached, err := svc.ListBudgets(ctx)
	require.NoError(t, err)
	assert.Equal(t, first, cached)
	assert.Equal(t, 1, reads)

	updated := domain.Budget{Category: "еда", Limit: 7000}
	cache.beforeDelete = func() { assert.Equal(t, updated, stored[0], "invalidate after persistence") }
	require.NoError(t, svc.SetBudget(ctx, updated))
	assert.Equal(t, []string{"budgets:all"}, cache.deleted)
	assert.NotContains(t, cache.data, "budgets:all")
	fresh, err := svc.ListBudgets(ctx)
	require.NoError(t, err)
	assert.Equal(t, []domain.Budget{updated}, fresh)
	assert.Equal(t, 2, reads)
}

func TestReportCacheAndInvalidation(t *testing.T) {
	ctx := context.Background()
	const key = "report:summary:2025-09-01:2025-09-30"
	report := domain.ReportSummary{From: "2025-09-01", To: "2025-09-30", Total: 0.3,
		Categories: []domain.CategoryTotal{{Category: "еда", Total: 0.3}}}
	reads := 0
	expenses := &expenseStub{
		categories: func(_ context.Context, period domain.Period) ([]string, error) {
			reads++
			assert.Equal(t, domain.Period{From: report.From, To: report.To}, period)
			return []string{"еда"}, nil
		},
		periodSum: func(context.Context, string, domain.Period) (float64, error) { return 0.30, nil },
	}
	manager := transactionFunc(func(_ context.Context, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
		b := &budgetStub{get: func(context.Context, string) (domain.Budget, error) { return domain.Budget{}, domain.ErrNotFound }}
		e := &expenseStub{add: func(_ context.Context, tx domain.Transaction) (domain.Transaction, error) { tx.ID = 1; return tx, nil }}
		return fn(b, e)
	})
	cache := newCache()
	cache.data["unrelated"] = []byte(`{}`)
	svc := service.New(nil, expenses, manager, cache)
	first, err := svc.GetReportSummary(ctx, report.From, report.To)
	require.NoError(t, err)
	assert.Equal(t, report, first)
	assert.Equal(t, 30*time.Second, cache.ttls[key])
	cached, err := svc.GetReportSummary(ctx, report.From, report.To)
	require.NoError(t, err)
	assert.Equal(t, first, cached)
	assert.Equal(t, 1, reads)
	cache.data["report:summary:2025-08-01:2025-08-31"] = []byte(`{}`)
	_, err = svc.AddTransaction(ctx, validTransaction())
	require.NoError(t, err)
	assert.NotContains(t, cache.data, key)
	assert.NotContains(t, cache.data, "report:summary:2025-08-01:2025-08-31")
	assert.Contains(t, cache.data, "unrelated")
	_, err = svc.GetReportSummary(ctx, report.From, report.To)
	require.NoError(t, err)
	assert.Equal(t, 2, reads)
}

func TestReadAndSetDependencyErrors(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("database unavailable")
	budgets := &budgetStub{
		set:  func(context.Context, domain.Budget) error { return failure },
		list: func(context.Context) ([]domain.Budget, error) { return nil, failure },
	}
	expenses := &expenseStub{
		list: func(context.Context) ([]domain.Transaction, error) { return nil, failure },
		categories: func(context.Context, domain.Period) ([]string, error) {
			return nil, failure
		},
	}
	cache := newCache()
	svc := service.New(budgets, expenses, nil, cache)
	assert.ErrorIs(t, svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 100}), failure)
	_, err := svc.ListBudgets(ctx)
	assert.ErrorIs(t, err, failure)
	_, err = svc.ListTransactions(ctx)
	assert.ErrorIs(t, err, failure)
	_, err = svc.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
	assert.ErrorIs(t, err, failure)
	assert.Empty(t, cache.deleted)
	assert.Empty(t, cache.data, "failed reads must not be cached")
}

func TestCacheUnavailableDoesNotFailOperations(t *testing.T) {
	ctx := context.Background()
	budget := domain.Budget{Category: "еда", Limit: 100}
	tx := validTransaction()
	tx.ID = 1
	report := domain.ReportSummary{From: "2025-09-01", To: "2025-09-30", Total: tx.Amount,
		Categories: []domain.CategoryTotal{{Category: "еда", Total: tx.Amount}}}
	budgets := &budgetStub{
		set:  func(context.Context, domain.Budget) error { return nil },
		get:  func(context.Context, string) (domain.Budget, error) { return budget, nil },
		list: func(context.Context) ([]domain.Budget, error) { return []domain.Budget{budget}, nil },
	}
	expenses := &expenseStub{
		add:        func(context.Context, domain.Transaction) (domain.Transaction, error) { return tx, nil },
		list:       func(context.Context) ([]domain.Transaction, error) { return []domain.Transaction{tx}, nil },
		sum:        func(context.Context, string) (float64, error) { return 0, nil },
		categories: func(context.Context, domain.Period) ([]string, error) { return []string{"еда"}, nil },
		periodSum:  func(context.Context, string, domain.Period) (float64, error) { return tx.Amount, nil },
	}
	manager := transactionFunc(func(_ context.Context, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
		return fn(budgets, expenses)
	})
	cache := newCache()
	cache.getErr = errors.New("cache read failed")
	cache.setErr = errors.New("cache write failed")
	cache.deleteErr = errors.New("cache delete failed")
	for name, selectedCache := range map[string]domain.Cache{"unavailable": cache, "disabled": nil} {
		t.Run(name, func(t *testing.T) {
			svc := service.New(budgets, expenses, manager, selectedCache)
			require.NoError(t, svc.SetBudget(ctx, budget))
			gotBudgets, err := svc.ListBudgets(ctx)
			require.NoError(t, err)
			assert.Equal(t, []domain.Budget{budget}, gotBudgets)
			saved, err := svc.AddTransaction(ctx, validTransaction())
			require.NoError(t, err)
			assert.Equal(t, tx, saved)
			gotTransactions, err := svc.ListTransactions(ctx)
			require.NoError(t, err)
			assert.Equal(t, []domain.Transaction{tx}, gotTransactions)
			gotReport, err := svc.GetReportSummary(ctx, report.From, report.To)
			require.NoError(t, err)
			assert.Equal(t, report, gotReport)
		})
	}
}

func TestMalformedCacheFallsBackToRepository(t *testing.T) {
	ctx := context.Background()
	cache := newCache()
	cache.data["budgets:all"] = []byte(`broken JSON`)
	cache.data["report:summary:2025-09-01:2025-09-30"] = []byte(`broken JSON`)
	budgets := &budgetStub{list: func(context.Context) ([]domain.Budget, error) { return []domain.Budget{}, nil }}
	report := domain.ReportSummary{From: "2025-09-01", To: "2025-09-30", Categories: []domain.CategoryTotal{}}
	expenses := &expenseStub{categories: func(context.Context, domain.Period) ([]string, error) { return nil, nil }}
	svc := service.New(budgets, expenses, nil, cache)
	list, err := svc.ListBudgets(ctx)
	require.NoError(t, err)
	assert.Equal(t, []domain.Budget{}, list)
	got, err := svc.GetReportSummary(ctx, report.From, report.To)
	require.NoError(t, err)
	assert.Equal(t, report, got)
}
