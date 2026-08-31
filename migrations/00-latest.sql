-- v0 -> v1: Latest revision
CREATE TABLE chat (
    id       BIGINT PRIMARY KEY,
    title    TEXT   NOT NULL,
    left_at  BIGINT,
    settings TEXT   NOT NULL DEFAULT '{}'
);

CREATE TABLE challenge (
    id         TEXT    PRIMARY KEY,
    chat_id    BIGINT  NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    user_id    BIGINT  NOT NULL,
    state      TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    payload    TEXT    NOT NULL,
    delivery   TEXT    NOT NULL,
    attempts   INTEGER NOT NULL DEFAULT 0,
    reason     TEXT,
    expires_at BIGINT  NOT NULL,
    settled_at BIGINT,
    settled_by BIGINT,
    epoch      INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX challenge_open
    ON challenge (chat_id, user_id) WHERE state = 'pending';
CREATE INDEX challenge_due
    ON challenge (expires_at) WHERE state = 'pending';

-- Durable outbox for verification side effects. A terminal challenge transition and its first
-- action are inserted in one transaction; workers lease and settle actions independently.
CREATE TABLE pending_action (
    id           TEXT    PRIMARY KEY,
    challenge_id TEXT    NOT NULL REFERENCES challenge(id) ON DELETE CASCADE,
    kind         TEXT    NOT NULL,
    payload      TEXT    NOT NULL,
    state        TEXT    NOT NULL DEFAULT 'pending',
    attempts     INTEGER NOT NULL DEFAULT 0,
    next_try_at  BIGINT  NOT NULL,
    claim_owner  TEXT,
    claim_until  BIGINT,
    done_at      BIGINT,
    failed_at    BIGINT,
    last_error   TEXT
);

CREATE INDEX pending_action_due
    ON pending_action (next_try_at) WHERE state = 'pending';

-- Telegram accepts one update stream per bot token. One fixed row is enough because there is
-- exactly one such resource; a generic lease namespace would create unused policy surface.
CREATE TABLE update_poll_lease (
    singleton  INTEGER PRIMARY KEY CHECK (singleton = 1),
    holder     TEXT    NOT NULL,
    expires_at BIGINT  NOT NULL
);

CREATE TABLE rule (
    id         TEXT    PRIMARY KEY,
    chat_id    BIGINT  NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    collection TEXT    NOT NULL,
    ordinal    INTEGER NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    definition TEXT    NOT NULL
);

-- Snapshot tables carry phase-3A state that has not yet moved to conditional transitions.
CREATE TABLE verification_failure (
    chat_id BIGINT  NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    user_id BIGINT  NOT NULL,
    count   INTEGER NOT NULL,
    last_at BIGINT  NOT NULL,
    PRIMARY KEY (chat_id, user_id)
);

CREATE TABLE agent_tally (
    model TEXT    PRIMARY KEY,
    count INTEGER NOT NULL
);

CREATE TABLE verification_runtime (
    key   TEXT   PRIMARY KEY,
    value BIGINT NOT NULL
);

CREATE TABLE warning_counter (
    chat_id BIGINT  NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    user_id BIGINT  NOT NULL,
    count   INTEGER NOT NULL,
    PRIMARY KEY (chat_id, user_id)
);
