package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

type Transaction struct {
	ID          int
	Amount      float64
	Category    string
	Description string
	Date        string
}

func (tx Transaction) Validate() error {
	if tx.Amount <= 0 || math.IsNaN(tx.Amount) || math.IsInf(tx.Amount, 0) {
		return errors.New("сумма транзакции должна быть положительным конечным числом")
	}
	if tx.Amount > maxMoney {
		return fmt.Errorf("сумма транзакции не должна превышать %.2f", maxMoney)
	}
	if !hasCents(tx.Amount) {
		return errors.New("сумма транзакции должна содержать не больше двух знаков после запятой")
	}
	if strings.TrimSpace(tx.Category) == "" {
		return errors.New("категория транзакции не должна быть пустой")
	}
	if _, err := time.Parse(dateLayout, tx.Date); err != nil {
		return fmt.Errorf("дата транзакции должна быть в формате ГГГГ-ММ-ДД: %w", err)
	}
	return nil
}

// AddTransaction сохраняет транзакцию и возвращает её с ID из базы данных.
// Если у категории есть бюджет и сумма трат превысит лимит, возвращается
// ошибка, обёрнутая в ErrBudgetExceeded, и запись не добавляется.
// Категории без бюджета не ограничены.
func AddTransaction(ctx context.Context, tx Transaction) (Transaction, error) {
	if err := tx.Validate(); err != nil {
		return Transaction{}, err
	}

	sqlTx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return Transaction{}, fmt.Errorf("начало транзакции БД: %w", err)
	}
	defer sqlTx.Rollback()

	// FOR UPDATE блокирует строку бюджета до конца транзакции, поэтому
	// параллельные запросы по одной категории проверяют лимит по очереди
	// и вместе не превышают его.
	var limit float64
	err = sqlTx.QueryRowContext(ctx,
		`SELECT limit_amount FROM budgets WHERE category = $1 FOR UPDATE`, tx.Category).Scan(&limit)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return Transaction{}, fmt.Errorf("чтение бюджета: %w", err)
	default:
		var spent float64
		err := sqlTx.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(amount), 0) FROM expenses WHERE category = $1`, tx.Category).Scan(&spent)
		if err != nil {
			return Transaction{}, fmt.Errorf("подсчёт трат: %w", err)
		}
		if toCents(spent)+toCents(tx.Amount) > toCents(limit) {
			return Transaction{}, fmt.Errorf("%w: категория %q, потрачено %.2f, сумма %.2f, лимит %.2f",
				ErrBudgetExceeded, tx.Category, spent, tx.Amount, limit)
		}
	}

	err = sqlTx.QueryRowContext(ctx, `
		INSERT INTO expenses (amount, category, description, date) VALUES ($1, $2, $3, $4)
		RETURNING id`,
		tx.Amount, tx.Category, tx.Description, tx.Date).Scan(&tx.ID)
	if err != nil {
		return Transaction{}, fmt.Errorf("сохранение транзакции: %w", err)
	}
	if err := sqlTx.Commit(); err != nil {
		return Transaction{}, fmt.Errorf("сохранение транзакции: %w", err)
	}

	// Новая трата меняет отчёты, поэтому закешированные отчёты сбрасываются.
	if err := redisCache.DeleteByPrefix(ctx, reportCachePrefix); err != nil {
		log.Printf("ledger: не удалось сбросить кеш отчётов: %v", err)
	}
	return tx, nil
}

// ListTransactions возвращает транзакции от новых к старым: по дате, затем по ID.
func ListTransactions(ctx context.Context) ([]Transaction, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT id, amount, category, COALESCE(description, ''), date
		FROM expenses
		ORDER BY date DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("чтение транзакций: %w", err)
	}
	defer rows.Close()

	result := make([]Transaction, 0)
	for rows.Next() {
		var tx Transaction
		var date time.Time
		if err := rows.Scan(&tx.ID, &tx.Amount, &tx.Category, &tx.Description, &date); err != nil {
			return nil, fmt.Errorf("чтение транзакций: %w", err)
		}
		tx.Date = date.Format(dateLayout)
		result = append(result, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение транзакций: %w", err)
	}
	return result, nil
}
