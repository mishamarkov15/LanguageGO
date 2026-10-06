package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Budget struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

var budgets = make(map[string]Budget)

func SetBudget(b Budget) {
	budgets[b.Category] = b
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

	for _, budget := range loaded {
		SetBudget(budget)
	}
	return nil
}
