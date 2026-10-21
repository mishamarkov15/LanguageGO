package ledger

import (
	"errors"
	"fmt"
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
	if strings.TrimSpace(tx.Category) == "" {
		return errors.New("категория транзакции не должна быть пустой")
	}
	if _, err := time.Parse("2006-01-02", tx.Date); err != nil {
		return fmt.Errorf("дата транзакции должна быть в формате ГГГГ-ММ-ДД: %w", err)
	}
	return nil
}

// AddTransaction сохраняет транзакцию и возвращает её с назначенным ID.
// При превышении лимита возвращается ошибка, обёрнутая в ErrBudgetExceeded.
func AddTransaction(tx Transaction) (Transaction, error) {
	if err := tx.Validate(); err != nil {
		return Transaction{}, err
	}

	mu.Lock()
	defer mu.Unlock()

	if budget, ok := budgets[tx.Category]; ok {
		total := tx.Amount
		for _, saved := range transactions {
			if saved.Category == tx.Category {
				total += saved.Amount
			}
		}
		if total > budget.Limit {
			return Transaction{}, fmt.Errorf("%w: категория %q, сумма %.2f, лимит %.2f",
				ErrBudgetExceeded, tx.Category, total, budget.Limit)
		}
	}

	tx.ID = len(transactions) + 1
	transactions = append(transactions, tx)
	return tx, nil
}

func ListTransactions() []Transaction {
	mu.RLock()
	defer mu.RUnlock()

	result := make([]Transaction, len(transactions))
	copy(result, transactions)
	return result
}
