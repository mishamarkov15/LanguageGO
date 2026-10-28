package ledger

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

const (
	reportCachePrefix = "report:summary:"
	reportCacheTTL    = 30 * time.Second
)

// Period — интервал отчёта, обе даты включительно, в формате ГГГГ-ММ-ДД.
type Period struct {
	From string
	To   string
}

func (p Period) Validate() error {
	from, err := time.Parse(dateLayout, p.From)
	if err != nil {
		return errors.New("параметр from должен быть датой в формате ГГГГ-ММ-ДД")
	}
	to, err := time.Parse(dateLayout, p.To)
	if err != nil {
		return errors.New("параметр to должен быть датой в формате ГГГГ-ММ-ДД")
	}
	if to.Before(from) {
		return errors.New("дата to не может быть раньше даты from")
	}
	return nil
}

type CategoryTotal struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
}

type ReportSummary struct {
	From       string          `json:"from"`
	To         string          `json:"to"`
	Total      float64         `json:"total"`
	Categories []CategoryTotal `json:"categories"`
}

// GetReportSummary считает траты по категориям за период [from, to].
// Результат кешируется в Redis на reportCacheTTL под ключом
// report:summary:<from>:<to> и сбрасывается при добавлении транзакции.
func GetReportSummary(ctx context.Context, from, to string) (ReportSummary, error) {
	if err := (Period{From: from, To: to}).Validate(); err != nil {
		return ReportSummary{}, err
	}

	key := reportCachePrefix + from + ":" + to
	var report ReportSummary
	hit, err := redisCache.GetJSON(ctx, key, &report)
	if err != nil {
		log.Printf("ledger: ошибка чтения кеша %s: %v", key, err)
	}
	if hit {
		log.Printf("ledger: %s — cache hit", key)
		return report, nil
	}

	report, err = queryReportSummary(ctx, from, to)
	if err != nil {
		return ReportSummary{}, err
	}

	if err := redisCache.SetJSON(ctx, key, report, reportCacheTTL); err != nil {
		log.Printf("ledger: ошибка записи кеша %s: %v", key, err)
	} else {
		log.Printf("ledger: %s — cache miss, отчёт посчитан в БД и сохранён на %s", key, reportCacheTTL)
	}
	return report, nil
}

func queryReportSummary(ctx context.Context, from, to string) (ReportSummary, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT category, SUM(amount)
		FROM expenses
		WHERE date BETWEEN $1 AND $2
		GROUP BY category
		ORDER BY category`, from, to)
	if err != nil {
		return ReportSummary{}, fmt.Errorf("расчёт отчёта: %w", err)
	}
	defer rows.Close()

	report := ReportSummary{From: from, To: to, Categories: make([]CategoryTotal, 0)}
	var totalCents int64
	for rows.Next() {
		var item CategoryTotal
		if err := rows.Scan(&item.Category, &item.Total); err != nil {
			return ReportSummary{}, fmt.Errorf("расчёт отчёта: %w", err)
		}
		totalCents += toCents(item.Total)
		report.Categories = append(report.Categories, item)
	}
	if err := rows.Err(); err != nil {
		return ReportSummary{}, fmt.Errorf("расчёт отчёта: %w", err)
	}
	report.Total = fromCents(totalCents)
	return report, nil
}
