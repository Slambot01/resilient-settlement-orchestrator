CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY,
    merchant_id VARCHAR(255) NOT NULL,
    order_id VARCHAR(255) NOT NULL,

    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,

    status VARCHAR(50) NOT NULL DEFAULT 'created',

    -- PSP details
    psp VARCHAR(50),
    psp_payment_id VARCHAR(255),

    -- Customer
    customer_email VARCHAR(255),
    customer_phone VARCHAR(50),
    customer_name VARCHAR(255),

    -- Payment method
    payment_method_type VARCHAR(50),
    payment_method_details JSONB,

    -- Metadata
    metadata JSONB DEFAULT '{}',

    -- Audit
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255)
);

CREATE INDEX idx_payments_merchant_id ON payments(merchant_id);
CREATE INDEX idx_payments_order_id ON payments(order_id);
CREATE INDEX idx_payments_psp_payment_id ON payments(psp, psp_payment_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_created_at ON payments(created_at);

-- State transition audit trail
CREATE TABLE IF NOT EXISTS payment_state_transitions (
    id UUID PRIMARY KEY,
    payment_id UUID NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    from_status VARCHAR(50),
    to_status VARCHAR(50) NOT NULL,
    reason TEXT,
    triggered_by VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pst_payment_id ON payment_state_transitions(payment_id);
CREATE INDEX idx_pst_created_at ON payment_state_transitions(created_at);
