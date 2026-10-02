-- v3 -> v4 (compatible with v1+): Index retained challenge history and action lookups
CREATE INDEX IF NOT EXISTS challenge_history
    ON challenge (chat_id, settled_at, id);
CREATE INDEX IF NOT EXISTS challenge_user_history
    ON challenge (chat_id, user_id, settled_at);
CREATE INDEX IF NOT EXISTS pending_action_history
    ON pending_action (challenge_id, kind);
