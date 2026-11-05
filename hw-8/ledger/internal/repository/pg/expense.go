package pg

import (
	"context"
	"fmt"
	"time"

	"hw-8/ledger/internal/domain"
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

func (r *expenseRepository) Summary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT category, SUM(amount)
		FROM expenses WHERE date BETWEEN $1 AND $2
		GROUP BY category ORDER BY category`, from, to)
	if err != nil {
		return domain.ReportSummary{}, fmt.Errorf("расчёт отчёта: %w", err)
	}
	defer rows.Close()

	report := domain.ReportSummary{From: from, To: to, Categories: make([]domain.CategoryTotal, 0)}
	var totalCents int64
	for rows.Next() {
		var item domain.CategoryTotal
		if err := rows.Scan(&item.Category, &item.Total); err != nil {
			return domain.ReportSummary{}, fmt.Errorf("расчёт отчёта: %w", err)
		}
		totalCents += domain.ToCents(item.Total)
		report.Categories = append(report.Categories, item)
	}
	if err := rows.Err(); err != nil {
		return domain.ReportSummary{}, fmt.Errorf("расчёт отчёта: %w", err)
	}
	report.Total = domain.FromCents(totalCents)
	return report, nil
}
