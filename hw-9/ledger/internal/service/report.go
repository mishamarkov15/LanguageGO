package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"hw-9/ledger/internal/domain"
)

func (s *ledgerService) calculateReport(ctx context.Context, period domain.Period) (domain.ReportSummary, error) {
	work, cancel := context.WithCancelCause(ctx)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(400 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-ticker.C:
				log.Print("ledger: идёт расчёт отчёта")
			}
		}
	}()
	defer func() {
		cause := context.Cause(work)
		cancel(nil)
		<-heartbeatDone
		if cause != nil {
			log.Printf("ledger: расчёт прерван: %v; heartbeat остановлен", cause)
		} else {
			log.Print("ledger: heartbeat остановлен")
		}
	}()

	categories, err := s.expenses.CategoriesInPeriod(work, period)
	if err != nil {
		cancel(fmt.Errorf("получение категорий: %w", err))
		return domain.ReportSummary{}, context.Cause(work)
	}
	totals := make([]domain.CategoryTotal, len(categories))
	var workers sync.WaitGroup
	for i, category := range categories {
		if work.Err() != nil {
			break
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			if work.Err() != nil {
				return
			}
			sum, err := s.expenses.SumInPeriod(work, category, period)
			if err != nil {
				cancel(fmt.Errorf("сумма по категории %q: %w", category, err))
				return
			}
			// Каждая горутина записывает только свой элемент.
			totals[i] = domain.CategoryTotal{Category: category, Total: sum}
		}()
	}
	workers.Wait()
	if err := context.Cause(work); err != nil {
		return domain.ReportSummary{}, err
	}
	var cents int64
	for _, item := range totals {
		cents += domain.ToCents(item.Total)
	}
	return domain.ReportSummary{From: period.From, To: period.To, Categories: totals, Total: domain.FromCents(cents)}, nil
}
