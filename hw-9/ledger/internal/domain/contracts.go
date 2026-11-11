package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound       = errors.New("запись не найдена")
	ErrBudgetExceeded = errors.New("бюджет превышен")
)

type ValidationError struct {
	Err error
}

func (e ValidationError) Error() string { return e.Err.Error() }
func (e ValidationError) Unwrap() error { return e.Err }

type BudgetRepository interface {
	Set(context.Context, Budget) error
	GetByCategory(context.Context, string) (Budget, error)
	List(context.Context) ([]Budget, error)
}

type ExpenseRepository interface {
	Add(context.Context, Transaction) (Transaction, error)
	List(context.Context) ([]Transaction, error)
	SumByCategory(context.Context, string) (float64, error)
	CategoriesInPeriod(context.Context, Period) ([]string, error)
	SumInPeriod(context.Context, string, Period) (float64, error)
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(BudgetRepository, ExpenseRepository) error) error
}

type Cache interface {
	GetJSON(context.Context, string, any) (bool, error)
	SetJSON(context.Context, string, any, time.Duration) error
	Delete(context.Context, ...string) error
	DeleteByPrefix(context.Context, string) error
}
