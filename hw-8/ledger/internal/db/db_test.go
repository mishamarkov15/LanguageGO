package db

import (
	"context"
	"io"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func TestDSNFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "defaults",
			want: "postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable",
		},
		{
			name: "database_url_has_priority",
			env: map[string]string{
				"DATABASE_URL": "postgres://user:pass@db:5433/cashcraft?sslmode=disable",
				"DB_HOST":      "ignored",
			},
			want: "postgres://user:pass@db:5433/cashcraft?sslmode=disable",
		},
		{
			name: "separate_variables",
			env: map[string]string{
				"DB_HOST": "db.local",
				"DB_PORT": "6543",
				"DB_USER": "cash",
				"DB_PASS": "p@ss:word",
				"DB_NAME": "ledger",
			},
			want: "postgres://cash:p%40ss%3Aword@db.local:6543/ledger?sslmode=disable",
		},
		{
			name: "partial_variables",
			env:  map[string]string{"DB_NAME": "cashapp_dev"},
			want: "postgres://postgres:postgres@localhost:5432/cashapp_dev?sslmode=disable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASS", "DB_NAME"} {
				t.Setenv(key, tc.env[key])
			}

			assert.Equal(t, tc.want, DSNFromEnv())
		})
	}
}

func TestOpen(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid_dsn", func(t *testing.T) {
		_, err := Open(ctx, "postgres://user:pass@localhost:not-a-port/db")

		assert.ErrorContains(t, err, "некорректная строка подключения")
	})

	t.Run("unreachable", func(t *testing.T) {
		_, err := Open(ctx, "postgres://postgres:postgres@127.0.0.1:1/cashapp?sslmode=disable&connect_timeout=2")

		assert.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		dsn := os.Getenv("TEST_DATABASE_URL")
		if dsn == "" {
			t.Skip("TEST_DATABASE_URL не задана: тест с PostgreSQL пропущен")
		}

		db, err := Open(ctx, dsn)

		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		assert.Equal(t, 10, db.Stats().MaxOpenConnections)
		assert.NoError(t, db.PingContext(ctx))
	})
}
