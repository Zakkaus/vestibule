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
