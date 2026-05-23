package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DashboardService struct {
	db     *pgxpool.Pool
	router *PaymentRouter
}

func NewDashboardService(db *pgxpool.Pool, router *PaymentRouter) *DashboardService {
	return &DashboardService{
		db:     db,
		router: router,
	}
}

// DashboardStats holds aggregate metrics for the admin dashboard.
type DashboardStats struct {
	TotalPayments    int64          `json:"total_payments"`
	TotalRevenue     int64          `json:"total_revenue"`
	SuccessRate      float64        `json:"success_rate"`
	FailureRate      float64        `json:"failure_rate"`
	AvgPaymentAmount float64        `json:"avg_payment_amount"`
	StatusBreakdown  map[string]int `json:"status_breakdown"`
	CurrencyBreakdown map[string]int64 `json:"currency_breakdown"`
	PSPBreakdown     map[string]int `json:"psp_breakdown"`
	TodayPayments    int64          `json:"today_payments"`
	TodayRevenue     int64          `json:"today_revenue"`
	PeriodStart      time.Time      `json:"period_start"`
	PeriodEnd        time.Time      `json:"period_end"`
}

// GetStats returns aggregate dashboard metrics for a given time range.
// If no range is specified, defaults to last 30 days.
func (s *DashboardService) GetStats(ctx context.Context, from, to time.Time) (*DashboardStats, error) {
	stats := &DashboardStats{
		StatusBreakdown:   make(map[string]int),
		CurrencyBreakdown: make(map[string]int64),
		PSPBreakdown:      make(map[string]int),
		PeriodStart:       from,
		PeriodEnd:         to,
	}

	// Total payments, revenue, and average
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(amount), 0), COALESCE(AVG(amount), 0)
		FROM payments
		WHERE created_at >= $1 AND created_at < $2
	`, from, to).Scan(&stats.TotalPayments, &stats.TotalRevenue, &stats.AvgPaymentAmount)
	if err != nil {
		return nil, fmt.Errorf("querying totals: %w", err)
	}

	// Success and failure counts
	var successCount, failureCount int64
	err = s.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'captured'),
			COUNT(*) FILTER (WHERE status = 'failed')
		FROM payments
		WHERE created_at >= $1 AND created_at < $2
	`, from, to).Scan(&successCount, &failureCount)
	if err != nil {
		return nil, fmt.Errorf("querying success/failure: %w", err)
	}

	if stats.TotalPayments > 0 {
		stats.SuccessRate = float64(successCount) / float64(stats.TotalPayments) * 100
		stats.FailureRate = float64(failureCount) / float64(stats.TotalPayments) * 100
	}

	// Status breakdown
	rows, err := s.db.Query(ctx, `
		SELECT status, COUNT(*)
		FROM payments
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY status
	`, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying status breakdown: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats.StatusBreakdown[status] = count
	}

	// Currency breakdown (total volume per currency)
	rows, err = s.db.Query(ctx, `
		SELECT currency, COALESCE(SUM(amount), 0)
		FROM payments
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY currency
	`, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying currency breakdown: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var currency string
		var total int64
		if err := rows.Scan(&currency, &total); err != nil {
			return nil, err
		}
		stats.CurrencyBreakdown[currency] = total
	}

	// PSP breakdown (count per PSP)
	rows, err = s.db.Query(ctx, `
		SELECT psp, COUNT(*)
		FROM payments
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY psp
	`, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying PSP breakdown: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var psp string
		var count int
		if err := rows.Scan(&psp, &count); err != nil {
			return nil, err
		}
		stats.PSPBreakdown[psp] = count
	}

	// Today's stats
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.UTC)
	todayEnd := todayStart.Add(24 * time.Hour)

	err = s.db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(amount), 0)
		FROM payments
		WHERE created_at >= $1 AND created_at < $2
	`, todayStart, todayEnd).Scan(&stats.TodayPayments, &stats.TodayRevenue)
	if err != nil {
		return nil, fmt.Errorf("querying today stats: %w", err)
	}

	return stats, nil
}

// DailyVolume holds per-day payment volume for charting.
type DailyVolume struct {
	Date    string `json:"date"`
	Count   int    `json:"count"`
	Revenue int64  `json:"revenue"`
}

// GetDailyVolume returns payment counts and revenue per day for charting.
func (s *DashboardService) GetDailyVolume(ctx context.Context, from, to time.Time) ([]DailyVolume, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DATE(created_at) as day, COUNT(*), COALESCE(SUM(amount), 0)
		FROM payments
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY DATE(created_at)
		ORDER BY day ASC
	`, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying daily volume: %w", err)
	}
	defer rows.Close()

	var result []DailyVolume
	for rows.Next() {
		var d DailyVolume
		var day time.Time
		if err := rows.Scan(&day, &d.Count, &d.Revenue); err != nil {
			return nil, err
		}
		d.Date = day.Format("2006-01-02")
		result = append(result, d)
	}

	return result, nil
}
