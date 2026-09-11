-- v2 -> v3 (compatible with v1+): Persist the singleton daily owner-status delivery switch and attempt date
CREATE TABLE daily_status (
    singleton         INTEGER PRIMARY KEY CHECK (singleton = 1),
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    last_attempt_date TEXT    NOT NULL DEFAULT ''
);

INSERT INTO daily_status (singleton) VALUES (1);
