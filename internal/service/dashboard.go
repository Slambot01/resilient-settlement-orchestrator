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

// PSPHealth combines circuit breaker state with historical performance metrics.
type PSPHealth struct {
	PSP              string  `json:"psp"`
	CircuitState     string  `json:"circuit_state"`
	TotalRequests    int64   `json:"total_requests"`
	TotalFailures    int64   `json:"total_failures"`
	ConsecutiveFails int     `json:"consecutive_failures"`
	SuccessRate      float64 `json:"success_rate"`
	DBTotalPayments  int     `json:"db_total_payments"`
	DBCapturedCount  int     `json:"db_captured_count"`
	DBFailedCount    int     `json:"db_failed_count"`
	DBSuccessRate    float64 `json:"db_success_rate"`
}

// GetPSPHealth returns circuit breaker state and DB-backed success metrics per PSP.
func (s *DashboardService) GetPSPHealth(ctx context.Context) ([]PSPHealth, error) {
	breakerStats := s.router.GetBreakerStats()

	// Query DB for per-PSP payment outcomes
	rows, err := s.db.Query(ctx, `
		SELECT psp,
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'captured') as captured,
			COUNT(*) FILTER (WHERE status = 'failed') as failed
		FROM payments
		GROUP BY psp
		ORDER BY psp ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying PSP metrics: %w", err)
	}
	defer rows.Close()

	dbMetrics := make(map[string][3]int) // [total, captured, failed]
	for rows.Next() {
		var psp string
		var total, captured, failed int
		if err := rows.Scan(&psp, &total, &captured, &failed); err != nil {
			return nil, err
		}
		dbMetrics[psp] = [3]int{total, captured, failed}
	}

	// Merge circuit breaker stats with DB metrics
	seen := make(map[string]bool)
	var result []PSPHealth

	for psp, bs := range breakerStats {
		h := PSPHealth{
			PSP:              psp,
			CircuitState:     string(bs.State),
			TotalRequests:    bs.TotalRequests,
			TotalFailures:    bs.TotalFailures,
			ConsecutiveFails: bs.ConsecutiveFails,
			SuccessRate:      bs.SuccessRate * 100,
		}

		if m, ok := dbMetrics[psp]; ok {
			h.DBTotalPayments = m[0]
			h.DBCapturedCount = m[1]
			h.DBFailedCount = m[2]
			if m[0] > 0 {
				h.DBSuccessRate = float64(m[1]) / float64(m[0]) * 100
			}
		}

		result = append(result, h)
		seen[psp] = true
	}

	// Include PSPs that have DB records but no circuit breaker (shouldn't happen, but defensive)
	for psp, m := range dbMetrics {
		if seen[psp] {
			continue
		}
		h := PSPHealth{
			PSP:             psp,
			CircuitState:    "unknown",
			DBTotalPayments: m[0],
			DBCapturedCount: m[1],
			DBFailedCount:   m[2],
		}
		if m[0] > 0 {
			h.DBSuccessRate = float64(m[1]) / float64(m[0]) * 100
		}
		result = append(result, h)
	}

	return result, nil
}

// RecentPayment is a compact payment summary for listing.
type RecentPayment struct {
	ID            string    `json:"id"`
	MerchantID    string    `json:"merchant_id"`
	OrderID       string    `json:"order_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	PSP           string    `json:"psp"`
	CustomerEmail *string   `json:"customer_email,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// GetRecentPayments returns paginated payment records ordered by most recent.
func (s *DashboardService) GetRecentPayments(ctx context.Context, offset, limit int) ([]RecentPayment, int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM payments`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting payments: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, merchant_id, order_id, amount, currency, status, psp, customer_email, created_at
		FROM payments
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying recent payments: %w", err)
	}
	defer rows.Close()

	var result []RecentPayment
	for rows.Next() {
		var p RecentPayment
		if err := rows.Scan(&p.ID, &p.MerchantID, &p.OrderID, &p.Amount, &p.Currency, &p.Status, &p.PSP, &p.CustomerEmail, &p.CreatedAt); err != nil {
			return nil, 0, err
		}
		result = append(result, p)
	}

	return result, total, nil
}

// ActivityEvent represents a single event in the activity feed.
type ActivityEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	EventType   string    `json:"event_type"`
	PaymentID   string    `json:"payment_id,omitempty"`
	Description string    `json:"description"`
	Actor       string    `json:"actor"`
}

// GetActivityFeed returns a combined feed of state transitions and webhook events.
func (s *DashboardService) GetActivityFeed(ctx context.Context, limit int) ([]ActivityEvent, error) {
	rows, err := s.db.Query(ctx, `
		(
			SELECT created_at, 'state_transition' as source_type, payment_id,
				CONCAT(from_status, ' → ', to_status, ': ', reason) as description,
				triggered_by as actor
			FROM payment_state_transitions
			ORDER BY created_at DESC
			LIMIT $1
		)
		UNION ALL
		(
			SELECT created_at, 'webhook' as source_type, COALESCE(internal_payment_id, '') as payment_id,
				CONCAT(psp, '/', event_type, ' [', status, ']') as description,
				psp as actor
			FROM webhook_events
			ORDER BY created_at DESC
			LIMIT $1
		)
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying activity feed: %w", err)
	}
	defer rows.Close()

	var result []ActivityEvent
	for rows.Next() {
		var e ActivityEvent
		if err := rows.Scan(&e.Timestamp, &e.EventType, &e.PaymentID, &e.Description, &e.Actor); err != nil {
			return nil, err
		}
		result = append(result, e)
	}

	return result, nil
}
