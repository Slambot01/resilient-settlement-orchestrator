CREATE TABLE IF NOT EXISTS reconciliation_records (
    id UUID PRIMARY KEY,
    reconciliation_date DATE NOT NULL,
    psp VARCHAR(50) NOT NULL,

    -- Summary
    total_payments INT DEFAULT 0,
    total_amount BIGINT DEFAULT 0,

    -- Comparison
    internal_records_count INT DEFAULT 0,
    psp_records_count INT DEFAULT 0,
    matched_count INT DEFAULT 0,
    discrepancy_count INT DEFAULT 0,

    status VARCHAR(20) NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed', 'failed')),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_rr_reconciliation_date ON reconciliation_records(reconciliation_date);
CREATE INDEX idx_rr_psp ON reconciliation_records(psp);
CREATE INDEX idx_rr_status ON reconciliation_records(status);

CREATE TABLE IF NOT EXISTS reconciliation_discrepancies (
    id UUID PRIMARY KEY,
    reconciliation_record_id UUID NOT NULL REFERENCES reconciliation_records(id) ON DELETE CASCADE,
    payment_id UUID,

    discrepancy_type VARCHAR(50) NOT NULL CHECK (discrepancy_type IN (
        'missing_in_psp', 'missing_internal', 'amount_mismatch', 'status_mismatch'
    )),

    internal_amount BIGINT,
    psp_amount BIGINT,
    internal_status VARCHAR(50),
    psp_status VARCHAR(50),

    resolution_status VARCHAR(20) DEFAULT 'open' CHECK (resolution_status IN ('open', 'investigating', 'resolved')),
    resolution_notes TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX idx_rd_reconciliation_record_id ON reconciliation_discrepancies(reconciliation_record_id);
CREATE INDEX idx_rd_resolution_status ON reconciliation_discrepancies(resolution_status);
CREATE INDEX idx_rd_payment_id ON reconciliation_discrepancies(payment_id);
