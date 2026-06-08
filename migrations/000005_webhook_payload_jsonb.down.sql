-- Revert raw_payload back to TEXT.
ALTER TABLE webhook_events ALTER COLUMN raw_payload TYPE TEXT USING raw_payload::text;
