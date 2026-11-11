package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInvalidRedisConfig(t *testing.T) {
	t.Setenv("REDIS_DB", "invalid")
	svc, close, err := New(context.Background())
	require.ErrorContains(t, err, "REDIS_DB")
	assert.Nil(t, svc)
	assert.Nil(t, close)
}

func TestNewInvalidDatabaseConfig(t *testing.T) {
	t.Setenv("REDIS_DB", "0")
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost:invalid/test")
	svc, close, err := New(context.Background())
	require.ErrorContains(t, err, "PostgreSQL")
	assert.Nil(t, svc)
	assert.Nil(t, close)
}

func appTestDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задана: тест с PostgreSQL пропущен")
	}
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close() })
	tag := fmt.Sprintf("hw9_app_%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+tag)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := admin.ExecContext(cleanupCtx, "DROP SCHEMA "+tag+" CASCADE")
		assert.NoError(t, err)
	})

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		require.NoError(t, err)
		q := u.Query()
		q.Set("search_path", tag)
		q.Set("application_name", tag)
		u.RawQuery = q.Encode()
		dsn = u.String()
	} else {
		dsn += " search_path=" + tag + " application_name=" + tag
	}
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("REDIS_DB", "0")
	t.Setenv("REDIS_PASSWORD", "")
	return admin, tag
}

func assertConnectionsClosed(t *testing.T, admin *sql.DB, application string) {
	t.Helper()
	assert.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var count int
		err := admin.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pg_stat_activity WHERE application_name = $1`, application).Scan(&count)
		return err == nil && count == 0
	}, 3*time.Second, 20*time.Millisecond, "фабрика должна закрыть соединения PostgreSQL")
}

func TestNewDoesNotMigrateAndClosesResources(t *testing.T) {
	admin, tag := appTestDatabase(t)
	mr := miniredis.RunT(t)
	t.Setenv("REDIS_ADDR", mr.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc, close, err := New(ctx)
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, close)
	t.Cleanup(func() { close() })

	// Новая схема пустая: New подключает зависимости, но не создаёт таблицы.
	_, err = svc.ListBudgets(ctx)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "42P01", pgErr.Code)
	require.NoError(t, close())
	assertConnectionsClosed(t, admin, tag)
}

func TestNewClosesDatabaseWhenRedisFails(t *testing.T) {
	admin, tag := appTestDatabase(t)
	t.Setenv("REDIS_ADDR", "127.0.0.1:1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc, close, err := New(ctx)
	require.ErrorContains(t, err, "Redis")
	assert.Nil(t, svc)
	assert.Nil(t, close)
	assertConnectionsClosed(t, admin, tag)
}
