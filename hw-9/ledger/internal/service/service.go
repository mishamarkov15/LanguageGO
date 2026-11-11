package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"hw-9/ledger/internal/domain"
)

const (
	budgetsCacheKey   = "budgets:all"
	reportCachePrefix = "report:summary:"
	cacheTTL          = 30 * time.Second
)

type Service interface {
	SetBudget(context.Context, domain.Budget) error
	ListBudgets(context.Context) ([]domain.Budget, error)
	AddTransaction(context.Context, domain.Transaction) (domain.Transaction, error)
	ListTransactions(context.Context) ([]domain.Transaction, error)
	GetReportSummary(context.Context, string, string) (domain.ReportSummary, error)
}

type ledgerService struct {
	budgets  domain.BudgetRepository
	expenses domain.ExpenseRepository
	tx       domain.TransactionManager
	cache    domain.Cache
}

func New(b domain.BudgetRepository, e domain.ExpenseRepository, tx domain.TransactionManager, c domain.Cache) Service {
	if c == nil {
		c = noCache{}
	}
	return &ledgerService{budgets: b, expenses: e, tx: tx, cache: c}
}

func (s *ledgerService) SetBudget(ctx context.Context, b domain.Budget) error {
	if err := b.Validate(); err != nil {
		return &domain.ValidationError{Err: err}
	}
	if err := s.budgets.Set(ctx, b); err != nil {
		return fmt.Errorf("сохранение бюджета: %w", err)
	}
	if err := s.cache.Delete(ctx, budgetsCacheKey); err != nil {
		log.Printf("ledger: не удалось сбросить кеш %s: %v", budgetsCacheKey, err)
	}
	return nil
}

func (s *ledgerService) ListBudgets(ctx context.Context) ([]domain.Budget, error) {
	var cached []domain.Budget
	hit, err := s.cache.GetJSON(ctx, budgetsCacheKey, &cached)
	if err != nil {
		log.Printf("ledger: ошибка чтения кеша %s: %v", budgetsCacheKey, err)
	} else if hit {
		return cached, nil
	}
	result, err := s.budgets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("чтение бюджетов: %w", err)
	}
	if err := s.cache.SetJSON(ctx, budgetsCacheKey, result, cacheTTL); err != nil {
		log.Printf("ledger: ошибка записи кеша %s: %v", budgetsCacheKey, err)
	}
	return result, nil
}

func (s *ledgerService) AddTransaction(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	if err := tx.Validate(); err != nil {
		return domain.Transaction{}, &domain.ValidationError{Err: err}
	}

	var saved domain.Transaction
	err := s.tx.WithinTransaction(ctx, func(budgets domain.BudgetRepository, expenses domain.ExpenseRepository) error {
		budget, err := budgets.GetByCategory(ctx, tx.Category)
		switch {
		case errors.Is(err, domain.ErrNotFound):
		case err != nil:
			return fmt.Errorf("чтение бюджета: %w", err)
		default:
			spent, err := expenses.SumByCategory(ctx, tx.Category)
			if err != nil {
				return fmt.Errorf("подсчёт трат: %w", err)
			}
			if domain.ToCents(spent)+domain.ToCents(tx.Amount) > domain.ToCents(budget.Limit) {
				return fmt.Errorf("%w: категория %q, потрачено %.2f, сумма %.2f, лимит %.2f",
					domain.ErrBudgetExceeded, tx.Category, spent, tx.Amount, budget.Limit)
			}
		}
		saved, err = expenses.Add(ctx, tx)
		if err != nil {
			return fmt.Errorf("сохранение транзакции: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Transaction{}, err
	}

	// Инвалидация происходит только после успешного commit.
	if err := s.cache.DeleteByPrefix(ctx, reportCachePrefix); err != nil {
		log.Printf("ledger: не удалось сбросить кеш отчётов: %v", err)
	}
	return saved, nil
}

func (s *ledgerService) ListTransactions(ctx context.Context) ([]domain.Transaction, error) {
	result, err := s.expenses.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("чтение транзакций: %w", err)
	}
	return result, nil
}

func (s *ledgerService) GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	period := domain.Period{From: from, To: to}
	if err := period.Validate(); err != nil {
		return domain.ReportSummary{}, &domain.ValidationError{Err: err}
	}
	key := reportCachePrefix + from + ":" + to
	var report domain.ReportSummary
	hit, err := s.cache.GetJSON(ctx, key, &report)
	if ctx.Err() != nil {
		return domain.ReportSummary{}, ctx.Err()
	}
	if err != nil {
		log.Printf("ledger: ошибка чтения кеша %s: %v", key, err)
	} else if hit {
		log.Printf("ledger: %s — cache hit", key)
		return report, nil
	}
	report, err = s.calculateReport(ctx, period)
	if err != nil {
		return domain.ReportSummary{}, fmt.Errorf("расчёт отчёта: %w", err)
	}
	if err := s.cache.SetJSON(ctx, key, report, cacheTTL); err != nil {
		log.Printf("ledger: ошибка записи кеша %s: %v", key, err)
	} else {
		log.Printf("ledger: %s — cache miss, отчёт посчитан в БД и сохранён на %s", key, cacheTTL)
	}
	if err := ctx.Err(); err != nil {
		return domain.ReportSummary{}, err
	}
	return report, nil
}

// noCache позволяет использовать сервис без внешнего кеша.
type noCache struct{}

func (noCache) GetJSON(context.Context, string, any) (bool, error)        { return false, nil }
func (noCache) SetJSON(context.Context, string, any, time.Duration) error { return nil }
func (noCache) Delete(context.Context, ...string) error                   { return nil }
func (noCache) DeleteByPrefix(context.Context, string) error              { return nil }
