package models

import (
	"time"
)

type AccountType string

const (
	AccountTypeAsset     AccountType = "asset"
	AccountTypeLiability AccountType = "liability"
	AccountTypeRevenue   AccountType = "revenue"
	AccountTypeExpense   AccountType = "expense"
)

// Well-known account codes seeded during migration.
const (
	AccMerchantReceivable = "MERCHANT_RECV"
	AccPSPSettlement      = "PSP_SETTLEMENT"
	AccCustomerDeposit    = "CUSTOMER_DEP"
	AccMerchantPayable    = "MERCHANT_PAY"
	AccFeePayable         = "FEE_PAYABLE"
	AccPlatformFee        = "PLATFORM_FEE"
	AccProcessingFee      = "PROCESSING_FEE"
	AccPSPFeeExpense      = "PSP_FEE_EXP"
	AccRefundExpense      = "REFUND_EXP"
)

type TransactionStatus string

const (
	TxStatusPending  TransactionStatus = "pending"
	TxStatusPosted   TransactionStatus = "posted"
	TxStatusReversed TransactionStatus = "reversed"
)

type LedgerAccount struct {
	ID             string      `json:"id"`
	AccountCode    string      `json:"account_code"`
	AccountName    string      `json:"account_name"`
	AccountType    AccountType `json:"account_type"`
	Currency       string      `json:"currency"`
	CurrentBalance int64       `json:"current_balance"`
	CreatedAt      time.Time   `json:"created_at"`
}

type LedgerTransaction struct {
	ID                     string            `json:"id"`
	PaymentID              string            `json:"payment_id"`
	TransactionType        string            `json:"transaction_type"`
	Status                 TransactionStatus `json:"status"`
	CreatedAt              time.Time         `json:"created_at"`
	PostedAt               *time.Time        `json:"posted_at,omitempty"`
	ReversedAt             *time.Time        `json:"reversed_at,omitempty"`
	ReversedByTransactionID *string          `json:"reversed_by_transaction_id,omitempty"`
}

type LedgerEntry struct {
	ID             string    `json:"id"`
	TransactionID  string    `json:"transaction_id"`
	AccountID      string    `json:"account_id"`
	Debit          int64     `json:"debit"`
	Credit         int64     `json:"credit"`
	RunningBalance int64     `json:"running_balance"`
	Description    string    `json:"description,omitempty"`
	ReferenceID    string    `json:"reference_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// PostTransactionRequest groups the entries to be atomically recorded.
type PostTransactionRequest struct {
	PaymentID       string
	TransactionType string // "payment_capture", "refund", "fee", "settlement"
	Entries         []EntryLine
}

// EntryLine is a single debit or credit within a ledger transaction.
type EntryLine struct {
	AccountCode string
	Debit       int64
	Credit      int64
	Description string
	ReferenceID string
}

// BalanceResponse is the API output for account balance queries.
type BalanceResponse struct {
	AccountCode    string `json:"account_code"`
	AccountName    string `json:"account_name"`
	AccountType    string `json:"account_type"`
	CurrentBalance int64  `json:"current_balance"`
	Currency       string `json:"currency"`
}
