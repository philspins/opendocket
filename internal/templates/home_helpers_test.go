package templates

import (
	"testing"

	"github.com/philspins/opendocket/internal/opennorth"
)

// ── homeRepresentativeNameRole ────────────────────────────────────────────────

func TestHomeRepresentativeNameRole_UsesRepWhenSet(t *testing.T) {
	rep := opennorth.Representative{Name: "Jane Doe", ElectedOffice: "MP"}
	name, role := homeRepresentativeNameRole(rep, "Fallback Name", "Fallback Role")
	if name != "Jane Doe" {
		t.Errorf("name = %q, want %q", name, "Jane Doe")
	}
	if role != "MP" {
		t.Errorf("role = %q, want %q", role, "MP")
	}
}

func TestHomeRepresentativeNameRole_FallsBackWhenEmpty(t *testing.T) {
	rep := opennorth.Representative{Name: "", ElectedOffice: ""}
	name, role := homeRepresentativeNameRole(rep, "Fallback Name", "Fallback Role")
	if name != "Fallback Name" {
		t.Errorf("name = %q, want %q", name, "Fallback Name")
	}
	if role != "Fallback Role" {
		t.Errorf("role = %q, want %q", role, "Fallback Role")
	}
}

func TestHomeRepresentativeNameRole_TrimsWhitespace(t *testing.T) {
	rep := opennorth.Representative{Name: "  Alice  ", ElectedOffice: "  MLA  "}
	name, role := homeRepresentativeNameRole(rep, "", "")
	if name != "Alice" {
		t.Errorf("name = %q, expected whitespace trimmed", name)
	}
	if role != "MLA" {
		t.Errorf("role = %q, expected whitespace trimmed", role)
	}
}

// ── homeRepresentativeHeading ─────────────────────────────────────────────────

func TestHomeRepresentativeHeading_WithRole(t *testing.T) {
	rep := opennorth.Representative{Name: "Jane Doe", ElectedOffice: "MP"}
	got := homeRepresentativeHeading(rep, "", "")
	want := "Jane Doe (MP)"
	if got != want {
		t.Errorf("homeRepresentativeHeading() = %q, want %q", got, want)
	}
}

func TestHomeRepresentativeHeading_WithoutRole(t *testing.T) {
	rep := opennorth.Representative{Name: "Jane Doe", ElectedOffice: ""}
	got := homeRepresentativeHeading(rep, "", "")
	// When role is empty, just the name with no parens
	if got != "Jane Doe" {
		t.Errorf("homeRepresentativeHeading() = %q, want %q", got, "Jane Doe")
	}
}

func TestHomeRepresentativeHeading_FallbackBothEmpty(t *testing.T) {
	rep := opennorth.Representative{}
	got := homeRepresentativeHeading(rep, "Your Rep", "")
	if got != "Your Rep" {
		t.Errorf("homeRepresentativeHeading() = %q, want %q", got, "Your Rep")
	}
}

// ── statusPhrase ──────────────────────────────────────────────────────────────

func TestStatusPhrase(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"in_session", "in session"},
		{"on_break", "on break"},
		{"prorogued", "status unavailable"},
		{"", "status unavailable"},
	}
	for _, tt := range tests {
		if got := statusPhrase(tt.status); got != tt.want {
			t.Errorf("statusPhrase(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
