package ledger_test

import (
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-6/ledger"
)

// resetLedger очищает хранилище до и после теста. Тесты, которые её вызывают,
// работают с общими данными пакета и поэтому не помечаются t.Parallel().
func resetLedger(t *testing.T) {
	t.Helper()
	ledger.Reset()
	t.Cleanup(ledger.Reset)
}

func TestTransactionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tx      ledger.Transaction
		wantErr bool
	}{
		{
			name: "valid",
			tx:   ledger.Transaction{Amount: 450, Category: "еда", Description: "ланч", Date: "2025-09-10"},
		},
		{
			name: "valid_without_description",
			tx:   ledger.Transaction{Amount: 0.01, Category: "еда", Date: "2025-09-10"},
		},
		{
			name:    "zero_amount",
			tx:      ledger.Transaction{Amount: 0, Category: "еда", Date: "2025-09-10"},
			wantErr: true,
		},
		{
			name:    "negative_amount",
			tx:      ledger.Transaction{Amount: -100, Category: "еда", Date: "2025-09-10"},
			wantErr: true,
		},
		{
			name:    "nan_amount",
			tx:      ledger.Transaction{Amount: math.NaN(), Category: "еда", Date: "2025-09-10"},
			wantErr: true,
		},
		{
			name:    "infinite_amount",
			tx:      ledger.Transaction{Amount: math.Inf(1), Category: "еда", Date: "2025-09-10"},
			wantErr: true,
		},
		{
			name:    "empty_category",
			tx:      ledger.Transaction{Amount: 100, Category: "", Date: "2025-09-10"},
			wantErr: true,
		},
		{
			name:    "blank_category",
			tx:      ledger.Transaction{Amount: 100, Category: "   ", Date: "2025-09-10"},
			wantErr: true,
		},
		{
			name:    "empty_date",
			tx:      ledger.Transaction{Amount: 100, Category: "еда", Date: ""},
			wantErr: true,
		},
		{
			name:    "wrong_date_format",
			tx:      ledger.Transaction{Amount: 100, Category: "еда", Date: "10.09.2025"},
			wantErr: true,
		},
		{
			name:    "nonexistent_date",
			tx:      ledger.Transaction{Amount: 100, Category: "еда", Date: "2025-02-30"},
			wantErr: true,
		},
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
		budget  ledger.Budget
		wantErr bool
	}{
		{name: "positive_limit", budget: ledger.Budget{Category: "еда", Limit: 5000}},
		{name: "fractional_limit", budget: ledger.Budget{Category: "еда", Limit: 0.5}},
		{name: "zero_limit", budget: ledger.Budget{Category: "еда", Limit: 0}, wantErr: true},
		{name: "negative_limit", budget: ledger.Budget{Category: "еда", Limit: -1}, wantErr: true},
		{name: "nan_limit", budget: ledger.Budget{Category: "еда", Limit: math.NaN()}, wantErr: true},
		{name: "infinite_limit", budget: ledger.Budget{Category: "еда", Limit: math.Inf(1)}, wantErr: true},
		{name: "empty_category", budget: ledger.Budget{Category: "", Limit: 5000}, wantErr: true},
		{name: "blank_category", budget: ledger.Budget{Category: "  ", Limit: 5000}, wantErr: true},
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

func TestCheckValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   ledger.Validatable
		wantErr bool
	}{
		{name: "valid_transaction", value: ledger.Transaction{Amount: 1, Category: "еда", Date: "2025-09-10"}},
		{name: "valid_budget", value: ledger.Budget{Category: "еда", Limit: 1}},
		{name: "invalid_transaction", value: ledger.Transaction{Amount: -1, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "invalid_budget", value: ledger.Budget{Category: "еда", Limit: 0}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ledger.CheckValid(tc.value)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAddTransaction(t *testing.T) {
	t.Run("within_limit", func(t *testing.T) {
		resetLedger(t)
		require.NoError(t, ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 5000}))

		saved, err := ledger.AddTransaction(ledger.Transaction{
			Amount: 3000, Category: "еда", Description: "продукты", Date: "2025-09-10",
		})

		require.NoError(t, err)
		assert.Equal(t, ledger.Transaction{
			ID: 1, Amount: 3000, Category: "еда", Description: "продукты", Date: "2025-09-10",
		}, saved)
		assert.Equal(t, []ledger.Transaction{saved}, ledger.ListTransactions())
	})

	t.Run("exactly_limit", func(t *testing.T) {
		resetLedger(t)
		require.NoError(t, ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 5000}))

		_, err := ledger.AddTransaction(ledger.Transaction{Amount: 3000, Category: "еда", Date: "2025-09-10"})
		require.NoError(t, err)
		_, err = ledger.AddTransaction(ledger.Transaction{Amount: 2000, Category: "еда", Date: "2025-09-11"})

		assert.NoError(t, err)
		assert.Len(t, ledger.ListTransactions(), 2)
	})

	t.Run("exceeded", func(t *testing.T) {
		resetLedger(t)
		require.NoError(t, ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 5000}))
		_, err := ledger.AddTransaction(ledger.Transaction{Amount: 4500, Category: "еда", Date: "2025-09-10"})
		require.NoError(t, err)
		before := len(ledger.ListTransactions())

		_, err = ledger.AddTransaction(ledger.Transaction{Amount: 600, Category: "еда", Date: "2025-09-11"})

		assert.ErrorIs(t, err, ledger.ErrBudgetExceeded)
		assert.Len(t, ledger.ListTransactions(), before, "отклонённая транзакция не должна сохраняться")
	})

	t.Run("other_category_does_not_count", func(t *testing.T) {
		resetLedger(t)
		require.NoError(t, ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 1000}))
		_, err := ledger.AddTransaction(ledger.Transaction{Amount: 5000, Category: "техника", Date: "2025-09-10"})
		require.NoError(t, err)

		_, err = ledger.AddTransaction(ledger.Transaction{Amount: 1000, Category: "еда", Date: "2025-09-10"})

		assert.NoError(t, err)
	})

	t.Run("no_budget", func(t *testing.T) {
		resetLedger(t)

		_, err := ledger.AddTransaction(ledger.Transaction{Amount: 1_000_000, Category: "книги", Date: "2025-09-10"})

		assert.NoError(t, err)
		assert.Len(t, ledger.ListTransactions(), 1)
	})

	t.Run("invalid", func(t *testing.T) {
		resetLedger(t)

		_, err := ledger.AddTransaction(ledger.Transaction{Amount: 0, Category: "еда", Date: "2025-09-10"})

		assert.Error(t, err)
		assert.NotErrorIs(t, err, ledger.ErrBudgetExceeded)
		assert.Empty(t, ledger.ListTransactions())
	})

	t.Run("sequential_ids", func(t *testing.T) {
		resetLedger(t)

		for want := 1; want <= 3; want++ {
			saved, err := ledger.AddTransaction(ledger.Transaction{Amount: 10, Category: "еда", Date: "2025-09-10"})
			require.NoError(t, err)
			assert.Equal(t, want, saved.ID)
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		resetLedger(t)
		require.NoError(t, ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 4000}))

		var wg sync.WaitGroup
		var rejected atomic.Int32
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := ledger.AddTransaction(ledger.Transaction{Amount: 100, Category: "еда", Date: "2025-09-10"})
				if errors.Is(err, ledger.ErrBudgetExceeded) {
					rejected.Add(1)
				}
			}()
		}
		wg.Wait()

		assert.Len(t, ledger.ListTransactions(), 40)
		assert.EqualValues(t, 10, rejected.Load())
	})
}

func TestListTransactions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		resetLedger(t)

		list := ledger.ListTransactions()

		assert.NotNil(t, list)
		assert.Empty(t, list)
	})

	t.Run("returns_copy", func(t *testing.T) {
		resetLedger(t)
		_, err := ledger.AddTransaction(ledger.Transaction{Amount: 10, Category: "еда", Date: "2025-09-10"})
		require.NoError(t, err)

		list := ledger.ListTransactions()
		list[0].Amount = 999

		assert.Equal(t, 10.0, ledger.ListTransactions()[0].Amount)
	})
}

func TestSetBudget(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		resetLedger(t)

		err := ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 5000})

		require.NoError(t, err)
		assert.Equal(t, []ledger.Budget{{Category: "еда", Limit: 5000}}, ledger.ListBudgets())
	})

	t.Run("update", func(t *testing.T) {
		resetLedger(t)
		require.NoError(t, ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 5000}))

		err := ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 7000})

		require.NoError(t, err)
		assert.Equal(t, []ledger.Budget{{Category: "еда", Limit: 7000}}, ledger.ListBudgets())
	})

	t.Run("invalid", func(t *testing.T) {
		resetLedger(t)

		err := ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 0})

		assert.Error(t, err)
		assert.Empty(t, ledger.ListBudgets())
	})
}

func TestListBudgets(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		resetLedger(t)

		list := ledger.ListBudgets()

		assert.NotNil(t, list)
		assert.Empty(t, list)
	})

	t.Run("sorted_by_category", func(t *testing.T) {
		resetLedger(t)
		for _, category := range []string{"транспорт", "еда", "книги"} {
			require.NoError(t, ledger.SetBudget(ledger.Budget{Category: category, Limit: 100}))
		}

		list := ledger.ListBudgets()

		assert.Equal(t, []ledger.Budget{
			{Category: "еда", Limit: 100},
			{Category: "книги", Limit: 100},
			{Category: "транспорт", Limit: 100},
		}, list)
	})
}

func TestLoadBudgets(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		resetLedger(t)

		err := ledger.LoadBudgets(strings.NewReader(`[
			{"category": "еда", "limit": 5000},
			{"category": "транспорт", "limit": 1500}
		]`))

		require.NoError(t, err)
		assert.Equal(t, []ledger.Budget{
			{Category: "еда", Limit: 5000},
			{Category: "транспорт", Limit: 1500},
		}, ledger.ListBudgets())
	})

	tests := []struct {
		name  string
		input string
	}{
		{name: "bad_json", input: `[{"category": "еда",`},
		{name: "null", input: `null`},
		{name: "not_array", input: `{"category": "еда", "limit": 5000}`},
		{name: "invalid_budget", input: `[{"category": "еда", "limit": 5000}, {"category": "", "limit": 100}]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetLedger(t)

			err := ledger.LoadBudgets(strings.NewReader(tc.input))

			assert.Error(t, err)
			assert.Empty(t, ledger.ListBudgets(), "при ошибке бюджеты не должны загружаться частично")
		})
	}

	t.Run("read_error", func(t *testing.T) {
		resetLedger(t)

		err := ledger.LoadBudgets(iotest.ErrReader(errors.New("disk error")))

		assert.Error(t, err)
	})
}

func TestReset(t *testing.T) {
	resetLedger(t)
	require.NoError(t, ledger.SetBudget(ledger.Budget{Category: "еда", Limit: 5000}))
	_, err := ledger.AddTransaction(ledger.Transaction{Amount: 10, Category: "еда", Date: "2025-09-10"})
	require.NoError(t, err)

	ledger.Reset()

	assert.Empty(t, ledger.ListTransactions())
	assert.Empty(t, ledger.ListBudgets())
	saved, err := ledger.AddTransaction(ledger.Transaction{Amount: 10, Category: "еда", Date: "2025-09-10"})
	require.NoError(t, err)
	assert.Equal(t, 1, saved.ID, "после Reset нумерация начинается заново")
}
