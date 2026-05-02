CREATE TABLE job_queue (
	id         TEXT    PRIMARY KEY,
	name       TEXT    NOT NULL,
	payload    TEXT,
	attempts   INTEGER NOT NULL DEFAULT 0,
	status     TEXT    NOT NULL DEFAULT 'pending',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE job_history (
	id           TEXT    PRIMARY KEY,
	name         TEXT    NOT NULL,
	payload      TEXT,
	attempts     INTEGER NOT NULL,
	completed_at INTEGER NOT NULL
);

CREATE TABLE job_graveyard (
	id         TEXT    PRIMARY KEY,
	name       TEXT    NOT NULL,
	payload    TEXT,
	attempts   INTEGER NOT NULL,
	last_error TEXT    NOT NULL,
	buried_at  INTEGER NOT NULL
);
