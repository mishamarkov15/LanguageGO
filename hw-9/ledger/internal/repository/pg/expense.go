package pg

import (
	"context"
	"fmt"
	"time"

	"hw-9/ledger/internal/domain"
)

type expenseRepository struct {
	db queryer
}

var _ domain.ExpenseRepository = (*expenseRepository)(nil)

func (r *expenseRepository) Add(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO expenses (amount, category, description, date) VALUES ($1, $2, $3, $4)
		RETURNING id`, tx.Amount, tx.Category, tx.Description, tx.Date).Scan(&tx.ID)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("сохранение транзакции: %w", err)
	}
	return tx, nil
}

func (r *expenseRepository) List(ctx context.Context) ([]domain.Transaction, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, amount, category, COALESCE(description, ''), date
		FROM expenses ORDER BY date DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("чтение транзакций: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Transaction, 0)
	for rows.Next() {
		var tx domain.Transaction
		var date time.Time
		if err := rows.Scan(&tx.ID, &tx.Amount, &tx.Category, &tx.Description, &date); err != nil {
			return nil, fmt.Errorf("чтение транзакций: %w", err)
		}
		tx.Date = date.Format("2006-01-02")
		result = append(result, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение транзакций: %w", err)
	}
	return result, nil
}

func (r *expenseRepository) SumByCategory(ctx context.Context, category string) (float64, error) {
	var total float64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM expenses WHERE category = $1`, category).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("подсчёт трат: %w", err)
	}
	return total, nil
}

func (r *expenseRepository) CategoriesInPeriod(ctx context.Context, period domain.Period) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT category FROM expenses
 WHERE date BETWEEN $1 AND $2 ORDER BY category`, period.From, period.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var categories []string
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (r *expenseRepository) SumInPeriod(ctx context.Context, category string, period domain.Period) (float64, error) {
	var total float64
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount), 0) FROM expenses
 WHERE category = $1 AND date BETWEEN $2 AND $3`, category, period.From, period.To).Scan(&total)
	return total, err
}
