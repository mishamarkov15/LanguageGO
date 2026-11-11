package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-9/ledger/internal/domain"
)

func TestTransactionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tx      domain.Transaction
		wantErr bool
	}{
		{name: "valid", tx: domain.Transaction{Amount: 450, Category: "еда", Description: "ланч", Date: "2025-09-10"}},
		{name: "valid_without_description", tx: domain.Transaction{Amount: 0.01, Category: "еда", Date: "2025-09-10"}},
		{name: "two_decimals", tx: domain.Transaction{Amount: 450.35, Category: "еда", Date: "2025-09-10"}},
		{name: "max_amount", tx: domain.Transaction{Amount: 999_999_999_999.99, Category: "еда", Date: "2025-09-10"}},
		{name: "zero_amount", tx: domain.Transaction{Amount: 0, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "negative_amount", tx: domain.Transaction{Amount: -100, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "nan_amount", tx: domain.Transaction{Amount: math.NaN(), Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "infinite_amount", tx: domain.Transaction{Amount: math.Inf(1), Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "too_large_amount", tx: domain.Transaction{Amount: 1e12, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "three_decimals", tx: domain.Transaction{Amount: 1.234, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "empty_category", tx: domain.Transaction{Amount: 100, Category: "", Date: "2025-09-10"}, wantErr: true},
		{name: "blank_category", tx: domain.Transaction{Amount: 100, Category: "   ", Date: "2025-09-10"}, wantErr: true},
		{name: "empty_date", tx: domain.Transaction{Amount: 100, Category: "еда", Date: ""}, wantErr: true},
		{name: "wrong_date_format", tx: domain.Transaction{Amount: 100, Category: "еда", Date: "10.09.2025"}, wantErr: true},
		{name: "nonexistent_date", tx: domain.Transaction{Amount: 100, Category: "еда", Date: "2025-02-30"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.tx.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBudgetValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		budget  domain.Budget
		wantErr bool
	}{
		{name: "positive_limit", budget: domain.Budget{Category: "еда", Limit: 5000}},
		{name: "fractional_limit", budget: domain.Budget{Category: "еда", Limit: 0.5}},
		{name: "max_limit", budget: domain.Budget{Category: "еда", Limit: 999_999_999_999.99}},
		{name: "zero_limit", budget: domain.Budget{Category: "еда", Limit: 0}, wantErr: true},
		{name: "negative_limit", budget: domain.Budget{Category: "еда", Limit: -1}, wantErr: true},
		{name: "nan_limit", budget: domain.Budget{Category: "еда", Limit: math.NaN()}, wantErr: true},
		{name: "infinite_limit", budget: domain.Budget{Category: "еда", Limit: math.Inf(1)}, wantErr: true},
		{name: "too_large_limit", budget: domain.Budget{Category: "еда", Limit: 1e12}, wantErr: true},
		{name: "three_decimals", budget: domain.Budget{Category: "еда", Limit: 0.001}, wantErr: true},
		{name: "empty_category", budget: domain.Budget{Category: "", Limit: 5000}, wantErr: true},
		{name: "blank_category", budget: domain.Budget{Category: "  ", Limit: 5000}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.budget.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPeriodValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		period  domain.Period
		wantErr bool
	}{
		{name: "month", period: domain.Period{From: "2025-09-01", To: "2025-09-30"}},
		{name: "single_day", period: domain.Period{From: "2025-09-10", To: "2025-09-10"}},
		{name: "empty_from", period: domain.Period{From: "", To: "2025-09-30"}, wantErr: true},
		{name: "empty_to", period: domain.Period{From: "2025-09-01", To: ""}, wantErr: true},
		{name: "wrong_format", period: domain.Period{From: "01.09.2025", To: "2025-09-30"}, wantErr: true},
		{name: "nonexistent_date", period: domain.Period{From: "2025-09-01", To: "2025-09-31"}, wantErr: true},
		{name: "to_before_from", period: domain.Period{From: "2025-09-30", To: "2025-09-01"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.period.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   domain.Validatable
		wantErr bool
	}{
		{name: "valid_transaction", value: domain.Transaction{Amount: 1, Category: "еда", Date: "2025-09-10"}},
		{name: "valid_budget", value: domain.Budget{Category: "еда", Limit: 1}},
		{name: "valid_period", value: domain.Period{From: "2025-09-01", To: "2025-09-30"}},
		{name: "invalid_transaction", value: domain.Transaction{Amount: -1, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "invalid_budget", value: domain.Budget{Category: "еда", Limit: 0}, wantErr: true},
		{name: "invalid_period", value: domain.Period{From: "2025-09-30", To: "2025-09-01"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := domain.CheckValid(tc.value)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMoneyConversions(t *testing.T) {
	for _, tc := range []struct {
		amount float64
		cents  int64
	}{
		{0, 0}, {0.01, 1}, {0.1, 10}, {0.2, 20}, {450.35, 45035},
		{999_999_999_999.99, 99_999_999_999_999},
	} {
		assert.Equal(t, tc.cents, domain.ToCents(tc.amount))
		assert.Equal(t, tc.amount, domain.FromCents(tc.cents))
	}
	assert.Equal(t, domain.ToCents(0.3), domain.ToCents(0.1)+domain.ToCents(0.2))
}

func TestValidationError(t *testing.T) {
	original := errors.New("invalid data")
	err := &domain.ValidationError{Err: original}
	assert.Equal(t, original.Error(), err.Error())
	assert.ErrorIs(t, err, original)
	var validationErr *domain.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Same(t, original, validationErr.Err)
}
