package ledger

import (
	"context"

	"hw-8/ledger/internal/app"
	"hw-8/ledger/internal/domain"
	"hw-8/ledger/internal/service"
)

type Service = service.Service
type Transaction = domain.Transaction
type Budget = domain.Budget
type Period = domain.Period
type CategoryTotal = domain.CategoryTotal
type ReportSummary = domain.ReportSummary
type Validatable = domain.Validatable
type ValidationError = domain.ValidationError

var ErrBudgetExceeded = domain.ErrBudgetExceeded

func New(ctx context.Context) (Service, func() error, error) {
	return app.New(ctx)
}

func CheckValid(v Validatable) error {
	return v.Validate()
}
