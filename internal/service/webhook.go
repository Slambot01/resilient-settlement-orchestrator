package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/adapter"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/models"
)

type WebhookService struct {
	db       *pgxpool.Pool
	ledger   *LedgerService
	adapters map[string]adapter.PSPAdapter
}

func NewWebhookService(db *pgxpool.Pool, ledger *LedgerService, adapters map[string]adapter.PSPAdapter) *WebhookService {
	return &WebhookService{
		db:       db,
		ledger:   ledger,
		adapters: adapters,
	}
}

// IngestWebhookRetry re-processes a webhook from the DLQ without signature verification.
// The payload was already verified on first ingestion; re-verification would fail because
// the original signature is not stored in the DLQ entry.
func (s *WebhookService) IngestWebhookRetry(ctx context.Context, psp, eventType string, payload []byte) error {
	_, ok := s.adapters[psp]
	if !ok {
		return fmt.Errorf("unknown psp: %s", psp)
	}
	return s.ingestWebhookInternal(ctx, psp, eventType, payload, "", true)
}

// IngestWebhook validates, deduplicates, and processes an incoming webhook.
// Returns nil if the event was already processed (idempotent).
func (s *WebhookService) IngestWebhook(ctx context.Context, psp, eventType string, payload []byte, signature string) error {
	return s.ingestWebhookInternal(ctx, psp, eventType, payload, signature, false)
}

func (s *WebhookService) ingestWebhookInternal(ctx context.Context, psp, eventType string, payload []byte, signature string, skipSigVerify bool) error {
	// 1. Verify signature (unless this is a DLQ retry)
	pspAdapter, ok := s.adapters[psp]
	if !ok {
		return fmt.Errorf("unknown psp: %s", psp)
	}

	if !skipSigVerify {
		if err := pspAdapter.VerifyWebhookSignature(payload, signature); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}

		// V-013: Reject stale webhooks to prevent replay attacks (5-minute window)
		if err := validateWebhookFreshness(payload); err != nil {
			slog.Warn("webhook rejected: stale timestamp",
				slog.String("psp", psp), slog.Any("error", err))
			return fmt.Errorf("webhook replay rejected: %w", err)
		}
	}

	// 2. Build idempotency key from PSP + raw payload hash
	idempotencyKey := fmt.Sprintf("%s:%s:%s", psp, eventType, uuid.NewSHA1(uuid.NameSpaceDNS, payload).String())

	// 3. Check for duplicate (idempotent insert)
	var existingID string
	err := s.db.QueryRow(ctx,
		`SELECT id FROM webhook_events WHERE idempotency_key = $1`, idempotencyKey,
	).Scan(&existingID)

	if err == nil {
		slog.Info("webhook already processed, skipping", slog.String("idempotency_key", idempotencyKey))
		return nil
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("checking duplicate webhook: %w", err)
	}

	// 4. Extract PSP payment ID from payload
	pspPaymentID := extractPSPPaymentID(psp, payload)

	// 5. Look up internal payment by PSP reference
	var internalPaymentID string
	err = s.db.QueryRow(ctx,
		`SELECT id FROM payments WHERE psp = $1 AND psp_payment_id = $2`, psp, pspPaymentID,
	).Scan(&internalPaymentID)

	if err == pgx.ErrNoRows {
		slog.Warn("no matching internal payment for webhook",
			slog.String("psp", psp),
			slog.String("psp_payment_id", pspPaymentID),
		)
	}

	// 6. Insert webhook record (V-004 fix: don't store raw signature — store verification status only)
	webhookID := uuid.NewString()
	now := time.Now().UTC()
	signatureStatus := "verified" // Signature was validated in step 2; never store the raw value

	_, err = s.db.Exec(ctx, `
		INSERT INTO webhook_events (id, psp, event_type, raw_payload, signature, psp_payment_id, internal_payment_id, status, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, webhookID, psp, eventType, string(payload), signatureStatus, pspPaymentID, nilIfEmpty(internalPaymentID), models.WebhookStatusProcessing, idempotencyKey, now)
	if err != nil {
		return fmt.Errorf("inserting webhook event: %w", err)
	}

	// 7. Process based on event type
	var processErr error
	switch eventType {
	case "payment.captured", "payment_intent.succeeded":
		processErr = s.processCaptureWebhook(ctx, internalPaymentID, pspPaymentID)
	case "payment.failed", "payment_intent.payment_failed":
		processErr = s.processFailureWebhook(ctx, internalPaymentID)
	case "refund.created", "refund.processed":
		processErr = s.processRefundWebhook(ctx, internalPaymentID, payload)
	default:
		slog.Info("unhandled webhook event type", slog.String("type", eventType))
	}

	// 8. Update webhook status
	finalStatus := models.WebhookStatusProcessed
	var errMsg string
	if processErr != nil {
		finalStatus = models.WebhookStatusFailed
		errMsg = processErr.Error()
	}

	_, err = s.db.Exec(ctx, `
		UPDATE webhook_events SET status = $1, error_message = $2, processed_at = $3 WHERE id = $4
	`, finalStatus, nilIfEmpty(errMsg), now, webhookID)
	if err != nil {
		slog.Error("failed to update webhook status", slog.Any("error", err))
	}

	return processErr
}

// processCaptureWebhook handles a successful capture event.
// Everything runs inside a single database transaction:
//   - Update payment status to captured
//   - Record state transition
//   - Post double-entry ledger entries
//   - Update account balances
//
// If any step fails, PostgreSQL rolls back the entire operation.
// The PSP will retry the webhook on a non-200 response.
func (s *WebhookService) processCaptureWebhook(ctx context.Context, paymentID, pspPaymentID string) error {
	if paymentID == "" {
		return fmt.Errorf("cannot process capture: no internal payment ID")
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	// 1. Lock the payment row and verify current status
	var currentStatus models.PaymentStatus
	var amount int64
	err = tx.QueryRow(ctx,
		`SELECT status, amount FROM payments WHERE id = $1 FOR UPDATE`, paymentID,
	).Scan(&currentStatus, &amount)
	if err != nil {
		return fmt.Errorf("locking payment: %w", err)
	}

	// Allow idempotent re-processing: if already captured, succeed silently
	if currentStatus == models.PaymentStatusCaptured {
		return nil
	}

	if err := ValidateTransition(currentStatus, models.PaymentStatusCaptured); err != nil {
		return fmt.Errorf("invalid state for capture: %w", err)
	}

	// 2. Update payment status
	_, err = tx.Exec(ctx,
		`UPDATE payments SET status = $1, updated_at = $2 WHERE id = $3`,
		models.PaymentStatusCaptured, now, paymentID,
	)
	if err != nil {
		return fmt.Errorf("updating payment status: %w", err)
	}

	// 3. Record state transition
	_, err = tx.Exec(ctx, `
		INSERT INTO payment_state_transitions (id, payment_id, from_status, to_status, reason, triggered_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.NewString(), paymentID, currentStatus, models.PaymentStatusCaptured, "Capture confirmed by PSP webhook", "webhook", now)
	if err != nil {
		return fmt.Errorf("recording transition: %w", err)
	}

	// 4. Post ledger entries within the same transaction
	//    Debit: PSP_SETTLEMENT (asset increases — PSP owes us money)
	//    Credit: MERCHANT_PAY (liability increases — we owe the merchant)
	txID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_transactions (id, payment_id, transaction_type, status, created_at, posted_at)
		VALUES ($1, $2, $3, $4, $5, $5)
	`, txID, paymentID, "payment_capture", models.TxStatusPosted, now)
	if err != nil {
		return fmt.Errorf("inserting ledger tx: %w", err)
	}

	// Debit PSP_SETTLEMENT
	err = s.postEntryInTx(ctx, tx, txID, models.AccPSPSettlement, amount, 0, paymentID, "Payment captured", now)
	if err != nil {
		return fmt.Errorf("debit entry: %w", err)
	}

	// Credit MERCHANT_PAY
	err = s.postEntryInTx(ctx, tx, txID, models.AccMerchantPayable, 0, amount, paymentID, "Payment captured", now)
	if err != nil {
		return fmt.Errorf("credit entry: %w", err)
	}

	// 5. Commit — all or nothing
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit capture tx: %w", err)
	}

	slog.Info("capture webhook processed",
		slog.String("payment_id", paymentID),
		slog.Int64("amount", amount),
	)
	return nil
}

func (s *WebhookService) processFailureWebhook(ctx context.Context, paymentID string) error {
	if paymentID == "" {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	var currentStatus models.PaymentStatus
	err = tx.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1 FOR UPDATE`, paymentID).Scan(&currentStatus)
	if err != nil {
		return err
	}

	if currentStatus.IsTerminal() {
		return nil
	}

	if err := ValidateTransition(currentStatus, models.PaymentStatusFailed); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE payments SET status = $1, updated_at = $2 WHERE id = $3`,
		models.PaymentStatusFailed, now, paymentID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO payment_state_transitions (id, payment_id, from_status, to_status, reason, triggered_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.NewString(), paymentID, currentStatus, models.PaymentStatusFailed, "Payment failed per PSP webhook", "webhook", now)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *WebhookService) processRefundWebhook(ctx context.Context, paymentID string, payload []byte) error {
	if paymentID == "" {
		return nil
	}

	// Extract refund amount from payload
	var parsed map[string]interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return fmt.Errorf("parsing refund webhook payload: %w", err)
	}

	refundAmount := extractRefundAmount(parsed)
	if refundAmount <= 0 {
		return fmt.Errorf("invalid refund amount in webhook payload")
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	var currentStatus models.PaymentStatus
	var totalAmount int64
	err = tx.QueryRow(ctx, `SELECT status, amount FROM payments WHERE id = $1 FOR UPDATE`, paymentID).Scan(&currentStatus, &totalAmount)
	if err != nil {
		return err
	}

	newStatus := models.PaymentStatusRefunded
	if refundAmount < totalAmount {
		newStatus = models.PaymentStatusPartiallyRefunded
	}

	if err := ValidateTransition(currentStatus, newStatus); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE payments SET status = $1, updated_at = $2 WHERE id = $3`, newStatus, now, paymentID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO payment_state_transitions (id, payment_id, from_status, to_status, reason, triggered_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.NewString(), paymentID, currentStatus, newStatus, "Refund confirmed by PSP webhook", "webhook", now)
	if err != nil {
		return err
	}

	// Reverse the original capture ledger entries
	txID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_transactions (id, payment_id, transaction_type, status, created_at, posted_at)
		VALUES ($1, $2, $3, $4, $5, $5)
	`, txID, paymentID, "refund", models.TxStatusPosted, now)
	if err != nil {
		return err
	}

	// Debit MERCHANT_PAY (reduce liability — we owe the merchant less)
	err = s.postEntryInTx(ctx, tx, txID, models.AccMerchantPayable, refundAmount, 0, paymentID, "Refund processed", now)
	if err != nil {
		return err
	}

	// Credit REFUND_EXP (expense increases)
	err = s.postEntryInTx(ctx, tx, txID, models.AccRefundExpense, 0, refundAmount, paymentID, "Refund processed", now)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// postEntryInTx inserts a ledger entry and updates the account balance within an existing transaction.
// Uses SELECT FOR UPDATE to prevent balance drift under concurrency.
func (s *WebhookService) postEntryInTx(ctx context.Context, tx pgx.Tx, txID, accountCode string, debit, credit int64, refID, desc string, now time.Time) error {
	var accountID string
	var currentBalance int64
	var accType models.AccountType

	err := tx.QueryRow(ctx,
		`SELECT id, current_balance, account_type FROM ledger_accounts WHERE account_code = $1 FOR UPDATE`,
		accountCode,
	).Scan(&accountID, &currentBalance, &accType)
	if err != nil {
		return fmt.Errorf("locking account %s: %w", accountCode, err)
	}

	newBalance := currentBalance
	switch accType {
	case models.AccountTypeAsset, models.AccountTypeExpense:
		newBalance += debit - credit
	case models.AccountTypeLiability, models.AccountTypeRevenue:
		newBalance += credit - debit
	}

	_, err = tx.Exec(ctx, `UPDATE ledger_accounts SET current_balance = $1 WHERE id = $2`, newBalance, accountID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_entries (id, transaction_id, account_id, debit, credit, running_balance, description, reference_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, uuid.NewString(), txID, accountID, debit, credit, newBalance, desc, refID, now)

	return err
}

// extractPSPPaymentID pulls the payment identifier from a raw webhook payload.
func extractPSPPaymentID(psp string, payload []byte) string {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return ""
	}

	switch psp {
	case "stripe":
		// Stripe: data.object.id
		if obj, ok := data["data"].(map[string]interface{}); ok {
			if inner, ok := obj["object"].(map[string]interface{}); ok {
				if id, ok := inner["id"].(string); ok {
					return id
				}
			}
		}
	case "razorpay":
		// Razorpay: payload.payment.entity.id
		if p, ok := data["payload"].(map[string]interface{}); ok {
			if payment, ok := p["payment"].(map[string]interface{}); ok {
				if entity, ok := payment["entity"].(map[string]interface{}); ok {
					if id, ok := entity["id"].(string); ok {
						return id
					}
				}
			}
		}
	case "mock":
		if id, ok := data["payment_id"].(string); ok {
			return id
		}
	}

	return ""
}

func extractRefundAmount(data map[string]interface{}) int64 {
	// Try Stripe format
	if obj, ok := data["data"].(map[string]interface{}); ok {
		if inner, ok := obj["object"].(map[string]interface{}); ok {
			if amount, ok := inner["amount"].(float64); ok {
				return int64(amount)
			}
		}
	}

	// Try Razorpay format
	if p, ok := data["payload"].(map[string]interface{}); ok {
		if refund, ok := p["refund"].(map[string]interface{}); ok {
			if entity, ok := refund["entity"].(map[string]interface{}); ok {
				if amount, ok := entity["amount"].(float64); ok {
					return int64(amount)
				}
			}
		}
	}

	// Try flat format (mock)
	if amount, ok := data["amount"].(float64); ok {
		return int64(amount)
	}

	return 0
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// webhookMaxAge is the maximum age of a webhook before it's rejected as stale.
const webhookMaxAge = 5 * time.Minute

// validateWebhookFreshness extracts the timestamp from a webhook payload and rejects
// events older than webhookMaxAge. Supports multiple PSP timestamp formats:
//   - Stripe: "created" (unix epoch seconds)
//   - Razorpay: "created_at" (unix epoch seconds)
//   - Mock/generic: "timestamp" (RFC3339 string)
//
// If no timestamp field is found, the webhook is accepted (best-effort).
func validateWebhookFreshness(payload []byte) error {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil // can't parse — let signature verification handle it
	}

	var eventTime time.Time

	// Stripe format: "created" is a unix epoch
	if created, ok := data["created"].(float64); ok && created > 0 {
		eventTime = time.Unix(int64(created), 0)
	}

	// Razorpay format: "created_at" is a unix epoch
	if eventTime.IsZero() {
		if createdAt, ok := data["created_at"].(float64); ok && createdAt > 0 {
			eventTime = time.Unix(int64(createdAt), 0)
		}
	}

	// Mock/generic format: "timestamp" is an RFC3339 string
	if eventTime.IsZero() {
		if ts, ok := data["timestamp"].(string); ok && ts != "" {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				eventTime = t
			}
		}
	}

	// If no timestamp found, accept (best-effort — not all webhook types include timestamps)
	if eventTime.IsZero() {
		return nil
	}

	age := time.Since(eventTime)
	if age > webhookMaxAge {
		return fmt.Errorf("webhook timestamp is %v old (max allowed: %v)", age.Round(time.Second), webhookMaxAge)
	}
	// Also reject future-dated webhooks (clock skew > 1 minute)
	if age < -1*time.Minute {
		return fmt.Errorf("webhook timestamp is %v in the future", (-age).Round(time.Second))
	}

	return nil
}
