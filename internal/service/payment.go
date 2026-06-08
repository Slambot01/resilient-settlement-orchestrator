package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/adapter"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/models"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/retry"
	appPubSub "github.com/Slambot01/resilient-settlement-orchestrator/internal/pubsub"
)

type PaymentService struct {
	db     *pgxpool.Pool
	router *PaymentRouter
	ledger *LedgerService
	pubsub *appPubSub.Client // nil when Pub/Sub is disabled
}

func NewPaymentService(db *pgxpool.Pool, router *PaymentRouter, ledger *LedgerService, pubsubClient *appPubSub.Client) *PaymentService {
	return &PaymentService{
		db:     db,
		router: router,
		ledger: ledger,
		pubsub: pubsubClient,
	}
}

// CreatePayment handles the initial payment creation flow.
func (s *PaymentService) CreatePayment(ctx context.Context, req models.CreatePaymentRequest) (*models.PaymentResponse, error) {
	paymentID := uuid.NewString()
	now := time.Now().UTC()

	// 1. Route the payment
	decision, pspAdapter, err := s.router.Route(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("routing payment: %w", err)
	}

	// 2. Insert initial payment record (Created) within a tx
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO payments (id, merchant_id, order_id, amount, currency, status, psp, customer_email, customer_name, customer_phone, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12)
	`, paymentID, req.MerchantID, req.OrderID, req.Amount, req.Currency, models.PaymentStatusCreated, decision.SelectedPSP, req.CustomerEmail, req.CustomerName, req.CustomerPhone, req.Metadata, now)
	
	if err != nil {
		tx.Rollback(ctx)
		return nil, fmt.Errorf("inserting payment: %w", err)
	}

	// Insert initial state transition
	transitionID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO payment_state_transitions (id, payment_id, from_status, to_status, reason, triggered_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, transitionID, paymentID, "", models.PaymentStatusCreated, "Payment initialized", "system", now)
	
	if err != nil {
		tx.Rollback(ctx)
		return nil, fmt.Errorf("inserting transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	// 3. Call the PSP Adapter with retry
	pspReq := adapter.PSPPaymentRequest{
		Amount:        req.Amount,
		Currency:      req.Currency,
		Description:   "Order " + req.OrderID,
		OrderID:       req.OrderID,
		CustomerEmail: req.CustomerEmail,
	}

	var pspRes *adapter.PSPPaymentResponse
	var pspErr error

	retryCfg := retry.DefaultConfig()
	pspErr = retry.Do(ctx, retryCfg, "psp_create_payment", func(ctx context.Context) error {
		res, err := pspAdapter.CreatePayment(ctx, pspReq)
		if err != nil {
			return err
		}
		pspRes = res
		return nil
	})

	// Record final outcome on the circuit breaker (after all retries)
	if pspErr != nil {
		s.router.RecordFailure(decision.SelectedPSP)
	} else {
		s.router.RecordSuccess(decision.SelectedPSP)
	}

	// 4. Update status based on PSP response
	finalStatus := models.PaymentStatusAuthorized
	var pspPaymentID string
	var reason string

	if pspErr != nil {
		finalStatus = models.PaymentStatusFailed
		reason = pspErr.Error()
	} else {
		pspPaymentID = pspRes.PSPPaymentID
		reason = "PSP authorization successful"
	}

	err = s.updatePaymentState(ctx, paymentID, models.PaymentStatusCreated, finalStatus, pspPaymentID, reason, "system")
	if err != nil {
		return nil, fmt.Errorf("updating final state: %w", err)
	}

	if pspErr != nil {
		return nil, fmt.Errorf("psp failed: %w", pspErr)
	}

	return s.GetPayment(ctx, paymentID)
}

// GetPayment fetches a payment and its transition history.
func (s *PaymentService) GetPayment(ctx context.Context, id string) (*models.PaymentResponse, error) {
	var p models.Payment
	err := s.db.QueryRow(ctx, `
		SELECT id, merchant_id, order_id, amount, currency, status, psp, psp_payment_id, customer_email, total_refunded, created_at, updated_at
		FROM payments WHERE id = $1
	`, id).Scan(&p.ID, &p.MerchantID, &p.OrderID, &p.Amount, &p.Currency, &p.Status, &p.PSP, &p.PSPPaymentID, &p.CustomerEmail, &p.TotalRefunded, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, payment_id, from_status, to_status, reason, triggered_by, created_at
		FROM payment_state_transitions WHERE payment_id = $1 ORDER BY created_at ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []models.StateTransition
	for rows.Next() {
		var t models.StateTransition
		err := rows.Scan(&t.ID, &t.PaymentID, &t.FromStatus, &t.ToStatus, &t.Reason, &t.TriggeredBy, &t.CreatedAt)
		if err != nil {
			return nil, err
		}
		transitions = append(transitions, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating state transitions: %w", err)
	}

	return &models.PaymentResponse{
		Payment:          p,
		StateTransitions: transitions,
	}, nil
}

// updatePaymentState is an internal helper to update status and record the transition atomically.
func (s *PaymentService) updatePaymentState(ctx context.Context, paymentID string, from, to models.PaymentStatus, pspPaymentID, reason, trigger string) error {
	if err := ValidateTransition(from, to); err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	// Update Payment (optimistic lock: WHERE status = $5)
	tag, err := tx.Exec(ctx, `
		UPDATE payments 
		SET status = $1, psp_payment_id = COALESCE(NULLIF($2, ''), psp_payment_id), updated_at = $3 
		WHERE id = $4 AND status = $5
	`, to, pspPaymentID, now, paymentID, from)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("optimistic lock failed: payment %s is no longer in status %s", paymentID, from)
	}

	// Insert transition
	transitionID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO payment_state_transitions (id, payment_id, from_status, to_status, reason, triggered_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, transitionID, paymentID, from, to, reason, trigger, now)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Publish payment state change event (fire-and-forget, doesn't block payment flow)
	if s.pubsub != nil {
		s.pubsub.PublishPaymentEvent(ctx, paymentID, string(from), string(to), trigger)
	}

	return nil
}

// CapturePayment captures an authorized payment.
// Uses SELECT FOR UPDATE to prevent concurrent capture race conditions (V-019 fix).
func (s *PaymentService) CapturePayment(ctx context.Context, id string) (*models.PaymentResponse, error) {
	// Start transaction — need row lock before PSP call
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the payment row to prevent concurrent captures
	var p models.Payment
	err = tx.QueryRow(ctx, `
		SELECT id, merchant_id, order_id, amount, currency, status, psp, psp_payment_id
		FROM payments WHERE id = $1 FOR UPDATE
	`, id).Scan(&p.ID, &p.MerchantID, &p.OrderID, &p.Amount, &p.Currency, &p.Status, &p.PSP, &p.PSPPaymentID)
	if err != nil {
		return nil, ErrPaymentNotFound
	}

	if p.Status != models.PaymentStatusAuthorized {
		return nil, ErrNotAuthorized
	}

	// Call PSP under row lock — prevents double-capture at PSP level
	adp, err := s.router.GetAdapter(models.PSPName(p.PSP))
	if err != nil {
		return nil, err
	}

	_, err = adp.CapturePayment(ctx, p.PSPPaymentID, p.Amount, p.Currency)
	if err != nil {
		return nil, fmt.Errorf("psp capture failed: %w", err)
	}

	now := time.Now().UTC()

	// Update status atomically
	_, err = tx.Exec(ctx, `
		UPDATE payments SET status = $1, updated_at = $2 WHERE id = $3
	`, models.PaymentStatusCaptured, now, id)
	if err != nil {
		return nil, fmt.Errorf("updating capture state: %w", err)
	}

	// Record state transition
	transitionID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO payment_state_transitions (id, payment_id, from_status, to_status, reason, triggered_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, transitionID, id, models.PaymentStatusAuthorized, models.PaymentStatusCaptured,
		"Payment captured via API", "api", now)
	if err != nil {
		return nil, fmt.Errorf("recording capture transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit capture tx: %w", err)
	}

	// Publish event (fire-and-forget, after commit)
	if s.pubsub != nil {
		s.pubsub.PublishPaymentEvent(ctx, id, string(models.PaymentStatusAuthorized), string(models.PaymentStatusCaptured), "api")
	}

	return s.GetPayment(ctx, id)
}

// RefundPayment refunds a captured payment with over-refund protection.
// Uses SELECT FOR UPDATE to prevent concurrent refund race conditions (V-002 fix).
func (s *PaymentService) RefundPayment(ctx context.Context, id string, amount int64) (*models.PaymentResponse, error) {
	// Start transaction early — we need row-level locking before any PSP call
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the payment row to prevent concurrent refunds
	var p models.Payment
	err = tx.QueryRow(ctx, `
		SELECT id, merchant_id, order_id, amount, currency, status, psp, psp_payment_id, total_refunded
		FROM payments WHERE id = $1 FOR UPDATE
	`, id).Scan(&p.ID, &p.MerchantID, &p.OrderID, &p.Amount, &p.Currency, &p.Status, &p.PSP, &p.PSPPaymentID, &p.TotalRefunded)
	if err != nil {
		return nil, ErrPaymentNotFound
	}

	// Only captured or partially refunded payments can be refunded
	if p.Status != models.PaymentStatusCaptured && p.Status != models.PaymentStatusPartiallyRefunded {
		return nil, fmt.Errorf("payment must be captured or partially refunded to refund, current status: %s", p.Status)
	}

	// Over-refund protection: cumulative refunded + this request must not exceed original amount
	remaining := p.Amount - p.TotalRefunded
	if amount > remaining {
		return nil, fmt.Errorf("refund amount %d exceeds remaining refundable amount %d (original: %d, already refunded: %d)",
			amount, remaining, p.Amount, p.TotalRefunded)
	}

	// Call PSP (still under row lock — prevents concurrent PSP calls for same payment)
	adp, err := s.router.GetAdapter(models.PSPName(p.PSP))
	if err != nil {
		return nil, err
	}

	_, err = adp.RefundPayment(ctx, p.PSPPaymentID, amount)
	if err != nil {
		return nil, fmt.Errorf("psp refund failed: %w", err)
	}

	// Determine new status
	newTotalRefunded := p.TotalRefunded + amount
	newStatus := models.PaymentStatusPartiallyRefunded
	if newTotalRefunded >= p.Amount {
		newStatus = models.PaymentStatusRefunded
	}

	now := time.Now().UTC()

	// Update payment status + total_refunded atomically
	_, err = tx.Exec(ctx, `
		UPDATE payments SET status = $1, total_refunded = $2, updated_at = $3 WHERE id = $4
	`, newStatus, newTotalRefunded, now, id)
	if err != nil {
		return nil, fmt.Errorf("updating refund state: %w", err)
	}

	// Record state transition
	transitionID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO payment_state_transitions (id, payment_id, from_status, to_status, reason, triggered_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, transitionID, id, p.Status, newStatus,
		fmt.Sprintf("Refund of %d processed (total refunded: %d/%d)", amount, newTotalRefunded, p.Amount),
		"api", now)
	if err != nil {
		return nil, fmt.Errorf("recording refund transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit refund tx: %w", err)
	}

	// Publish event (fire-and-forget, after commit)
	if s.pubsub != nil {
		s.pubsub.PublishPaymentEvent(ctx, id, string(p.Status), string(newStatus), "api")
	}

	return s.GetPayment(ctx, id)
}

// CancelPayment cancels an authorized payment.
func (s *PaymentService) CancelPayment(ctx context.Context, id string) (*models.PaymentResponse, error) {
	p, err := s.GetPayment(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.Status != models.PaymentStatusAuthorized {
		return nil, ErrNotAuthorized
	}

	adp, err := s.router.GetAdapter(models.PSPName(p.PSP))
	if err != nil {
		return nil, err
	}

	err = adp.CancelPayment(ctx, p.PSPPaymentID)
	if err != nil {
		return nil, fmt.Errorf("psp cancel failed: %w", err)
	}

	err = s.updatePaymentState(ctx, id, p.Status, models.PaymentStatusCancelled, p.PSPPaymentID, "Payment cancelled via API", "api")
	if err != nil {
		return nil, err
	}

	return s.GetPayment(ctx, id)
}
