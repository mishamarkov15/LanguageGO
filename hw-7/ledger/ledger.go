// Package ledger хранит транзакции и бюджеты в PostgreSQL и кеширует отчёты в Redis.
// Перед использованием пакета нужно вызвать Init.
package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"hw-7/ledger/internal/cache"
	"hw-7/ledger/internal/db"
)

// На этом этапе подключения хранятся в переменных пакета.
// В теме 8 они переедут в слой репозиториев.
var (
	database   *sql.DB
	redisCache *cache.Cache
)

type Config struct {
	DatabaseURL   string
	RedisAddr     string
	RedisDB       int
	RedisPassword string
}

// ConfigFromEnv читает настройки из переменных окружения: DATABASE_URL
// (или DB_HOST, DB_PORT, DB_USER, DB_PASS, DB_NAME), REDIS_ADDR, REDIS_DB, REDIS_PASSWORD.
func ConfigFromEnv() (Config, error) {
	redisCfg, err := cache.ConfigFromEnv()
	if err != nil {
		return Config{}, err
	}
	return Config{
		DatabaseURL:   db.DSNFromEnv(),
		RedisAddr:     redisCfg.Addr,
		RedisDB:       redisCfg.DB,
		RedisPassword: redisCfg.Password,
	}, nil
}

// Init подключается к PostgreSQL и Redis. Схема БД должна быть заранее
// создана миграциями goose.
func Init(ctx context.Context, cfg Config) error {
	sqlDB, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("не удалось подключиться к PostgreSQL: %w", err)
	}

	c, err := cache.New(ctx, cache.Config{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
	if err != nil {
		sqlDB.Close()
		return fmt.Errorf("не удалось подключиться к Redis: %w", err)
	}

	database, redisCache = sqlDB, c
	return nil
}

func Close() error {
	var errs []error
	if redisCache != nil {
		errs = append(errs, redisCache.Close())
	}
	if database != nil {
		errs = append(errs, database.Close())
	}
	return errors.Join(errs...)
}

// Reset удаляет все транзакции и бюджеты и очищает кеш Ledger.
// Нужна только тестам: вызывайте её на отдельной тестовой базе.
func Reset(ctx context.Context) error {
	if _, err := database.ExecContext(ctx, `TRUNCATE expenses, budgets RESTART IDENTITY`); err != nil {
		return fmt.Errorf("очистка таблиц: %w", err)
	}
	if err := redisCache.Delete(ctx, budgetsCacheKey); err != nil {
		return fmt.Errorf("очистка кеша: %w", err)
	}
	if err := redisCache.DeleteByPrefix(ctx, reportCachePrefix); err != nil {
		return fmt.Errorf("очистка кеша: %w", err)
	}
	return nil
}
