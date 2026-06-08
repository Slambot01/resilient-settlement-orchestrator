-- Add total_refunded column to track cumulative refunds and prevent over-refund attacks.
-- This is enforced at the application level with SELECT FOR UPDATE + validation,
-- and as a safety net via a CHECK constraint at the database level.
ALTER TABLE payments ADD COLUMN total_refunded BIGINT NOT NULL DEFAULT 0;

-- Database-level safety net: total_refunded can never exceed original amount
ALTER TABLE payments ADD CONSTRAINT chk_refund_cap CHECK (total_refunded >= 0 AND total_refunded <= amount);
