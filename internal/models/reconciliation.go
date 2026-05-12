package models

import (
	"time"
)

type ReconciliationStatus string

const (
	ReconStatusInProgress ReconciliationStatus = "in_progress"
	ReconStatusCompleted  ReconciliationStatus = "completed"
	ReconStatusFailed     ReconciliationStatus = "failed"
)

type DiscrepancyType string

const (
	DiscrepancyMissingInPSP    DiscrepancyType = "missing_in_psp"
	DiscrepancyMissingInternal DiscrepancyType = "missing_internal"
	DiscrepancyAmountMismatch  DiscrepancyType = "amount_mismatch"
	DiscrepancyStatusMismatch  DiscrepancyType = "status_mismatch"
)

type ResolutionStatus string

const (
	ResolutionOpen          ResolutionStatus = "open"
	ResolutionInvestigating ResolutionStatus = "investigating"
	ResolutionResolved      ResolutionStatus = "resolved"
)

type ReconciliationRecord struct {
	ID                  string               `json:"id"`
	ReconciliationDate  time.Time            `json:"reconciliation_date"`
	PSP                 string               `json:"psp"`
	TotalPayments       int                  `json:"total_payments"`
	TotalAmount         int64                `json:"total_amount"`
	InternalRecordCount int                  `json:"internal_records_count"`
	PSPRecordCount      int                  `json:"psp_records_count"`
	MatchedCount        int                  `json:"matched_count"`
	DiscrepancyCount    int                  `json:"discrepancy_count"`
	Status              ReconciliationStatus `json:"status"`
	CreatedAt           time.Time            `json:"created_at"`
	CompletedAt         *time.Time           `json:"completed_at,omitempty"`
}

type Discrepancy struct {
	ID                     string           `json:"id"`
	ReconciliationRecordID string           `json:"reconciliation_record_id"`
	PaymentID              string           `json:"payment_id,omitempty"`
	DiscrepancyType        DiscrepancyType  `json:"discrepancy_type"`
	InternalAmount         *int64           `json:"internal_amount,omitempty"`
	PSPAmount              *int64           `json:"psp_amount,omitempty"`
	InternalStatus         string           `json:"internal_status,omitempty"`
	PSPStatus              string           `json:"psp_status,omitempty"`
	ResolutionStatus       ResolutionStatus `json:"resolution_status"`
	ResolutionNotes        string           `json:"resolution_notes,omitempty"`
	CreatedAt              time.Time        `json:"created_at"`
	ResolvedAt             *time.Time       `json:"resolved_at,omitempty"`
}
