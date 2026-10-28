package ledger_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-7/ledger"
)

func TestMain(m *testing.M) {
	// Логи подключений и кеша не нужны в отчёте тестов.
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// testDSN возвращает строку подключения к тестовой базе или пропускает тест.
// Reset очищает таблицы, поэтому нужна отдельная база, а не DATABASE_URL.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задана: тест с PostgreSQL пропущен")
	}
	return dsn
}

// setupLedger подключает ledger к тестовой базе и к Redis в памяти (miniredis)
// и очищает данные. Тесты работают с общей базой, поэтому не параллельные.
func setupLedger(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	dsn := testDSN(t)
	mr := miniredis.RunT(t)
	ctx := context.Background()

	require.NoError(t, ledger.Init(ctx, ledger.Config{DatabaseURL: dsn, RedisAddr: mr.Addr()}))
	t.Cleanup(func() { ledger.Close() })
	require.NoError(t, ledger.Reset(ctx))
	return mr
}

// rawDB открывает отдельное соединение, чтобы менять данные в обход ledger
// и его кеша.
func rawDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func mustSetBudget(t *testing.T, category string, limit float64) {
	t.Helper()
	require.NoError(t, ledger.SetBudget(context.Background(), ledger.Budget{Category: category, Limit: limit}))
}

func mustAdd(t *testing.T, amount float64, category, date string) ledger.Transaction {
	t.Helper()
	saved, err := ledger.AddTransaction(context.Background(),
		ledger.Transaction{Amount: amount, Category: category, Date: date})
	require.NoError(t, err)
	return saved
}

func mustListTransactions(t *testing.T) []ledger.Transaction {
	t.Helper()
	list, err := ledger.ListTransactions(context.Background())
	require.NoError(t, err)
	return list
}

func mustListBudgets(t *testing.T) []ledger.Budget {
	t.Helper()
	list, err := ledger.ListBudgets(context.Background())
	require.NoError(t, err)
	return list
}

func TestTransactionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tx      ledger.Transaction
		wantErr bool
	}{
		{name: "valid", tx: ledger.Transaction{Amount: 450, Category: "еда", Description: "ланч", Date: "2025-09-10"}},
		{name: "valid_without_description", tx: ledger.Transaction{Amount: 0.01, Category: "еда", Date: "2025-09-10"}},
		{name: "two_decimals", tx: ledger.Transaction{Amount: 450.35, Category: "еда", Date: "2025-09-10"}},
		{name: "max_amount", tx: ledger.Transaction{Amount: 999_999_999_999.99, Category: "еда", Date: "2025-09-10"}},
		{name: "zero_amount", tx: ledger.Transaction{Amount: 0, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "negative_amount", tx: ledger.Transaction{Amount: -100, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "nan_amount", tx: ledger.Transaction{Amount: math.NaN(), Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "infinite_amount", tx: ledger.Transaction{Amount: math.Inf(1), Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "too_large_amount", tx: ledger.Transaction{Amount: 1e12, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "three_decimals", tx: ledger.Transaction{Amount: 1.234, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "empty_category", tx: ledger.Transaction{Amount: 100, Category: "", Date: "2025-09-10"}, wantErr: true},
		{name: "blank_category", tx: ledger.Transaction{Amount: 100, Category: "   ", Date: "2025-09-10"}, wantErr: true},
		{name: "empty_date", tx: ledger.Transaction{Amount: 100, Category: "еда", Date: ""}, wantErr: true},
		{name: "wrong_date_format", tx: ledger.Transaction{Amount: 100, Category: "еда", Date: "10.09.2025"}, wantErr: true},
		{name: "nonexistent_date", tx: ledger.Transaction{Amount: 100, Category: "еда", Date: "2025-02-30"}, wantErr: true},
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
		{name: "max_limit", budget: ledger.Budget{Category: "еда", Limit: 999_999_999_999.99}},
		{name: "zero_limit", budget: ledger.Budget{Category: "еда", Limit: 0}, wantErr: true},
		{name: "negative_limit", budget: ledger.Budget{Category: "еда", Limit: -1}, wantErr: true},
		{name: "nan_limit", budget: ledger.Budget{Category: "еда", Limit: math.NaN()}, wantErr: true},
		{name: "infinite_limit", budget: ledger.Budget{Category: "еда", Limit: math.Inf(1)}, wantErr: true},
		{name: "too_large_limit", budget: ledger.Budget{Category: "еда", Limit: 1e12}, wantErr: true},
		{name: "three_decimals", budget: ledger.Budget{Category: "еда", Limit: 0.001}, wantErr: true},
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

func TestPeriodValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		period  ledger.Period
		wantErr bool
	}{
		{name: "month", period: ledger.Period{From: "2025-09-01", To: "2025-09-30"}},
		{name: "single_day", period: ledger.Period{From: "2025-09-10", To: "2025-09-10"}},
		{name: "empty_from", period: ledger.Period{From: "", To: "2025-09-30"}, wantErr: true},
		{name: "empty_to", period: ledger.Period{From: "2025-09-01", To: ""}, wantErr: true},
		{name: "wrong_format", period: ledger.Period{From: "01.09.2025", To: "2025-09-30"}, wantErr: true},
		{name: "nonexistent_date", period: ledger.Period{From: "2025-09-01", To: "2025-09-31"}, wantErr: true},
		{name: "to_before_from", period: ledger.Period{From: "2025-09-30", To: "2025-09-01"}, wantErr: true},
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
		value   ledger.Validatable
		wantErr bool
	}{
		{name: "valid_transaction", value: ledger.Transaction{Amount: 1, Category: "еда", Date: "2025-09-10"}},
		{name: "valid_budget", value: ledger.Budget{Category: "еда", Limit: 1}},
		{name: "valid_period", value: ledger.Period{From: "2025-09-01", To: "2025-09-30"}},
		{name: "invalid_transaction", value: ledger.Transaction{Amount: -1, Category: "еда", Date: "2025-09-10"}, wantErr: true},
		{name: "invalid_budget", value: ledger.Budget{Category: "еда", Limit: 0}, wantErr: true},
		{name: "invalid_period", value: ledger.Period{From: "2025-09-30", To: "2025-09-01"}, wantErr: true},
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

func TestInit(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid_dsn", func(t *testing.T) {
		err := ledger.Init(ctx, ledger.Config{DatabaseURL: "postgres://localhost:bad-port/db", RedisAddr: "localhost:6379"})

		assert.ErrorContains(t, err, "PostgreSQL")
	})

	t.Run("redis_unreachable", func(t *testing.T) {
		dsn := testDSN(t)
		mr := miniredis.RunT(t)
		addr := mr.Addr()
		mr.Close()

		err := ledger.Init(ctx, ledger.Config{DatabaseURL: dsn, RedisAddr: addr})

		assert.ErrorContains(t, err, "Redis")
	})
}

func TestConfigFromEnv(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/cashapp?sslmode=disable")
		t.Setenv("REDIS_ADDR", "redis:6379")
		t.Setenv("REDIS_DB", "1")
		t.Setenv("REDIS_PASSWORD", "secret")

		cfg, err := ledger.ConfigFromEnv()

		require.NoError(t, err)
		assert.Equal(t, ledger.Config{
			DatabaseURL:   "postgres://user:pass@db:5432/cashapp?sslmode=disable",
			RedisAddr:     "redis:6379",
			RedisDB:       1,
			RedisPassword: "secret",
		}, cfg)
	})

	t.Run("bad_redis_db", func(t *testing.T) {
		t.Setenv("REDIS_DB", "one")

		_, err := ledger.ConfigFromEnv()

		assert.Error(t, err)
	})
}

func TestSetBudget(t *testing.T) {
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		setupLedger(t)

		require.NoError(t, ledger.SetBudget(ctx, ledger.Budget{Category: "еда", Limit: 5000}))

		assert.Equal(t, []ledger.Budget{{Category: "еда", Limit: 5000}}, mustListBudgets(t))
	})

	t.Run("upsert", func(t *testing.T) {
		setupLedger(t)
		mustSetBudget(t, "еда", 5000)

		require.NoError(t, ledger.SetBudget(ctx, ledger.Budget{Category: "еда", Limit: 7000.5}))

		var rows int
		require.NoError(t, rawDB(t).QueryRow(`SELECT COUNT(*) FROM budgets WHERE category = 'еда'`).Scan(&rows))
		assert.Equal(t, 1, rows, "UPSERT не должен создавать дубль категории")
		assert.Equal(t, []ledger.Budget{{Category: "еда", Limit: 7000.5}}, mustListBudgets(t))
	})

	t.Run("invalid", func(t *testing.T) {
		setupLedger(t)

		err := ledger.SetBudget(ctx, ledger.Budget{Category: "еда", Limit: 0})

		assert.Error(t, err)
		assert.Empty(t, mustListBudgets(t))
	})

	t.Run("drops_cache", func(t *testing.T) {
		mr := setupLedger(t)
		mustSetBudget(t, "еда", 5000)
		mustListBudgets(t)
		require.True(t, mr.Exists("budgets:all"))

		mustSetBudget(t, "транспорт", 1000)

		assert.False(t, mr.Exists("budgets:all"))
		assert.Len(t, mustListBudgets(t), 2)
	})
}

func TestListBudgets(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		setupLedger(t)

		list := mustListBudgets(t)

		assert.NotNil(t, list)
		assert.Empty(t, list)
	})

	t.Run("sorted_by_category", func(t *testing.T) {
		setupLedger(t)
		for _, category := range []string{"транспорт", "еда", "книги"} {
			mustSetBudget(t, category, 100)
		}

		assert.Equal(t, []ledger.Budget{
			{Category: "еда", Limit: 100},
			{Category: "книги", Limit: 100},
			{Category: "транспорт", Limit: 100},
		}, mustListBudgets(t))
	})

	t.Run("cached_with_ttl", func(t *testing.T) {
		mr := setupLedger(t)
		mustSetBudget(t, "еда", 5000)
		mustListBudgets(t)
		assert.Equal(t, 30*time.Second, mr.TTL("budgets:all"))

		// Изменение в обход ledger не видно, пока кеш жив.
		_, err := rawDB(t).Exec(`UPDATE budgets SET limit_amount = 1 WHERE category = 'еда'`)
		require.NoError(t, err)
		assert.Equal(t, []ledger.Budget{{Category: "еда", Limit: 5000}}, mustListBudgets(t))

		mr.FastForward(31 * time.Second)
		assert.Equal(t, []ledger.Budget{{Category: "еда", Limit: 1}}, mustListBudgets(t))
	})
}

func TestAddTransaction(t *testing.T) {
	ctx := context.Background()

	t.Run("within_limit", func(t *testing.T) {
		setupLedger(t)
		mustSetBudget(t, "еда", 5000)

		saved, err := ledger.AddTransaction(ctx, ledger.Transaction{
			Amount: 3000, Category: "еда", Description: "продукты", Date: "2025-09-10",
		})

		require.NoError(t, err)
		assert.Equal(t, ledger.Transaction{
			ID: 1, Amount: 3000, Category: "еда", Description: "продукты", Date: "2025-09-10",
		}, saved)
		assert.Equal(t, []ledger.Transaction{saved}, mustListTransactions(t))
	})

	t.Run("exactly_limit", func(t *testing.T) {
		setupLedger(t)
		mustSetBudget(t, "еда", 5000)
		mustAdd(t, 3000, "еда", "2025-09-10")

		_, err := ledger.AddTransaction(ctx, ledger.Transaction{Amount: 2000, Category: "еда", Date: "2025-09-11"})

		assert.NoError(t, err)
		assert.Len(t, mustListTransactions(t), 2)
	})

	t.Run("exceeded", func(t *testing.T) {
		setupLedger(t)
		mustSetBudget(t, "еда", 5000)
		mustAdd(t, 4500, "еда", "2025-09-10")
		before := len(mustListTransactions(t))

		_, err := ledger.AddTransaction(ctx, ledger.Transaction{Amount: 600, Category: "еда", Date: "2025-09-11"})

		assert.ErrorIs(t, err, ledger.ErrBudgetExceeded)
		assert.Len(t, mustListTransactions(t), before, "отклонённая транзакция не должна сохраняться")
	})

	t.Run("cents_precision", func(t *testing.T) {
		setupLedger(t)
		mustSetBudget(t, "еда", 0.3)
		mustAdd(t, 0.1, "еда", "2025-09-10")

		_, err := ledger.AddTransaction(ctx, ledger.Transaction{Amount: 0.2, Category: "еда", Date: "2025-09-10"})
		require.NoError(t, err, "0.1 + 0.2 укладывается в лимит 0.3")

		_, err = ledger.AddTransaction(ctx, ledger.Transaction{Amount: 0.01, Category: "еда", Date: "2025-09-10"})
		assert.ErrorIs(t, err, ledger.ErrBudgetExceeded)
	})

	t.Run("other_category_does_not_count", func(t *testing.T) {
		setupLedger(t)
		mustSetBudget(t, "еда", 1000)
		mustAdd(t, 5000, "техника", "2025-09-10")

		_, err := ledger.AddTransaction(ctx, ledger.Transaction{Amount: 1000, Category: "еда", Date: "2025-09-10"})

		assert.NoError(t, err)
	})

	t.Run("no_budget", func(t *testing.T) {
		setupLedger(t)

		_, err := ledger.AddTransaction(ctx, ledger.Transaction{Amount: 1_000_000, Category: "книги", Date: "2025-09-10"})

		assert.NoError(t, err)
		assert.Len(t, mustListTransactions(t), 1)
	})

	t.Run("invalid", func(t *testing.T) {
		setupLedger(t)

		_, err := ledger.AddTransaction(ctx, ledger.Transaction{Amount: 0, Category: "еда", Date: "2025-09-10"})

		assert.Error(t, err)
		assert.NotErrorIs(t, err, ledger.ErrBudgetExceeded)
		assert.Empty(t, mustListTransactions(t))
	})

	t.Run("sequential_ids", func(t *testing.T) {
		setupLedger(t)

		for want := 1; want <= 3; want++ {
			assert.Equal(t, want, mustAdd(t, 10, "еда", "2025-09-10").ID)
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		setupLedger(t)
		mustSetBudget(t, "еда", 4000)

		var wg sync.WaitGroup
		var rejected atomic.Int32
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := ledger.AddTransaction(ctx, ledger.Transaction{Amount: 100, Category: "еда", Date: "2025-09-10"})
				if errors.Is(err, ledger.ErrBudgetExceeded) {
					rejected.Add(1)
				} else {
					assert.NoError(t, err)
				}
			}()
		}
		wg.Wait()

		assert.Len(t, mustListTransactions(t), 40)
		assert.EqualValues(t, 10, rejected.Load())
	})
}

func TestListTransactions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		setupLedger(t)

		list := mustListTransactions(t)

		assert.NotNil(t, list)
		assert.Empty(t, list)
	})

	t.Run("newest_first", func(t *testing.T) {
		setupLedger(t)
		first := mustAdd(t, 10, "еда", "2025-09-10")
		older := mustAdd(t, 20, "еда", "2025-09-01")
		sameDay := mustAdd(t, 30, "еда", "2025-09-10")

		assert.Equal(t, []ledger.Transaction{sameDay, first, older}, mustListTransactions(t))
	})

	t.Run("null_description", func(t *testing.T) {
		setupLedger(t)
		_, err := rawDB(t).Exec(`INSERT INTO expenses (amount, category, description, date) VALUES (5, 'еда', NULL, '2025-09-10')`)
		require.NoError(t, err)

		list := mustListTransactions(t)

		require.Len(t, list, 1)
		assert.Equal(t, "", list[0].Description)
	})
}

func TestGetReportSummary(t *testing.T) {
	ctx := context.Background()
	const key = "report:summary:2025-09-01:2025-09-30"

	t.Run("totals_by_category", func(t *testing.T) {
		setupLedger(t)
		mustAdd(t, 450.10, "еда", "2025-09-01")
		mustAdd(t, 0.2, "еда", "2025-09-30")
		mustAdd(t, 65.5, "транспорт", "2025-09-15")
		mustAdd(t, 1000, "еда", "2025-08-31")
		mustAdd(t, 1000, "книги", "2025-10-01")

		report, err := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")

		require.NoError(t, err)
		assert.Equal(t, ledger.ReportSummary{
			From:  "2025-09-01",
			To:    "2025-09-30",
			Total: 515.8,
			Categories: []ledger.CategoryTotal{
				{Category: "еда", Total: 450.3},
				{Category: "транспорт", Total: 65.5},
			},
		}, report)
	})

	t.Run("empty_period", func(t *testing.T) {
		setupLedger(t)

		report, err := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")

		require.NoError(t, err)
		assert.Zero(t, report.Total)
		assert.NotNil(t, report.Categories)
		assert.Empty(t, report.Categories)
	})

	t.Run("cache_hit_until_ttl", func(t *testing.T) {
		mr := setupLedger(t)
		mustAdd(t, 100, "еда", "2025-09-10")

		first, err := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, mr.TTL(key))

		// Трата в обход ledger не сбрасывает кеш: пока TTL не истёк, отчёт берётся из Redis.
		_, err = rawDB(t).Exec(`INSERT INTO expenses (amount, category, date) VALUES (50, 'еда', '2025-09-11')`)
		require.NoError(t, err)
		cached, err := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
		require.NoError(t, err)
		assert.Equal(t, first, cached)

		mr.FastForward(31 * time.Second)
		fresh, err := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
		require.NoError(t, err)
		assert.Equal(t, 150.0, fresh.Total)
	})

	t.Run("add_transaction_drops_cache", func(t *testing.T) {
		mr := setupLedger(t)
		mustAdd(t, 100, "еда", "2025-09-10")
		_, err := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
		require.NoError(t, err)
		require.True(t, mr.Exists(key))

		mustAdd(t, 50, "еда", "2025-09-11")

		assert.False(t, mr.Exists(key))
		report, err := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
		require.NoError(t, err)
		assert.Equal(t, 150.0, report.Total)
	})

	t.Run("invalid_period", func(t *testing.T) {
		setupLedger(t)

		_, err := ledger.GetReportSummary(ctx, "2025-09-30", "2025-09-01")

		assert.Error(t, err)
	})
}

func TestRedisUnavailable(t *testing.T) {
	ctx := context.Background()
	mr := setupLedger(t)
	mustSetBudget(t, "еда", 1000)
	mr.Close()

	require.NoError(t, ledger.SetBudget(ctx, ledger.Budget{Category: "еда", Limit: 2000}))
	_, err := ledger.AddTransaction(ctx, ledger.Transaction{Amount: 1500, Category: "еда", Date: "2025-09-10"})
	require.NoError(t, err)
	report, err := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
	require.NoError(t, err)

	assert.Equal(t, 1500.0, report.Total)
	assert.Equal(t, []ledger.Budget{{Category: "еда", Limit: 2000}}, mustListBudgets(t))
}

func TestDatabaseUnavailable(t *testing.T) {
	ctx := context.Background()
	setupLedger(t)
	require.NoError(t, ledger.Close())

	_, listTxErr := ledger.ListTransactions(ctx)
	_, listBudgetsErr := ledger.ListBudgets(ctx)
	_, addErr := ledger.AddTransaction(ctx, ledger.Transaction{Amount: 1, Category: "еда", Date: "2025-09-10"})
	_, reportErr := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")

	assert.Error(t, ledger.SetBudget(ctx, ledger.Budget{Category: "еда", Limit: 1}))
	assert.Error(t, listTxErr)
	assert.Error(t, listBudgetsErr)
	assert.Error(t, addErr)
	assert.NotErrorIs(t, addErr, ledger.ErrBudgetExceeded)
	assert.Error(t, reportErr)
	assert.Error(t, ledger.Reset(ctx))
}

func TestDataSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	setupLedger(t)
	mustSetBudget(t, "еда", 5000)
	saved := mustAdd(t, 450, "еда", "2025-09-10")

	require.NoError(t, ledger.Close())
	mr := miniredis.RunT(t)
	require.NoError(t, ledger.Init(ctx, ledger.Config{DatabaseURL: testDSN(t), RedisAddr: mr.Addr()}))

	assert.Equal(t, []ledger.Budget{{Category: "еда", Limit: 5000}}, mustListBudgets(t))
	assert.Equal(t, []ledger.Transaction{saved}, mustListTransactions(t))
}

func TestReset(t *testing.T) {
	ctx := context.Background()
	mr := setupLedger(t)
	mustSetBudget(t, "еда", 5000)
	mustAdd(t, 10, "еда", "2025-09-10")
	mustListBudgets(t)
	_, err := ledger.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
	require.NoError(t, err)

	require.NoError(t, ledger.Reset(ctx))

	assert.Empty(t, mr.Keys())
	assert.Empty(t, mustListTransactions(t))
	assert.Empty(t, mustListBudgets(t))
	assert.Equal(t, 1, mustAdd(t, 10, "еда", "2025-09-10").ID, "после Reset нумерация начинается заново")
}
