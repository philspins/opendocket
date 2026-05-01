// Package db provides SQLite connection setup and schema migration for Open Docket.
// All upsert helpers and write-side record types live in internal/store.
package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// DefaultPath is the SQLite database file used when no path is provided.
const DefaultPath = "opendocket.db"

// Open returns an initialised *sql.DB with WAL mode and FK enforcement.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		path = DefaultPath
	}
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open db %q: %w", path, err)
	}
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Migrate creates all tables and indices if they do not already exist.
func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS members (
			id               TEXT PRIMARY KEY,
			name             TEXT NOT NULL,
			party            TEXT,
			riding           TEXT,
			province         TEXT,
			role             TEXT,
			photo_url        TEXT,
			email            TEXT,
			website          TEXT,
			chamber          TEXT DEFAULT 'commons',
			active           INTEGER DEFAULT 1,
			last_scraped     TEXT,
			government_level TEXT DEFAULT 'federal'
		)`,
		`CREATE TABLE IF NOT EXISTS bills (
			id                 TEXT PRIMARY KEY,
			parliament         INTEGER,
			session            INTEGER,
			number             TEXT,
			title              TEXT,
			short_title        TEXT,
			bill_type          TEXT,
			chamber            TEXT,
			sponsor_id         TEXT REFERENCES members(id),
			current_stage      TEXT,
			current_status     TEXT,
			category           TEXT,
			summary_ai         TEXT,
			summary_lop        TEXT,
			full_text_url      TEXT,
			legisinfo_url      TEXT,
			introduced_date    TEXT,
			last_activity_date TEXT,
			last_scraped       TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS divisions (
			id           TEXT PRIMARY KEY,
			parliament   INTEGER,
			session      INTEGER,
			number       INTEGER,
			date         TEXT,
			bill_id      TEXT REFERENCES bills(id),
			description  TEXT,
			yeas         INTEGER,
			nays         INTEGER,
			paired       INTEGER DEFAULT 0,
			result       TEXT,
			chamber      TEXT DEFAULT 'commons',
			sitting_url  TEXT,
			last_scraped TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS member_votes (
			division_id TEXT REFERENCES divisions(id),
			member_id   TEXT REFERENCES members(id),
			vote        TEXT,
			PRIMARY KEY (division_id, member_id)
		)`,
		`CREATE TABLE IF NOT EXISTS bill_stages (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			bill_id TEXT REFERENCES bills(id),
			stage   TEXT,
			chamber TEXT,
			date    TEXT,
			notes   TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS sitting_calendar (
			parliament INTEGER,
			session    INTEGER,
			date       TEXT,
			PRIMARY KEY (parliament, session, date)
		)`,
		`CREATE TABLE IF NOT EXISTS legislature_calendar_dates (
			jurisdiction TEXT,
			date         TEXT,
			last_scraped TEXT,
			PRIMARY KEY (jurisdiction, date)
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id                   TEXT PRIMARY KEY,
			email                TEXT UNIQUE,
			email_verified       INTEGER DEFAULT 0,
			address              TEXT,
			postal_code          TEXT,
			federal_riding_id    TEXT,
			provincial_riding_id TEXT,
			created_at           TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			email_digest         TEXT DEFAULT 'weekly'
		)`,
		`CREATE TABLE IF NOT EXISTS user_follows (
			user_id    TEXT REFERENCES users(id) ON DELETE CASCADE,
			member_id  TEXT REFERENCES members(id) ON DELETE CASCADE,
			created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			PRIMARY KEY (user_id, member_id)
		)`,
		`CREATE TABLE IF NOT EXISTS bill_reactions (
			user_id    TEXT REFERENCES users(id) ON DELETE CASCADE,
			bill_id    TEXT REFERENCES bills(id) ON DELETE CASCADE,
			reaction   TEXT CHECK (reaction IN ('support', 'oppose', 'neutral')),
			note       TEXT,
			created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			PRIMARY KEY (user_id, bill_id)
		)`,
		`CREATE TABLE IF NOT EXISTS policy_submissions (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id      TEXT REFERENCES users(id),
			member_id    TEXT REFERENCES members(id),
			subject      TEXT,
			body         TEXT,
			category     TEXT,
			submitted_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		)`,
		`CREATE TABLE IF NOT EXISTS bill_reaction_counts (
			bill_id          TEXT PRIMARY KEY REFERENCES bills(id),
			support_count    INTEGER DEFAULT 0,
			oppose_count     INTEGER DEFAULT 0,
			neutral_count    INTEGER DEFAULT 0,
			total_reactions  INTEGER DEFAULT 0,
			refreshed_at     TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS email_verification_tokens (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    TEXT REFERENCES users(id) ON DELETE CASCADE,
			email      TEXT,
			token      TEXT UNIQUE,
			code       TEXT,
			expires_at TEXT,
			used_at    TEXT,
			created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_identities (
			provider         TEXT,
			provider_user_id TEXT,
			user_id          TEXT REFERENCES users(id) ON DELETE CASCADE,
			email            TEXT,
			created_at       TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			PRIMARY KEY (provider, provider_user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS user_sessions (
			id         TEXT PRIMARY KEY,
			user_id    TEXT REFERENCES users(id) ON DELETE CASCADE,
			expires_at TEXT,
			created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		)`,
		`CREATE TABLE IF NOT EXISTS user_category_preferences (
			user_id  TEXT REFERENCES users(id) ON DELETE CASCADE,
			category TEXT NOT NULL,
			PRIMARY KEY (user_id, category)
		)`,
		`CREATE TABLE IF NOT EXISTS user_bill_subscriptions (
			user_id    TEXT REFERENCES users(id) ON DELETE CASCADE,
			bill_id    TEXT REFERENCES bills(id) ON DELETE CASCADE,
			created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			PRIMARY KEY (user_id, bill_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_divisions_bill      ON divisions(bill_id)`,
		`CREATE INDEX IF NOT EXISTS idx_member_votes_member ON member_votes(member_id)`,
		`CREATE INDEX IF NOT EXISTS idx_bills_stage         ON bills(current_stage)`,
		`CREATE INDEX IF NOT EXISTS idx_bills_category      ON bills(category)`,
		`CREATE INDEX IF NOT EXISTS idx_bill_stages_bill    ON bill_stages(bill_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_follows_member ON user_follows(member_id)`,
		`CREATE INDEX IF NOT EXISTS idx_bill_reactions_bill ON bill_reactions(bill_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_tokens_user   ON email_verification_tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user        ON user_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_leg_calendar_juris_date ON legislature_calendar_dates(jurisdiction, date)`,
		`CREATE INDEX IF NOT EXISTS idx_user_cat_prefs_user  ON user_category_preferences(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_bill_subs_user  ON user_bill_subscriptions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_bill_subs_bill  ON user_bill_subscriptions(bill_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	// Forward-compatible migrations for older DBs.
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN email_verified INTEGER DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN address TEXT`)
	var addressesToClear int
	if err := db.QueryRow(`SELECT COUNT(1) FROM users WHERE COALESCE(TRIM(address), '') <> ''`).Scan(&addressesToClear); err != nil {
		return fmt.Errorf("migrate: count user addresses: %w", err)
	}
	if addressesToClear > 0 {
		if _, err := db.Exec(`UPDATE users SET address = '' WHERE COALESCE(TRIM(address), '') <> ''`); err != nil {
			return fmt.Errorf("migrate: clear user addresses: %w", err)
		}
	}
	_, _ = db.Exec(`ALTER TABLE members ADD COLUMN government_level TEXT DEFAULT 'federal'`)

	return nil
}
