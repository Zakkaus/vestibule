-- v3 -> v4 (compatible with v1+): Add challenge-subordinate web proof tokens
CREATE TABLE verify_tokens (
    challenge_id TEXT PRIMARY KEY REFERENCES challenge(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    salt BYTEA NOT NULL,
    pow_bits INTEGER NOT NULL CHECK (pow_bits BETWEEN 12 AND 22),
    issued_at BIGINT NOT NULL
);
CREATE UNIQUE INDEX verify_tokens_hash ON verify_tokens(token_hash);
