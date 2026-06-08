package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/models"
)

type LedgerService struct {
	db *pgxpool.Pool
}

func NewLedgerService(db *pgxpool.Pool) *LedgerService {
	return &LedgerService{db: db}
}

// PostLedgerTransaction records a double-entry ledger transaction.
// It uses row-level locking (SELECT FOR UPDATE) to ensure concurrent balance updates are atomic.
func (s *LedgerService) PostLedgerTransaction(ctx context.Context, req models.PostTransactionRequest) error {
	if len(req.Entries) < 2 {
		return fmt.Errorf("ledger transaction requires at least 2 entries for double-entry bookkeeping")
	}

	// 1. Validate double entry principle
	var totalDebit, totalCredit int64
	for _, entry := range req.Entries {
		if entry.Debit < 0 || entry.Credit < 0 {
			return fmt.Errorf("negative entry amounts are not allowed")
		}
		if entry.Debit > 0 && entry.Credit > 0 {
			return fmt.Errorf("an entry cannot have both debit and credit")
		}
		totalDebit += entry.Debit
		totalCredit += entry.Credit
	}

	if totalDebit != totalCredit {
		return fmt.Errorf("double-entry validation failed: debits (%d) do not equal credits (%d)", totalDebit, totalCredit)
	}

	// 2. Execute within a single database transaction
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("beginning db transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	txID := uuid.NewString()
	var paymentIDPtr *string
	if req.PaymentID != "" {
		paymentIDPtr = &req.PaymentID
	}

	// 3. Insert transaction header
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_transactions (id, payment_id, transaction_type, status, created_at, posted_at)
		VALUES ($1, $2, $3, $4, $5, $5)
	`, txID, paymentIDPtr, req.TransactionType, models.TxStatusPosted, now)
	if err != nil {
		return fmt.Errorf("inserting ledger transaction: %w", err)
	}

	// 4. Process entries and update account balances
	for _, entry := range req.Entries {
		// Acquire exclusive row lock on the account. This prevents the concurrency drift issue.
		// Concurrent transactions hitting the same account will block here until this tx commits/rolls back.
		var accountID string
		var currentBalance int64
		var accType models.AccountType
		err = tx.QueryRow(ctx, `
			SELECT id, current_balance, account_type
			FROM ledger_accounts 
			WHERE account_code = $1 
			FOR UPDATE
		`, entry.AccountCode).Scan(&accountID, &currentBalance, &accType)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("%w: %s", ErrAccountNotFound, entry.AccountCode)
			}
			return fmt.Errorf("locking account %s: %w", entry.AccountCode, err)
		}

		// Calculate new running balance based on account type.
		// Assets/Expenses have a normal debit balance, Liabilities/Revenue have a normal credit balance.
		newBalance := currentBalance
		switch accType {
		case models.AccountTypeAsset, models.AccountTypeExpense:
			// Normal balance is debit
			newBalance += entry.Debit - entry.Credit
		case models.AccountTypeLiability, models.AccountTypeRevenue:
			// Normal balance is credit
			newBalance += entry.Credit - entry.Debit
		}

		// Update the account balance
		_, err = tx.Exec(ctx, `
			UPDATE ledger_accounts 
			SET current_balance = $1 
			WHERE id = $2
		`, newBalance, accountID)
		if err != nil {
			return fmt.Errorf("updating account balance %s: %w", entry.AccountCode, err)
		}

		// Insert the entry
		entryID := uuid.NewString()
		_, err = tx.Exec(ctx, `
			INSERT INTO ledger_entries (id, transaction_id, account_id, debit, credit, running_balance, description, reference_id, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, entryID, txID, accountID, entry.Debit, entry.Credit, newBalance, entry.Description, entry.ReferenceID, now)
		if err != nil {
			return fmt.Errorf("inserting ledger entry for %s: %w", entry.AccountCode, err)
		}
	}

	// 5. Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing ledger transaction: %w", err)
	}

	return nil
}

// GetAccountBalance retrieves the current balance for an account without locking.
func (s *LedgerService) GetAccountBalance(ctx context.Context, accountCode string) (*models.BalanceResponse, error) {
	var resp models.BalanceResponse
	err := s.db.QueryRow(ctx, `
		SELECT account_code, account_name, account_type, current_balance, currency
		FROM ledger_accounts
		WHERE account_code = $1
	`, accountCode).Scan(&resp.AccountCode, &resp.AccountName, &resp.AccountType, &resp.CurrentBalance, &resp.Currency)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrAccountNotFound, accountCode)
		}
		return nil, fmt.Errorf("querying account balance: %w", err)
	}

	return &resp, nil
}

// LedgerEntryRow is a flat row for displaying ledger entries in the dashboard.
type LedgerEntryRow struct {
	ID             string    `json:"id"`
	TransactionID  string    `json:"transaction_id"`
	AccountCode    string    `json:"account_code"`
	AccountName    string    `json:"account_name"`
	Debit          int64     `json:"debit"`
	Credit         int64     `json:"credit"`
	RunningBalance int64     `json:"running_balance"`
	Description    *string   `json:"description,omitempty"`
	TxType         string    `json:"transaction_type"`
	CreatedAt      time.Time `json:"created_at"`
}

// GetRecentEntries returns the most recent ledger entries with account info.
func (s *LedgerService) GetRecentEntries(ctx context.Context, limit int) ([]LedgerEntryRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id, e.transaction_id, a.account_code, a.account_name,
			e.debit, e.credit, e.running_balance, e.description,
			t.transaction_type, e.created_at
		FROM ledger_entries e
		JOIN ledger_accounts a ON a.id = e.account_id
		JOIN ledger_transactions t ON t.id = e.transaction_id
		ORDER BY e.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying ledger entries: %w", err)
	}
	defer rows.Close()

	var result []LedgerEntryRow
	for rows.Next() {
		var r LedgerEntryRow
		if err := rows.Scan(&r.ID, &r.TransactionID, &r.AccountCode, &r.AccountName,
			&r.Debit, &r.Credit, &r.RunningBalance, &r.Description,
			&r.TxType, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}

	return result, nil
}
