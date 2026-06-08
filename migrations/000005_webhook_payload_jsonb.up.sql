-- Change raw_payload from TEXT to JSONB for better indexing and query support.
-- This is safe because all stored payloads are JSON.
ALTER TABLE webhook_events ALTER COLUMN raw_payload TYPE JSONB USING raw_payload::jsonb;
