DROP INDEX IF EXISTS idx_tasks_provider_message_id;
ALTER TABLE tasks DROP COLUMN IF EXISTS provider_message_id;
