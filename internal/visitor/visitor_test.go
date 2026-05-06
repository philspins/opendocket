package visitor_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/visitor"
)

// mockAuth implements visitor.SessionReader.
type mockAuth struct {
	user store.UserRow
	ok   bool
}

func (m *mockAuth) SessionUser(_ *http.Request) (store.UserRow, bool) {
	return m.user, m.ok
}

func TestVisitor_IsAuthenticated(t *testing.T) {
	user := store.UserRow{ID: "u1", Email: "a@b.com"}

	if v := (visitor.Visitor{User: &user}); !v.IsAuthenticated() {
		t.Error("IsAuthenticated() should be true when User is set")
	}
	if v := (visitor.Visitor{}); v.IsAuthenticated() {
		t.Error("IsAuthenticated() should be false when User is nil")
	}
}

func TestVisitor_HasRidingContext(t *testing.T) {
	tests := []struct {
		federal    string
		provincial string
		want       bool
	}{
		{"fed-123", "", true},
		{"", "prov-456", true},
		{"fed-123", "prov-456", true},
		{"", "", false},
		{"  ", "  ", false},
	}
	for _, tt := range tests {
		v := visitor.Visitor{FederalRidingID: tt.federal, ProvincialRidingID: tt.provincial}
		if got := v.HasRidingContext(); got != tt.want {
			t.Errorf("HasRidingContext() = %v, want %v (federal=%q, provincial=%q)",
				got, tt.want, tt.federal, tt.provincial)
		}
	}
}

func TestFromRequest_AuthenticatedUser(t *testing.T) {
	user := store.UserRow{
		ID:                 "u1",
		FederalRidingID:    "  fed-42  ",
		ProvincialRidingID: "prov-7",
	}
	auth := &mockAuth{user: user, ok: true}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	v := visitor.FromRequest(req, auth)

	if !v.IsAuthenticated() {
		t.Fatal("expected authenticated visitor")
	}
	if v.FederalRidingID != "fed-42" {
		t.Errorf("FederalRidingID = %q, want %q (whitespace should be trimmed)", v.FederalRidingID, "fed-42")
	}
	if v.ProvincialRidingID != "prov-7" {
		t.Errorf("ProvincialRidingID = %q, want %q", v.ProvincialRidingID, "prov-7")
	}
}

func TestFromRequest_GuestWithCookies(t *testing.T) {
	auth := &mockAuth{ok: false}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: visitor.FederalRidingCookie, Value: "fed-99"})
	req.AddCookie(&http.Cookie{Name: visitor.ProvincialRidingCookie, Value: "prov-88"})

	v := visitor.FromRequest(req, auth)

	if v.IsAuthenticated() {
		t.Error("expected unauthenticated guest visitor")
	}
	if v.FederalRidingID != "fed-99" {
		t.Errorf("FederalRidingID = %q, want %q", v.FederalRidingID, "fed-99")
	}
	if v.ProvincialRidingID != "prov-88" {
		t.Errorf("ProvincialRidingID = %q, want %q", v.ProvincialRidingID, "prov-88")
	}
}

func TestFromRequest_GuestNoCookies(t *testing.T) {
	auth := &mockAuth{ok: false}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	v := visitor.FromRequest(req, auth)

	if v.IsAuthenticated() {
		t.Error("expected unauthenticated visitor")
	}
	if v.HasRidingContext() {
		t.Error("expected no riding context without cookies")
	}
}

func TestFromRequest_CookieURLDecoding(t *testing.T) {
	auth := &mockAuth{ok: false}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// %22 is a double-quote; some clients wrap values in quotes after URL-encoding.
	req.AddCookie(&http.Cookie{Name: visitor.FederalRidingCookie, Value: "%22fed-42%22"})

	v := visitor.FromRequest(req, auth)

	if v.FederalRidingID != "fed-42" {
		t.Errorf("FederalRidingID = %q, want %q (URL decode + quote strip)", v.FederalRidingID, "fed-42")
	}
}

func TestFromRequest_AuthUserIgnoresCookies(t *testing.T) {
	user := store.UserRow{ID: "u1", FederalRidingID: "from-profile"}
	auth := &mockAuth{user: user, ok: true}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Cookie should be ignored because the session takes priority.
	req.AddCookie(&http.Cookie{Name: visitor.FederalRidingCookie, Value: "from-cookie"})

	v := visitor.FromRequest(req, auth)

	if v.FederalRidingID != "from-profile" {
		t.Errorf("FederalRidingID = %q, want %q (session profile should win over cookie)", v.FederalRidingID, "from-profile")
	}
}

func TestRidingCookieTTL_IsPositive(t *testing.T) {
	if visitor.RidingCookieTTL <= 0 {
		t.Errorf("RidingCookieTTL = %v, must be positive", visitor.RidingCookieTTL)
	}
}
