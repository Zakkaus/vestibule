-- v1 -> v2: Per-chat settings revisions
ALTER TABLE chat ADD COLUMN settings_revision BIGINT NOT NULL DEFAULT 0;
