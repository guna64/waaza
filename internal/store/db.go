package store

import (
	"database/sql"
	"fmt"
)

func OpenPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS waaza_users (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			token TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS waaza_instances (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			token TEXT NOT NULL,
			webhook TEXT NOT NULL DEFAULT '',
			events JSONB NOT NULL DEFAULT '[]'::jsonb,
			history INTEGER NOT NULL DEFAULT 0,
			hmac_key TEXT NOT NULL DEFAULT '',
			proxy JSONB NOT NULL DEFAULT '{}'::jsonb,
			s3 JSONB NOT NULL DEFAULT '{}'::jsonb,
			connected BOOLEAN NOT NULL DEFAULT false,
			logged_in BOOLEAN NOT NULL DEFAULT false,
			jid TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_waaza_instances_token ON waaza_instances(token)`,
		`CREATE TABLE IF NOT EXISTS waaza_outbox (
			id TEXT PRIMARY KEY,
			instance_id TEXT NOT NULL,
			payload JSONB NOT NULL,
			status TEXT NOT NULL,
			attempt INTEGER NOT NULL DEFAULT 0,
			max_attempt INTEGER NOT NULL DEFAULT 5,
			next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_error TEXT NOT NULL DEFAULT '',
			wa_message_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_waaza_outbox_status_retry ON waaza_outbox(status,next_retry_at,created_at)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
