package ledger

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

type Budget struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

var ErrBudgetExceeded = errors.New("бюджет превышен")

const (
	budgetsCacheKey = "budgets:all"
	budgetsCacheTTL = 30 * time.Second
)

func (b Budget) Validate() error {
	if b.Limit <= 0 || math.IsNaN(b.Limit) || math.IsInf(b.Limit, 0) {
		return errors.New("лимит бюджета должен быть положительным конечным числом")
	}
	if b.Limit > maxMoney {
		return fmt.Errorf("лимит бюджета не должен превышать %.2f", maxMoney)
	}
	if !hasCents(b.Limit) {
		return errors.New("лимит бюджета должен содержать не больше двух знаков после запятой")
	}
	if strings.TrimSpace(b.Category) == "" {
		return errors.New("категория бюджета не должна быть пустой")
	}
	return nil
}

// SetBudget создаёт бюджет категории или обновляет лимит существующего.
func SetBudget(ctx context.Context, b Budget) error {
	if err := b.Validate(); err != nil {
		return err
	}

	_, err := database.ExecContext(ctx, `
		INSERT INTO budgets (category, limit_amount) VALUES ($1, $2)
		ON CONFLICT (category) DO UPDATE SET limit_amount = EXCLUDED.limit_amount`,
		b.Category, b.Limit)
	if err != nil {
		return fmt.Errorf("сохранение бюджета: %w", err)
	}

	if err := redisCache.Delete(ctx, budgetsCacheKey); err != nil {
		log.Printf("ledger: не удалось сбросить кеш %s: %v", budgetsCacheKey, err)
	}
	return nil
}

// ListBudgets возвращает бюджеты, отсортированные по категории. Список
// кешируется в Redis на budgetsCacheTTL и сбрасывается при SetBudget.
func ListBudgets(ctx context.Context) ([]Budget, error) {
	var cached []Budget
	hit, err := redisCache.GetJSON(ctx, budgetsCacheKey, &cached)
	if err != nil {
		log.Printf("ledger: ошибка чтения кеша %s: %v", budgetsCacheKey, err)
	}
	if hit {
		return cached, nil
	}

	rows, err := database.QueryContext(ctx, `SELECT category, limit_amount FROM budgets ORDER BY category`)
	if err != nil {
		return nil, fmt.Errorf("чтение бюджетов: %w", err)
	}
	defer rows.Close()

	result := make([]Budget, 0)
	for rows.Next() {
		var b Budget
		if err := rows.Scan(&b.Category, &b.Limit); err != nil {
			return nil, fmt.Errorf("чтение бюджетов: %w", err)
		}
		result = append(result, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение бюджетов: %w", err)
	}

	if err := redisCache.SetJSON(ctx, budgetsCacheKey, result, budgetsCacheTTL); err != nil {
		log.Printf("ledger: ошибка записи кеша %s: %v", budgetsCacheKey, err)
	}
	return result, nil
}
