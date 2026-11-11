// Package app собирает Ledger из PostgreSQL, Redis и сервисного слоя.
package app

import (
	"context"
	"errors"
	"fmt"

	"hw-9/ledger/internal/cache"
	"hw-9/ledger/internal/db"
	"hw-9/ledger/internal/repository/pg"
	"hw-9/ledger/internal/service"
)

// New читает настройки окружения и подключает зависимости. Миграции
// выполняются отдельно перед запуском приложения.
func New(ctx context.Context) (service.Service, func() error, error) {
	cfg, err := cache.ConfigFromEnv()
	if err != nil {
		return nil, nil, err
	}
	database, err := db.Open(ctx, db.DSNFromEnv())
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось подключиться к PostgreSQL: %w", err)
	}
	redisCache, err := cache.New(ctx, cfg)
	if err != nil {
		closeErr := database.Close()
		return nil, nil, errors.Join(fmt.Errorf("не удалось подключиться к Redis: %w", err), closeErr)
	}

	store := pg.New(database)
	svc := service.New(store.Budgets(), store.Expenses(), store, redisCache)
	close := func() error {
		return errors.Join(redisCache.Close(), database.Close())
	}
	return svc, close, nil
}
