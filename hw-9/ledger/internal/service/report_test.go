package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hw-9/ledger/internal/domain"
	"hw-9/ledger/internal/service"
)

func TestReportWorkers(t *testing.T) {
	failure := errors.New("category query failed")
	period := domain.Period{From: "2026-01-01", To: "2026-01-31"}
	categories := []string{"еда", "книги", "транспорт"}
	amounts := map[string]float64{"еда": 1.01, "книги": 2.02, "транспорт": 3.03}
	for _, tc := range []struct {
		name    string
		wantErr error
	}{
		{"complete", nil},
		{"cancel", context.Canceled},
		{"query_error", failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			started := make(chan string, len(categories))
			finished := make(chan string, len(categories))
			release := make(chan struct{})
			expenses := &expenseStub{
				categories: func(_ context.Context, p domain.Period) ([]string, error) {
					assert.Equal(t, period, p)
					return categories, nil
				},
				periodSum: func(ctx context.Context, category string, p domain.Period) (float64, error) {
					defer func() { finished <- category }()
					assert.Equal(t, period, p)
					started <- category
					if tc.wantErr == failure && category != "еда" {
						<-ctx.Done()
						return 0, ctx.Err()
					}
					select {
					case <-ctx.Done():
						return 0, ctx.Err()
					case <-release:
						return amounts[category], tc.wantErr
					}
				},
			}
			cache := newCache()
			svc := service.New(nil, expenses, nil, cache)
			done := make(chan struct{})
			var report domain.ReportSummary
			var err error
			go func() {
				defer close(done)
				report, err = svc.GetReportSummary(ctx, period.From, period.To)
			}()
			var running []string
			for range categories {
				select {
				case category := <-started:
					running = append(running, category)
				case <-ctx.Done():
					t.Fatal("category queries did not start concurrently")
				}
			}
			require.ElementsMatch(t, categories, running)
			if tc.wantErr == context.Canceled {
				cancel()
			} else {
				close(release)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("report did not finish")
			}
			require.Len(t, finished, len(categories))
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Zero(t, report)
				assert.Empty(t, cache.data)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 6.06, report.Total)
			assert.Equal(t, []domain.CategoryTotal{
				{Category: "еда", Total: 1.01}, {Category: "книги", Total: 2.02}, {Category: "транспорт", Total: 3.03},
			}, report.Categories)
		})
	}
}
