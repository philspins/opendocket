// Package visitor resolves who is making a request — authenticated user or guest.
// Both cases are first-class: a guest may have riding context via cookies without
// having an account. Callers receive a Visitor and can check IsAuthenticated() and
// HasRidingContext() rather than assembling identity from multiple sources.
package visitor

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/philspins/opendocket/internal/store"
)

// Cookie names for guest riding identity. Exported so the server can write them
// without duplicating the string literals.
const (
	FederalRidingCookie    = "od_guest_federal_riding_id"
	ProvincialRidingCookie = "od_guest_provincial_riding_id"
	RidingCookieTTL        = 24 * time.Hour
)

// SessionReader abstracts auth.Service so this package does not import it.
type SessionReader interface {
	SessionUser(r *http.Request) (store.UserRow, bool)
}

// Visitor is a person making a request. May be authenticated (User is non-nil)
// or a guest (User is nil). Either may have riding context from a saved profile
// or from guest cookies set during a previous riding lookup.
//
// Visitor deliberately does not carry resolved representative objects; callers
// fetch those fresh at render time so the display always reflects the current
// election outcome.
type Visitor struct {
	User               *store.UserRow
	FederalRidingID    string
	ProvincialRidingID string
}

// IsAuthenticated reports whether the visitor has a valid session.
func (v Visitor) IsAuthenticated() bool { return v.User != nil }

// HasRidingContext reports whether the visitor has any riding identity set,
// regardless of authentication state.
func (v Visitor) HasRidingContext() bool {
	return strings.TrimSpace(v.FederalRidingID) != "" ||
		strings.TrimSpace(v.ProvincialRidingID) != ""
}

// FromRequest resolves a Visitor from the incoming request. It first checks for
// an authenticated session (via auth), then falls back to guest riding cookies.
func FromRequest(r *http.Request, auth SessionReader) Visitor {
	if user, ok := auth.SessionUser(r); ok {
		return Visitor{
			User:               &user,
			FederalRidingID:    strings.TrimSpace(user.FederalRidingID),
			ProvincialRidingID: strings.TrimSpace(user.ProvincialRidingID),
		}
	}
	return fromCookies(r)
}

// fromCookies builds a guest Visitor from the riding cookies, if present.
func fromCookies(r *http.Request) Visitor {
	return Visitor{
		FederalRidingID:    ridingFromCookie(r, FederalRidingCookie),
		ProvincialRidingID: ridingFromCookie(r, ProvincialRidingCookie),
	}
}

// ridingFromCookie reads and URL-decodes a riding cookie value, stripping any
// surrounding quotes that some clients add.
func ridingFromCookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(c.Value)
	if raw == "" {
		return ""
	}
	if decoded, err := url.QueryUnescape(raw); err == nil {
		return strings.Trim(strings.TrimSpace(decoded), "\"")
	}
	return strings.Trim(raw, "\"")
}
