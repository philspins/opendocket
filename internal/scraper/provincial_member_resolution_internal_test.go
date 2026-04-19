package scraper

import (
	"path/filepath"
	"testing"

	"github.com/philspins/open-democracy/internal/db"
)

func TestResolveProvincialMemberID_StripsTitlesAndMatchesInitialPlusSurname(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	_, err = conn.Exec(`INSERT INTO members (id, name, province, chamber, active, government_level) VALUES
		('nb-legislature-wilson-sherry', 'Sherry Wilson', 'New Brunswick', 'new_brunswick', 1, 'provincial'),
		('nb-legislature-wilson-mary', 'Mary Wilson', 'New Brunswick', 'new_brunswick', 1, 'provincial'),
		('nb-legislature-savoie-glen', 'Glen Savoie', 'New Brunswick', 'new_brunswick', 1, 'provincial'),
		('nb-legislature-chiasson-chuck', 'Chuck Chiasson', 'New Brunswick', 'new_brunswick', 1, 'provincial'),
		('manitoba-legislature-dela-cruz-nellie', 'Nellie Kennedy Dela Cruz', 'Manitoba', 'manitoba', 1, 'provincial')`)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}

	tests := []struct {
		province   string
		sourceName string
		wantID     string
	}{
		{"New Brunswick", "Hon. Ms. S. Wilson", "nb-legislature-wilson-sherry"},
		{"New Brunswick", "Hon. Ms. M. Wilson", "nb-legislature-wilson-mary"},
		{"New Brunswick", "Hon. Mr. G. Savoie", "nb-legislature-savoie-glen"},
		{"New Brunswick", "Mr. C. Chiasson", "nb-legislature-chiasson-chuck"},
		{"Manitoba", "DELA CRUZ", "manitoba-legislature-dela-cruz-nellie"},
	}

	for _, tc := range tests {
		got, err := resolveProvincialMemberID(conn, tc.province, tc.sourceName)
		if err != nil {
			t.Fatalf("resolveProvincialMemberID(%q): %v", tc.sourceName, err)
		}
		if got != tc.wantID {
			t.Fatalf("resolveProvincialMemberID(%q)=%q, want %q", tc.sourceName, got, tc.wantID)
		}
	}
}
