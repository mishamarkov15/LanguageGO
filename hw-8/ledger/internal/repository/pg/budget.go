package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"hw-8/ledger/internal/domain"
)

type budgetRepository struct {
	db   queryer
	lock bool
}

var _ domain.BudgetRepository = (*budgetRepository)(nil)

func (r *budgetRepository) Set(ctx context.Context, b domain.Budget) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO budgets (category, limit_amount) VALUES ($1, $2)
		ON CONFLICT (category) DO UPDATE SET limit_amount = EXCLUDED.limit_amount`,
		b.Category, b.Limit)
	if err != nil {
		return fmt.Errorf("сохранение бюджета: %w", err)
	}
	return nil
}

func (r *budgetRepository) GetByCategory(ctx context.Context, category string) (domain.Budget, error) {
	query := `SELECT category, limit_amount FROM budgets WHERE category = $1`
	if r.lock {
		query += ` FOR UPDATE`
	}
	var b domain.Budget
	if err := r.db.QueryRowContext(ctx, query, category).Scan(&b.Category, &b.Limit); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Budget{}, fmt.Errorf("бюджет категории %q: %w", category, domain.ErrNotFound)
		}
		return domain.Budget{}, fmt.Errorf("чтение бюджета: %w", err)
	}
	return b, nil
}

func (r *budgetRepository) List(ctx context.Context) ([]domain.Budget, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT category, limit_amount FROM budgets ORDER BY category`)
	if err != nil {
		return nil, fmt.Errorf("чтение бюджетов: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Budget, 0)
	for rows.Next() {
		var b domain.Budget
		if err := rows.Scan(&b.Category, &b.Limit); err != nil {
			return nil, fmt.Errorf("чтение бюджетов: %w", err)
		}
		result = append(result, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение бюджетов: %w", err)
	}
	return result, nil
}
