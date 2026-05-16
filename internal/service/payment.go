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
)

type PaymentService struct {
	db     *pgxpool.Pool
	router *PaymentRouter
	ledger *LedgerService
}

func NewPaymentService(db *pgxpool.Pool, router *PaymentRouter, ledger *LedgerService) *PaymentService {
	return &PaymentService{
		db:     db,
		router: router,
		ledger: ledger,
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

	// 3. Call the PSP Adapter
	pspReq := adapter.PSPPaymentRequest{
		Amount:        req.Amount,
		Currency:      req.Currency,
		Description:   "Order " + req.OrderID,
		OrderID:       req.OrderID,
		CustomerEmail: req.CustomerEmail,
	}

	pspRes, pspErr := pspAdapter.CreatePayment(ctx, pspReq)

	// Record outcome on the circuit breaker
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
		SELECT id, merchant_id, order_id, amount, currency, status, psp, psp_payment_id, customer_email, created_at, updated_at
		FROM payments WHERE id = $1
	`, id).Scan(&p.ID, &p.MerchantID, &p.OrderID, &p.Amount, &p.Currency, &p.Status, &p.PSP, &p.PSPPaymentID, &p.CustomerEmail, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("payment not found")
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

	// Update Payment
	_, err = tx.Exec(ctx, `
		UPDATE payments 
		SET status = $1, psp_payment_id = COALESCE(NULLIF($2, ''), psp_payment_id), updated_at = $3 
		WHERE id = $4 AND status = $5
	`, to, pspPaymentID, now, paymentID, from)
	if err != nil {
		return err
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

	return tx.Commit(ctx)
}
