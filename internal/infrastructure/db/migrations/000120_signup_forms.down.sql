UPDATE public.contacts SET source = 'unknown' WHERE source = 'form';
ALTER TABLE public.contacts DROP CONSTRAINT IF EXISTS contacts_source_check;
ALTER TABLE public.contacts
    ADD CONSTRAINT contacts_source_check
    CHECK (source IN ('unknown', 'manual', 'campaign', 'import', 'sheet_sync', 'api', 'ai_assistant'));
DROP TABLE IF EXISTS signup_form_pending;
DROP TABLE IF EXISTS signup_forms;
