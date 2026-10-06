package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

type Budget struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

func (b Budget) Validate() error {
	if b.Limit <= 0 || math.IsNaN(b.Limit) || math.IsInf(b.Limit, 0) {
		return errors.New("лимит бюджета должен быть положительным конечным числом")
	}
	if strings.TrimSpace(b.Category) == "" {
		return errors.New("категория бюджета не должна быть пустой")
	}
	return nil
}

var budgets = make(map[string]Budget)

func SetBudget(b Budget) error {
	if err := b.Validate(); err != nil {
		return err
	}
	budgets[b.Category] = b
	return nil
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
