CREATE TABLE users (
	id            TEXT    PRIMARY KEY,
	username      TEXT    NOT NULL UNIQUE,
	password_hash TEXT    NOT NULL,
	last_login_at INTEGER,
	updated_at    INTEGER NOT NULL,
	created_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
	token_hash TEXT    PRIMARY KEY,
	user_id    TEXT    NOT NULL REFERENCES users(id),
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);

CREATE TABLE roles (
	id          INTEGER PRIMARY KEY,
	name        TEXT    NOT NULL UNIQUE,
	permissions INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE users_roles (
	user_id TEXT    NOT NULL REFERENCES users(id),
	role_id INTEGER NOT NULL REFERENCES roles(id),
	PRIMARY KEY (user_id, role_id)
);
