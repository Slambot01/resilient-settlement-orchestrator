-- Chart of accounts
CREATE TABLE IF NOT EXISTS ledger_accounts (
    id UUID PRIMARY KEY,
    account_code VARCHAR(50) UNIQUE NOT NULL,
    account_name VARCHAR(255) NOT NULL,
    account_type VARCHAR(50) NOT NULL CHECK (account_type IN ('asset', 'liability', 'revenue', 'expense')),
    currency VARCHAR(3) NOT NULL DEFAULT 'INR',
    current_balance BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_la_account_type ON ledger_accounts(account_type);
CREATE INDEX idx_la_account_code ON ledger_accounts(account_code);

-- Ledger transactions (groups of entries)
CREATE TABLE IF NOT EXISTS ledger_transactions (
    id UUID PRIMARY KEY,
    payment_id UUID REFERENCES payments(id),
    transaction_type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'posted', 'reversed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    posted_at TIMESTAMPTZ,
    reversed_at TIMESTAMPTZ,
    reversed_by_transaction_id UUID
);

CREATE INDEX idx_lt_payment_id ON ledger_transactions(payment_id);
CREATE INDEX idx_lt_transaction_type ON ledger_transactions(transaction_type);
CREATE INDEX idx_lt_created_at ON ledger_transactions(created_at);

-- Individual ledger entries (double-entry)
CREATE TABLE IF NOT EXISTS ledger_entries (
    id UUID PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES ledger_transactions(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES ledger_accounts(id),
    debit BIGINT NOT NULL DEFAULT 0,
    credit BIGINT NOT NULL DEFAULT 0,
    running_balance BIGINT NOT NULL,
    description TEXT,
    reference_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Each entry is either a debit OR a credit, never both
    CONSTRAINT chk_debit_credit CHECK (
        (debit > 0 AND credit = 0) OR (credit > 0 AND debit = 0)
    )
);

CREATE INDEX idx_le_transaction_id ON ledger_entries(transaction_id);
CREATE INDEX idx_le_account_id ON ledger_entries(account_id);
CREATE INDEX idx_le_reference_id ON ledger_entries(reference_id);
CREATE INDEX idx_le_created_at ON ledger_entries(created_at);

-- Seed the chart of accounts
INSERT INTO ledger_accounts (id, account_code, account_name, account_type, currency) VALUES
    ('a0000001-0000-0000-0000-000000000001', 'MERCHANT_RECV', 'Merchant Receivable', 'asset', 'INR'),
    ('a0000001-0000-0000-0000-000000000002', 'PSP_SETTLEMENT', 'PSP Settlement Account', 'asset', 'INR'),
    ('a0000001-0000-0000-0000-000000000003', 'CUSTOMER_DEP', 'Customer Deposit', 'liability', 'INR'),
    ('a0000001-0000-0000-0000-000000000004', 'MERCHANT_PAY', 'Merchant Payable', 'liability', 'INR'),
    ('a0000001-0000-0000-0000-000000000005', 'FEE_PAYABLE', 'Fee Payable', 'liability', 'INR'),
    ('a0000001-0000-0000-0000-000000000006', 'PLATFORM_FEE', 'Platform Fee Revenue', 'revenue', 'INR'),
    ('a0000001-0000-0000-0000-000000000007', 'PROCESSING_FEE', 'Payment Processing Fee', 'revenue', 'INR'),
    ('a0000001-0000-0000-0000-000000000008', 'PSP_FEE_EXP', 'PSP Fee Expense', 'expense', 'INR'),
    ('a0000001-0000-0000-0000-000000000009', 'REFUND_EXP', 'Refund Expense', 'expense', 'INR')
ON CONFLICT (account_code) DO NOTHING;
