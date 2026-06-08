package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/adapter"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/models"
)

type ReconciliationService struct {
	db       *pgxpool.Pool
	adapters map[string]adapter.PSPAdapter
}

func NewReconciliationService(db *pgxpool.Pool, adapters map[string]adapter.PSPAdapter) *ReconciliationService {
	return &ReconciliationService{
		db:       db,
		adapters: adapters,
	}
}

// RunBatchReconciliation compares all captured payments for a given PSP and date
// against the PSP's reported status, flagging any mismatches.
func (s *ReconciliationService) RunBatchReconciliation(ctx context.Context, psp string, date time.Time) (*models.ReconciliationRecord, error) {
	pspAdapter, ok := s.adapters[psp]
	if !ok {
		return nil, fmt.Errorf("unknown psp: %s", psp)
	}

	recordID := uuid.NewString()
	now := time.Now().UTC()
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	// Insert reconciliation record in progress
	_, err := s.db.Exec(ctx, `
		INSERT INTO reconciliation_records (id, reconciliation_date, psp, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, recordID, startOfDay, psp, models.ReconStatusInProgress, now)
	if err != nil {
		return nil, fmt.Errorf("inserting recon record: %w", err)
	}

	// Fetch all internal payments for this PSP and date range
	rows, err := s.db.Query(ctx, `
		SELECT id, psp_payment_id, amount, status
		FROM payments
		WHERE psp = $1 AND created_at >= $2 AND created_at < $3 AND psp_payment_id IS NOT NULL
		ORDER BY created_at ASC
	`, psp, startOfDay, endOfDay)
	if err != nil {
		s.markFailed(ctx, recordID, err.Error())
		return nil, err
	}
	defer rows.Close()

	type internalRecord struct {
		ID           string
		PSPPaymentID string
		Amount       int64
		Status       models.PaymentStatus
	}

	var records []internalRecord
	for rows.Next() {
		var r internalRecord
		if err := rows.Scan(&r.ID, &r.PSPPaymentID, &r.Amount, &r.Status); err != nil {
			s.markFailed(ctx, recordID, err.Error())
			return nil, err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		s.markFailed(ctx, recordID, err.Error())
		return nil, fmt.Errorf("iterating payment records: %w", err)
	}

	var totalAmount int64
	var matched, discrepancies int

	for _, rec := range records {
		totalAmount += rec.Amount

		// Query PSP for current status.
		// NOTE: This is an N+1 pattern — each payment requires a separate PSP API call.
		// This is acceptable because: (1) reconciliation runs as a batch job, not in a request path,
		// (2) most PSPs don't offer bulk status endpoints.
		// TODO: If PSP supports batch status API, use it to reduce round-trips.
		pspStatus, err := pspAdapter.GetPaymentStatus(ctx, rec.PSPPaymentID)
		if err != nil {
			// PSP record not found — flag discrepancy
			s.insertDiscrepancy(ctx, recordID, rec.ID, models.DiscrepancyMissingInPSP, &rec.Amount, nil, string(rec.Status), "")
			discrepancies++
			continue
		}

		// Compare amounts
		if pspStatus.Amount > 0 && pspStatus.Amount != rec.Amount {
			s.insertDiscrepancy(ctx, recordID, rec.ID, models.DiscrepancyAmountMismatch, &rec.Amount, &pspStatus.Amount, string(rec.Status), pspStatus.Status)
			discrepancies++
			continue
		}

		// Compare status (normalize PSP status to our internal mapping)
		normalizedStatus := normalizePSPStatus(psp, pspStatus.Status)
		if normalizedStatus != string(rec.Status) {
			s.insertDiscrepancy(ctx, recordID, rec.ID, models.DiscrepancyStatusMismatch, &rec.Amount, &pspStatus.Amount, string(rec.Status), pspStatus.Status)
			discrepancies++
			continue
		}

		matched++
	}

	// Finalize the reconciliation record
	_, err = s.db.Exec(ctx, `
		UPDATE reconciliation_records
		SET total_payments = $1, total_amount = $2, internal_records_count = $3,
			matched_count = $4, discrepancy_count = $5, status = $6, completed_at = $7
		WHERE id = $8
	`, len(records), totalAmount, len(records), matched, discrepancies, models.ReconStatusCompleted, now, recordID)
	if err != nil {
		return nil, fmt.Errorf("finalizing recon record: %w", err)
	}

	slog.Info("batch reconciliation completed",
		slog.String("psp", psp),
		slog.Int("total", len(records)),
		slog.Int("matched", matched),
		slog.Int("discrepancies", discrepancies),
	)

	return s.GetReconciliationRecord(ctx, recordID)
}

// ReconcileSinglePayment performs real-time reconciliation on a single payment.
// Typically called after processing a webhook.
func (s *ReconciliationService) ReconcileSinglePayment(ctx context.Context, paymentID string) error {
	var psp, pspPaymentID string
	var amount int64
	var status models.PaymentStatus

	err := s.db.QueryRow(ctx,
		`SELECT psp, psp_payment_id, amount, status FROM payments WHERE id = $1`, paymentID,
	).Scan(&psp, &pspPaymentID, &amount, &status)
	if err != nil {
		return fmt.Errorf("fetching payment: %w", err)
	}

	if pspPaymentID == "" {
		return nil // nothing to reconcile yet
	}

	pspAdapter, ok := s.adapters[psp]
	if !ok {
		return fmt.Errorf("unknown psp: %s", psp)
	}

	pspStatus, err := pspAdapter.GetPaymentStatus(ctx, pspPaymentID)
	if err != nil {
		slog.Warn("realtime recon: could not fetch PSP status",
			slog.String("payment_id", paymentID),
			slog.Any("error", err),
		)
		return nil
	}

	normalizedStatus := normalizePSPStatus(psp, pspStatus.Status)
	if normalizedStatus != string(status) {
		slog.Warn("realtime recon: status mismatch detected",
			slog.String("payment_id", paymentID),
			slog.String("internal", string(status)),
			slog.String("psp", pspStatus.Status),
		)
	}

	return nil
}

func (s *ReconciliationService) GetReconciliationRecord(ctx context.Context, id string) (*models.ReconciliationRecord, error) {
	var r models.ReconciliationRecord
	err := s.db.QueryRow(ctx, `
		SELECT id, reconciliation_date, psp, total_payments, total_amount,
			internal_records_count, matched_count, discrepancy_count, status, created_at, completed_at
		FROM reconciliation_records WHERE id = $1
	`, id).Scan(&r.ID, &r.ReconciliationDate, &r.PSP, &r.TotalPayments, &r.TotalAmount,
		&r.InternalRecordCount, &r.MatchedCount, &r.DiscrepancyCount, &r.Status, &r.CreatedAt, &r.CompletedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("reconciliation record not found")
		}
		return nil, err
	}
	return &r, nil
}

func (s *ReconciliationService) GetDiscrepancies(ctx context.Context, reconID string) ([]models.Discrepancy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, reconciliation_record_id, payment_id, discrepancy_type,
			internal_amount, psp_amount, internal_status, psp_status,
			resolution_status, resolution_notes, created_at, resolved_at
		FROM reconciliation_discrepancies
		WHERE reconciliation_record_id = $1
		ORDER BY created_at ASC
	`, reconID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Discrepancy
	for rows.Next() {
		var d models.Discrepancy
		err := rows.Scan(&d.ID, &d.ReconciliationRecordID, &d.PaymentID, &d.DiscrepancyType,
			&d.InternalAmount, &d.PSPAmount, &d.InternalStatus, &d.PSPStatus,
			&d.ResolutionStatus, &d.ResolutionNotes, &d.CreatedAt, &d.ResolvedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating discrepancies: %w", err)
	}
	return result, nil
}

func (s *ReconciliationService) insertDiscrepancy(ctx context.Context, reconID, paymentID string, dType models.DiscrepancyType, internalAmt, pspAmt *int64, internalStatus, pspStatus string) {
	_, err := s.db.Exec(ctx, `
		INSERT INTO reconciliation_discrepancies
			(id, reconciliation_record_id, payment_id, discrepancy_type, internal_amount, psp_amount, internal_status, psp_status, resolution_status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, uuid.NewString(), reconID, nilIfEmpty(paymentID), dType, internalAmt, pspAmt, nilIfEmpty(internalStatus), nilIfEmpty(pspStatus), models.ResolutionOpen, time.Now().UTC())

	if err != nil {
		slog.Error("failed to insert discrepancy",
			slog.String("recon_id", reconID),
			slog.Any("error", err),
		)
	}
}

func (s *ReconciliationService) markFailed(ctx context.Context, recordID, errMsg string) {
	_, err := s.db.Exec(ctx, `
		UPDATE reconciliation_records SET status = $1, completed_at = $2 WHERE id = $3
	`, models.ReconStatusFailed, time.Now().UTC(), recordID)
	if err != nil {
		slog.Error("failed to mark reconciliation as failed",
			slog.String("record_id", recordID),
			slog.Any("error", err),
		)
	}
}

// normalizePSPStatus maps raw PSP status strings to our internal status.
func normalizePSPStatus(psp, rawStatus string) string {
	switch psp {
	case "stripe":
		switch rawStatus {
		case "requires_capture":
			return string(models.PaymentStatusAuthorized)
		case "succeeded":
			return string(models.PaymentStatusCaptured)
		case "canceled":
			return string(models.PaymentStatusCancelled)
		}
	case "razorpay":
		switch rawStatus {
		case "authorized":
			return string(models.PaymentStatusAuthorized)
		case "captured":
			return string(models.PaymentStatusCaptured)
		case "refunded":
			return string(models.PaymentStatusRefunded)
		case "failed":
			return string(models.PaymentStatusFailed)
		}
	case "mock":
		return rawStatus
	}
	return rawStatus
}
