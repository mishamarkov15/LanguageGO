// Package pg реализует репозитории Ledger для PostgreSQL.
package pg

import (
	"context"
	"database/sql"
	"fmt"

	"hw-8/ledger/internal/domain"
)

type queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Store struct {
	db *sql.DB
}

var _ domain.TransactionManager = (*Store)(nil)

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Budgets() domain.BudgetRepository {
	return &budgetRepository{db: s.db}
}

func (s *Store) Expenses() domain.ExpenseRepository {
	return &expenseRepository{db: s.db}
}

// WithinTransaction передаёт callback репозитории, работающие в одной
// транзакции БД. Чтение бюджета блокирует строку до commit или rollback.
func (s *Store) WithinTransaction(ctx context.Context, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("начало транзакции БД: %w", err)
	}
	defer tx.Rollback()

	if err := fn(&budgetRepository{db: tx, lock: true}, &expenseRepository{db: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("завершение транзакции БД: %w", err)
	}
	return nil
}
