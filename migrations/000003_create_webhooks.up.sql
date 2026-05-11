CREATE TABLE IF NOT EXISTS webhook_events (
    id UUID PRIMARY KEY,
    psp VARCHAR(50) NOT NULL,
    event_type VARCHAR(100) NOT NULL,

    -- Raw data
    raw_payload TEXT NOT NULL,
    signature VARCHAR(500),

    -- Parsed
    psp_payment_id VARCHAR(255),
    internal_payment_id UUID,

    status VARCHAR(20) NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'processing', 'processed', 'failed')),

    -- Idempotency
    idempotency_key VARCHAR(500) UNIQUE NOT NULL,
    processed_at TIMESTAMPTZ,

    -- Error handling
    retry_count INT DEFAULT 0,
    max_retries INT DEFAULT 5,
    error_message TEXT,
    next_retry_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_we_psp_payment_id ON webhook_events(psp, psp_payment_id);
CREATE INDEX idx_we_internal_payment_id ON webhook_events(internal_payment_id);
CREATE INDEX idx_we_status ON webhook_events(status);
CREATE INDEX idx_we_idempotency_key ON webhook_events(idempotency_key);
CREATE INDEX idx_we_next_retry_at ON webhook_events(next_retry_at) WHERE status = 'failed';
