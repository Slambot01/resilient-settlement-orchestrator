ALTER TABLE payments DROP CONSTRAINT IF EXISTS chk_refund_cap;
ALTER TABLE payments DROP COLUMN IF EXISTS total_refunded;
