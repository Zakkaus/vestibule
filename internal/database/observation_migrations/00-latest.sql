-- v0 -> v1: Observe-only action journal [incompatible: v0 has no observation schema]
-- This table records only the shape of suppressed writes. Text, callback IDs, answers, and
-- message IDs are intentionally absent. Membership actions relate to chat so group deletion
-- removes their user identifiers; writes without a membership subject retain no identifiers.
CREATE TABLE verification_observation (
    id          TEXT    PRIMARY KEY,
    observed_at BIGINT  NOT NULL,
    operation   TEXT    NOT NULL CHECK (operation IN (
        'send', 'send_html_fallback', 'delete', 'notify', 'alert', 'audit_log', 'fail_alert',
        'approve_join', 'decline_join', 'ban', 'unban', 'mute', 'unmute', 'ack_fast', 'ack_result'
    )),
    chat_id     BIGINT  REFERENCES chat(id) ON DELETE CASCADE,
    user_id     BIGINT,
    seconds     INTEGER NOT NULL DEFAULT 0,
    flag        BOOLEAN NOT NULL DEFAULT FALSE,
    CHECK (
        (operation IN ('approve_join', 'decline_join', 'ban', 'unban', 'mute', 'unmute') AND
            chat_id IS NOT NULL AND user_id IS NOT NULL) OR
        (operation NOT IN ('approve_join', 'decline_join', 'ban', 'unban', 'mute', 'unmute') AND
            chat_id IS NULL AND user_id IS NULL)
    )
);

CREATE INDEX verification_observation_time
    ON verification_observation (observed_at, id);
