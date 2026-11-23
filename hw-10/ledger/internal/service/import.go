package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"

	"hw-10/ledger/internal/domain"
)

type importJob struct {
	index int
	tx    domain.Transaction
}

type importResult struct {
	index int
	err   error
}

func (s *ledgerService) ImportTransactions(ctx context.Context, items []domain.Transaction, workers int) (domain.ImportSummary, error) {
	if workers < 1 || workers > 32 {
		return domain.ImportSummary{}, &domain.ValidationError{Err: errors.New("workers должен быть целым числом от 1 до 32")}
	}
	jobs := make(chan importJob)
	results := make(chan importResult)
	done := make(chan domain.ImportSummary)
	go collectImportResults(results, done)

	var wg sync.WaitGroup
	for range min(workers, len(items)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if ctx.Err() != nil {
					return
				}
				_, err := s.AddTransaction(ctx, job.tx)
				if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
					return
				}
				// Коллектор дочитает завершённые операции даже после отмены.
				results <- importResult{index: job.index, err: err}
			}
		}()
	}

dispatch:
	for i, tx := range items {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- importJob{index: i, tx: tx}:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	return <-done, ctx.Err()
}

func collectImportResults(results <-chan importResult, done chan<- domain.ImportSummary) {
	var accepted, rejected atomic.Int64
	var failures []domain.ImportResultError
	for r := range results {
		if r.err == nil {
			accepted.Add(1)
		} else {
			rejected.Add(1)
			failures = append(failures, domain.ImportResultError{Index: r.index, Err: r.err})
		}
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].Index < failures[j].Index })
	done <- domain.ImportSummary{Accepted: accepted.Load(), Rejected: rejected.Load(), Errors: failures}
}
