// Package testutil provides shared test helpers.
package testutil

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/philspins/opendocket/internal/db"
)

// OpenDB opens an in-memory SQLite database scoped to the calling test.
// The database is fully migrated and automatically closed via t.Cleanup.
// Each call with a distinct t.Name() gets its own isolated database.
func OpenDB(t *testing.T) *sql.DB {
	t.Helper()
	// Sanitize the test name so it is a valid SQLite URI filename component.
	name := strings.NewReplacer("/", "_", " ", "_", "=", "_", ":", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_foreign_keys=on", name)
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("testutil.OpenDB sql.Open: %v", err)
	}
	if err := db.Migrate(conn); err != nil {
		conn.Close()
		t.Fatalf("testutil.OpenDB db.Migrate: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
