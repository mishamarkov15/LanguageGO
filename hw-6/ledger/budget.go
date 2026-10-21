package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
)

type Budget struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

var ErrBudgetExceeded = errors.New("бюджет превышен")

func (b Budget) Validate() error {
	if b.Limit <= 0 || math.IsNaN(b.Limit) || math.IsInf(b.Limit, 0) {
		return errors.New("лимит бюджета должен быть положительным конечным числом")
	}
	if strings.TrimSpace(b.Category) == "" {
		return errors.New("категория бюджета не должна быть пустой")
	}
	return nil
}

func SetBudget(b Budget) error {
	if err := b.Validate(); err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()

	budgets[b.Category] = b
	return nil
}

func ListBudgets() []Budget {
	mu.RLock()
	defer mu.RUnlock()

	result := make([]Budget, 0, len(budgets))
	for _, budget := range budgets {
		result = append(result, budget)
	}
	slices.SortFunc(result, func(a, b Budget) int {
		return strings.Compare(a.Category, b.Category)
	})
	return result
}

func LoadBudgets(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("не удалось прочитать бюджеты: %w", err)
	}

	var loaded []Budget
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("не удалось разобрать JSON бюджетов: %w", err)
	}
	if loaded == nil {
		return errors.New("бюджеты должны быть заданы массивом JSON, а не null")
	}

	for i, budget := range loaded {
		if err := budget.Validate(); err != nil {
			return fmt.Errorf("некорректный бюджет №%d: %w", i+1, err)
		}
	}
	for i, budget := range loaded {
		if err := SetBudget(budget); err != nil {
			return fmt.Errorf("не удалось установить бюджет №%d: %w", i+1, err)
		}
	}
	return nil
}
