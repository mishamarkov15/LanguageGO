package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	dateLayout = "2006-01-02"
	maxMoney   = 999_999_999_999.99
)

type Validatable interface {
	Validate() error
}

func CheckValid(v Validatable) error {
	return v.Validate()
}

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

type Budget struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

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

// Period задаёт интервал отчёта, включая обе даты.
type Period struct {
	From string
	To   string
}

func (p Period) Validate() error {
	from, err := time.Parse(dateLayout, p.From)
	if err != nil {
		return errors.New("параметр from должен быть датой в формате ГГГГ-ММ-ДД")
	}
	to, err := time.Parse(dateLayout, p.To)
	if err != nil {
		return errors.New("параметр to должен быть датой в формате ГГГГ-ММ-ДД")
	}
	if to.Before(from) {
		return errors.New("дата to не может быть раньше даты from")
	}
	return nil
}

type CategoryTotal struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
}

type ReportSummary struct {
	From       string          `json:"from"`
	To         string          `json:"to"`
	Total      float64         `json:"total"`
	Categories []CategoryTotal `json:"categories"`
}

func hasCents(x float64) bool {
	return math.Round(x*100)/100 == x
}

// ToCents переводит сумму в целые копейки для точного сравнения лимитов.
func ToCents(x float64) int64 {
	return int64(math.Round(x * 100))
}

func FromCents(cents int64) float64 {
	return float64(cents) / 100
}
