package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/philspins/opendocket/internal/db"
	"github.com/philspins/opendocket/internal/opennorth"
	"github.com/philspins/opendocket/internal/store"
)

const (
	testGuestFederalRidingCookie    = "od_guest_federal_riding_id"
	testGuestProvincialRidingCookie = "od_guest_provincial_riding_id"
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	srv, st, _ := newTestServerWithConn(t)
	return srv, st
}

func newTestServerWithConn(t *testing.T) (*Server, *store.Store, *sql.DB) {
	t.Helper()
	t.Setenv("SES_FROM_EMAIL", "")
	t.Setenv("OAUTH_BASE_URL", "http://127.0.0.1:8080")
	t.Setenv("GOOGLE_MAPS_API_KEY", "test-maps-key")

	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	st := store.New(conn)
	srv := New(st)
	return srv, st, conn
}

func newCompareServerWithTestMembers(t *testing.T) *Server {
	t.Helper()
	t.Setenv("SES_FROM_EMAIL", "")
	t.Setenv("OAUTH_BASE_URL", "http://127.0.0.1:8080")
	t.Setenv("GOOGLE_MAPS_API_KEY", "test-maps-key")

	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	st := store.New(conn)
	srv := New(st)

	_, err = conn.Exec(`
		INSERT INTO members (id, name, party, riding, province, chamber, active, government_level) VALUES
		('f-lib', 'Federal Liberal', 'Liberal', 'Ottawa Centre', 'Ontario', 'commons', 1, 'federal'),
		('f-con', 'Federal Conservative', 'Conservative', 'Calgary Centre', 'Alberta', 'commons', 1, 'federal'),
		('p-on-ndp', 'Ontario NDP', 'NDP', 'Toronto Centre', 'Ontario', 'ontario', 1, 'provincial'),
		('p-qc-caq', 'Quebec CAQ', 'CAQ', 'Quebec City', 'Quebec', 'quebec', 1, 'provincial')
	`)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}
	return srv
}

func TestHandleRequestVerification_DoesNotLeakSecrets(t *testing.T) {
	srv, _ := newTestServer(t)

	form := url.Values{}
	form.Set("email", "verify1@example.com")
	req := httptest.NewRequest(http.MethodPost, "/auth/request-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if strings.Contains(body, "token") || strings.Contains(body, "code") || strings.Contains(body, "verify_url") {
		t.Fatalf("response leaked secrets: %s", body)
	}
	if strings.TrimSpace(body) != `{"ok":true}` {
		t.Fatalf("unexpected response body: %s", body)
	}
}

func TestHandleVerifyEmail_ByCode_SetsSessionAndVerifies(t *testing.T) {
	srv, st := newTestServer(t)

	email := "verify-code@example.com"
	_, code, err := st.CreateEmailVerification(email, time.Hour)
	if err != nil {
		t.Fatalf("CreateEmailVerification: %v", err)
	}

	form := url.Values{}
	form.Set("email", email)
	form.Set("code", code)
	req := httptest.NewRequest(http.MethodPost, "/auth/verify", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	cookies := rr.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "od_session" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected od_session cookie to be set")
	}

	u, err := st.GetUserByEmail(email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if !u.EmailVerified {
		t.Fatalf("expected user to be email-verified")
	}
}

func TestHandleVerifyEmail_ByToken_SetsSessionAndVerifies(t *testing.T) {
	srv, st := newTestServer(t)

	email := "verify-token@example.com"
	token, _, err := st.CreateEmailVerification(email, time.Hour)
	if err != nil {
		t.Fatalf("CreateEmailVerification: %v", err)
	}

	form := url.Values{}
	form.Set("token", token)
	req := httptest.NewRequest(http.MethodPost, "/auth/verify", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	u, err := st.GetUserByEmail(email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if !u.EmailVerified {
		t.Fatalf("expected user to be email-verified")
	}
}

func TestHandleVerifyEmail_InvalidCredentials(t *testing.T) {
	srv, _ := newTestServer(t)

	form := url.Values{}
	form.Set("email", "nobody@example.com")
	form.Set("code", "000000")
	req := httptest.NewRequest(http.MethodPost, "/auth/verify", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleRequestVerification_RateLimitedByEmail(t *testing.T) {
	srv, _ := newTestServer(t)

	email := "rate@example.com"
	for i := 0; i < 3; i++ {
		form := url.Values{}
		form.Set("email", email)
		req := httptest.NewRequest(http.MethodPost, "/auth/request-verification", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d status=%d want %d", i+1, rr.Code, http.StatusOK)
		}
	}

	form := url.Values{}
	form.Set("email", email)
	req := httptest.NewRequest(http.MethodPost, "/auth/request-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusTooManyRequests)
	}
}

func TestSecurityHeaders_AreSetOnResponses(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing X-Content-Type-Options header")
	}
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("missing X-Frame-Options header")
	}
	if rr.Header().Get("Referrer-Policy") == "" {
		t.Fatalf("missing Referrer-Policy header")
	}
	if rr.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("missing Content-Security-Policy header")
	}
}

func TestHandleFollow_RequiresAuthenticatedSession(t *testing.T) {
	srv, _ := newTestServer(t)
	form := url.Values{}
	form.Set("member_id", "m1")
	req := httptest.NewRequest(http.MethodPost, "/api/follow", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rr.Body.String(), "authentication_required") {
		t.Fatalf("expected authentication_required error body, got: %s", rr.Body.String())
	}
}

func TestHandleReact_RateLimitedPerUser(t *testing.T) {
	t.Setenv("BILL_INTERACTION_RATE_LIMIT_PER_MINUTE", "2")
	srv, st := newTestServer(t)
	u, err := st.AuthenticateOAuth("google", "rate-user", "rate-react@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sessionID, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/react", nil)
		req.AddCookie(&http.Cookie{Name: "od_session", Value: sessionID})
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d unexpectedly rate-limited", i+1)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/react", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sessionID})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusTooManyRequests)
	}
}

func TestLegalPages_Render(t *testing.T) {
	srv, _ := newTestServer(t)

	for _, path := range []string{"/privacy", "/tos", "/delete-data"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("path=%s status=%d want %d", path, rr.Code, http.StatusOK)
		}
	}
}

func TestHandleCompare_RendersDropdownFiltersAndSelectedValues(t *testing.T) {
	srv := newCompareServerWithTestMembers(t)

	req := httptest.NewRequest(http.MethodGet, "/compare?level=provincial&province=Ontario&party=NDP", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `name="province"`) {
		t.Fatalf("expected province filter for provincial compare view")
	}
	if strings.Contains(body, `type="text" name="a"`) || strings.Contains(body, `type="text" name="b"`) {
		t.Fatalf("expected compare page to use dropdown selectors for both representatives")
	}
	for _, needle := range []string{
		`name="a"`,
		`name="b"`,
		`name="party"`,
		`value="provincial" selected`,
		`onchange="this.form.submit()"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected compare page body to contain %q", needle)
		}
	}
}

func TestHandleCompare_FiltersPartiesAndCandidatesByLevelAndProvince(t *testing.T) {
	srv := newCompareServerWithTestMembers(t)

	reqFederal := httptest.NewRequest(http.MethodGet, "/compare?level=federal", nil)
	rrFederal := httptest.NewRecorder()
	srv.ServeHTTP(rrFederal, reqFederal)
	bodyFederal := rrFederal.Body.String()
	if !strings.Contains(bodyFederal, `name="province"`) {
		t.Fatalf("expected province selector to always be rendered")
	}
	if strings.Contains(bodyFederal, `value="NDP"`) || strings.Contains(bodyFederal, `value="CAQ"`) {
		t.Fatalf("expected provincial parties to be hidden in federal mode")
	}
	if !strings.Contains(bodyFederal, `Federal Liberal (Liberal)`) || strings.Contains(bodyFederal, `Ontario NDP (NDP)`) {
		t.Fatalf("expected candidate list to include only federal candidates in federal mode")
	}

	reqProvincial := httptest.NewRequest(http.MethodGet, "/compare?level=provincial&province=Ontario", nil)
	rrProvincial := httptest.NewRecorder()
	srv.ServeHTTP(rrProvincial, reqProvincial)
	bodyProvincial := rrProvincial.Body.String()
	if !strings.Contains(bodyProvincial, `name="province"`) {
		t.Fatalf("expected province selector to be shown in provincial mode")
	}
	if strings.Contains(bodyProvincial, `value="Liberal"`) || strings.Contains(bodyProvincial, `value="Conservative"`) {
		t.Fatalf("expected federal parties to be hidden in provincial mode")
	}
	if !strings.Contains(bodyProvincial, `value="NDP"`) || strings.Contains(bodyProvincial, `value="CAQ"`) {
		t.Fatalf("expected party list to be filtered by selected province in provincial mode")
	}
	if !strings.Contains(bodyProvincial, `Ontario NDP (NDP)`) || strings.Contains(bodyProvincial, `Quebec CAQ (CAQ)`) {
		t.Fatalf("expected candidate list to be filtered by selected province in provincial mode")
	}
}

func TestDeleteDataCallback_ValidSignedRequest(t *testing.T) {
	srv, _ := newTestServer(t)
	t.Setenv("FACEBOOK_CLIENT_ID", "123456789")
	t.Setenv("FACEBOOK_CLIENT_SECRET", "test-app-secret")
	t.Setenv("OAUTH_BASE_URL", "https://opendocket.ca")

	signed := buildSignedRequest(t, "test-app-secret", map[string]any{
		"algorithm": "HMAC-SHA256",
		"app_id":    "123456789",
		"user_id":   "meta-user-1",
	})

	form := url.Values{}
	form.Set("signed_request", signed)
	req := httptest.NewRequest(http.MethodPost, "/delete-data", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp["confirmation_code"] == "" {
		t.Fatalf("expected confirmation_code in response")
	}
	if !strings.HasPrefix(resp["url"], "https://opendocket.ca/delete-data?confirmation_code=") {
		t.Fatalf("unexpected status url: %s", resp["url"])
	}
}

func TestDeleteDataCallback_RejectsInvalidRequest(t *testing.T) {
	srv, _ := newTestServer(t)
	t.Setenv("FACEBOOK_CLIENT_SECRET", "test-app-secret")

	form := url.Values{}
	form.Set("signed_request", "invalid")
	req := httptest.NewRequest(http.MethodPost, "/delete-data", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "invalid_signed_request") {
		t.Fatalf("expected invalid_signed_request error body, got: %s", rr.Body.String())
	}
}

func TestHandleRiding_PersistsOnlyRidingsForSessionUser(t *testing.T) {
	srv, st := newTestServer(t)
	u, err := st.UpsertUser("lookup@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	srv.riding.SetLookups(
		func(_ context.Context, _ string, _ string) (float64, float64, error) {
			return 45.0, -75.0, nil
		},
		func(_ context.Context, _, _ float64) ([]opennorth.Representative, error) {
			return []opennorth.Representative{
				{Name: "Jane MP", ElectedOffice: "MP", DistrictName: "Ottawa Centre"},
				{Name: "John MPP", ElectedOffice: "MPP (ON)", DistrictName: "Ottawa South"},
			}, nil
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/riding?address=123+Main+St,+Ottawa,+ON", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}

	got, err := st.GetUserByEmail("lookup@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.Address != "" {
		t.Fatalf("Address=%q want empty", got.Address)
	}
	if got.FederalRidingID != "Ottawa Centre" || got.ProvincialRidingID != "Ottawa South" {
		t.Fatalf("unexpected riding ids: %+v", got)
	}
}

func TestHandleProfile_PostSavesOnlyRidings(t *testing.T) {
	srv, st := newTestServer(t)
	u, err := st.UpsertUser("profile-save@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	srv.riding.SetLookups(
		func(_ context.Context, _ string, _ string) (float64, float64, error) {
			return 45.0, -75.0, nil
		},
		func(_ context.Context, _, _ float64) ([]opennorth.Representative, error) {
			return []opennorth.Representative{
				{Name: "Jane MP", ElectedOffice: "MP", DistrictName: "Ottawa Centre"},
				{Name: "John MPP", ElectedOffice: "MPP (ON)", DistrictName: "Ottawa South"},
			}, nil
		},
	)

	form := url.Values{}
	form.Set("address", "456 Elm St, Ottawa, ON")
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	if rr.Header().Get("Location") != "/profile?updated=1" {
		t.Fatalf("unexpected redirect location: %s", rr.Header().Get("Location"))
	}

	got, err := st.GetUserByEmail("profile-save@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.Address != "" {
		t.Fatalf("Address=%q want empty", got.Address)
	}
	if got.FederalRidingID != "Ottawa Centre" || got.ProvincialRidingID != "Ottawa South" {
		t.Fatalf("unexpected riding ids: %+v", got)
	}
}

func TestHandleProfile_PostManualRidingSelectionSavesRidings(t *testing.T) {
	srv, st := newTestServer(t)
	u, err := st.UpsertUser("profile-manual@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	form := url.Values{}
	form.Set("federal_riding", "Ottawa Centre")
	form.Set("provincial_riding", "Ottawa South")
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	if rr.Header().Get("Location") != "/profile?updated=1" {
		t.Fatalf("unexpected redirect location: %s", rr.Header().Get("Location"))
	}

	got, err := st.GetUserByEmail("profile-manual@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.FederalRidingID != "Ottawa Centre" || got.ProvincialRidingID != "Ottawa South" {
		t.Fatalf("unexpected riding ids: %+v", got)
	}
}

func TestHandleProfile_GetRendersManualRidingDropdowns(t *testing.T) {
	srv, st, conn := newTestServerWithConn(t)
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "mp-drop-test",
		Name:            "Test MP",
		Party:           "Liberal",
		Riding:          "Ottawa Centre",
		Chamber:         "commons",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "federal",
	}); err != nil {
		t.Fatalf("UpsertMember federal: %v", err)
	}
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "mpp-drop-test",
		Name:            "Test MPP",
		Party:           "NDP",
		Riding:          "Ottawa South",
		Chamber:         "ontario",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "provincial",
	}); err != nil {
		t.Fatalf("UpsertMember provincial: %v", err)
	}
	u, err := st.UpsertUser("profile-dropdowns@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `name="federal_riding"`) {
		t.Fatalf("expected federal_riding dropdown in profile page")
	}
	if !strings.Contains(body, `name="provincial_riding"`) {
		t.Fatalf("expected provincial_riding dropdown in profile page")
	}
	if !strings.Contains(body, "Ottawa Centre") {
		t.Fatalf("expected federal riding option in profile page")
	}
	if !strings.Contains(body, "Ottawa South") {
		t.Fatalf("expected provincial riding option in profile page")
	}
}

func TestHandleRiding_UnauthenticatedSetsRidingCookies(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.riding.SetLookups(
		func(_ context.Context, _ string, _ string) (float64, float64, error) {
			return 45.0, -75.0, nil
		},
		func(_ context.Context, _, _ float64) ([]opennorth.Representative, error) {
			return []opennorth.Representative{
				{Name: "Jane MP", ElectedOffice: "MP", DistrictName: "Ottawa Centre"},
				{Name: "John MPP", ElectedOffice: "MPP (ON)", DistrictName: "Ottawa South"},
			}, nil
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/riding?address=123+Main+St,+Ottawa,+ON", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	var gotFederal, gotProvincial string
	for _, c := range rr.Result().Cookies() {
		switch c.Name {
		case testGuestFederalRidingCookie:
			gotFederal = c.Value
		case testGuestProvincialRidingCookie:
			gotProvincial = c.Value
		}
	}
	if gotFederal != url.QueryEscape("Ottawa Centre") || gotProvincial != url.QueryEscape("Ottawa South") {
		t.Fatalf("unexpected cookie riding ids: federal=%q provincial=%q", gotFederal, gotProvincial)
	}
}

func TestHandleHome_UsesSavedRepresentativesAndHidesLookupHero(t *testing.T) {
	srv, st, conn := newTestServerWithConn(t)
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "mp-ottawa-centre",
		Name:            "Yasir Naqvi",
		Party:           "Liberal",
		Riding:          "Ottawa Centre",
		Chamber:         "commons",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "federal",
	}); err != nil {
		t.Fatalf("UpsertMember federal: %v", err)
	}
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "mpp-ottawa-south",
		Name:            "John Fraser",
		Party:           "Ontario Liberal Party",
		Riding:          "Ottawa South",
		Chamber:         "ontario",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "provincial",
	}); err != nil {
		t.Fatalf("UpsertMember provincial: %v", err)
	}
	u, err := st.UpsertUser("home@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	_, err = st.UpdateUserLocation(u.ID, "Ottawa Centre", "Ottawa South")
	if err != nil {
		t.Fatalf("UpdateUserLocation: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Ottawa Centre") || !strings.Contains(body, "Ottawa South") {
		t.Fatalf("expected saved riding context in home page body")
	}
	if !strings.Contains(body, "John Fraser") {
		t.Fatalf("expected provincial representative name from local member in home page body")
	}
	if !strings.Contains(body, "href=\"/members/mp-ottawa-centre\"") {
		t.Fatalf("expected federal representative name to link to member profile")
	}
	if !strings.Contains(body, "href=\"/members/mpp-ottawa-south\"") {
		t.Fatalf("expected provincial representative name to link to member profile")
	}
	if strings.Contains(body, "Find Your Riding") {
		t.Fatalf("expected lookup hero to be hidden once address is saved")
	}
}

func TestHandleHome_UnauthenticatedUsesRidingCookies(t *testing.T) {
	srv, _, conn := newTestServerWithConn(t)
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "mpp-london-fanshawe",
		Name:            "Teresa J. Armstrong",
		Party:           "Ontario NDP",
		Riding:          "London—Fanshawe",
		Chamber:         "ontario",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "provincial",
	}); err != nil {
		t.Fatalf("UpsertMember provincial: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: testGuestFederalRidingCookie, Value: url.QueryEscape("Ottawa Centre")})
	req.AddCookie(&http.Cookie{Name: testGuestProvincialRidingCookie, Value: url.QueryEscape("London—Fanshawe")})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Ottawa Centre") || !strings.Contains(body, "London—Fanshawe") {
		t.Fatalf("expected riding cookie context in home page body")
	}
	if !strings.Contains(body, "Teresa J. Armstrong") {
		t.Fatalf("expected provincial representative name from local member in home page body")
	}
	if strings.Contains(body, "Find Your Riding") {
		t.Fatalf("expected lookup hero to be hidden once riding cookies are set")
	}
}

func TestHandleHome_UnauthenticatedUsesCookiesFromRidingLookup(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.riding.SetLookups(
		func(_ context.Context, _ string, _ string) (float64, float64, error) {
			return 45.0, -75.0, nil
		},
		func(_ context.Context, _, _ float64) ([]opennorth.Representative, error) {
			return []opennorth.Representative{
				{Name: "Jane MP", ElectedOffice: "MP", DistrictName: "Ottawa Centre"},
				{Name: "John MPP", ElectedOffice: "MPP (ON)", DistrictName: "Ottawa South"},
			}, nil
		},
	)

	lookupReq := httptest.NewRequest(http.MethodGet, "/riding?address=123+Main+St,+Ottawa,+ON", nil)
	lookupRR := httptest.NewRecorder()
	srv.ServeHTTP(lookupRR, lookupReq)
	if lookupRR.Code != http.StatusOK {
		t.Fatalf("lookup status=%d want %d", lookupRR.Code, http.StatusOK)
	}

	homeReq := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range lookupRR.Result().Cookies() {
		homeReq.AddCookie(c)
	}
	homeRR := httptest.NewRecorder()
	srv.ServeHTTP(homeRR, homeReq)
	if homeRR.Code != http.StatusOK {
		t.Fatalf("home status=%d want %d", homeRR.Code, http.StatusOK)
	}
	body := homeRR.Body.String()
	if !strings.Contains(body, "Ottawa Centre") || !strings.Contains(body, "Ottawa South") {
		t.Fatalf("expected riding context from lookup cookies in home page body")
	}
}

func TestHandleHome_UnauthenticatedUsesUnicodeRidingCookiesFromLookup(t *testing.T) {
	srv, _ := newTestServer(t)
	const federalRiding = "Bonavista—Burin—Trinity"
	const provincialRiding = "Charlottetown-Lewis Point"
	srv.riding.SetLookups(
		func(_ context.Context, _ string, _ string) (float64, float64, error) {
			return 45.0, -75.0, nil
		},
		func(_ context.Context, _, _ float64) ([]opennorth.Representative, error) {
			return []opennorth.Representative{
				{Name: "Jane MP", ElectedOffice: "MP", DistrictName: federalRiding},
				{Name: "John MLA", ElectedOffice: "MLA", DistrictName: provincialRiding},
			}, nil
		},
	)

	lookupReq := httptest.NewRequest(http.MethodGet, "/riding?address=123+Main+St,+St.+John's,+NL", nil)
	lookupRR := httptest.NewRecorder()
	srv.ServeHTTP(lookupRR, lookupReq)
	if lookupRR.Code != http.StatusOK {
		t.Fatalf("lookup status=%d want %d", lookupRR.Code, http.StatusOK)
	}

	var gotFederalCookie, gotProvincialCookie string
	for _, c := range lookupRR.Result().Cookies() {
		switch c.Name {
		case testGuestFederalRidingCookie:
			gotFederalCookie = c.Value
		case testGuestProvincialRidingCookie:
			gotProvincialCookie = c.Value
		}
	}
	if gotFederalCookie != url.QueryEscape(federalRiding) {
		t.Fatalf("federal cookie=%q want %q", gotFederalCookie, url.QueryEscape(federalRiding))
	}
	if gotProvincialCookie != url.QueryEscape(provincialRiding) {
		t.Fatalf("provincial cookie=%q want %q", gotProvincialCookie, url.QueryEscape(provincialRiding))
	}

	homeReq := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range lookupRR.Result().Cookies() {
		homeReq.AddCookie(c)
	}
	homeRR := httptest.NewRecorder()
	srv.ServeHTTP(homeRR, homeReq)
	if homeRR.Code != http.StatusOK {
		t.Fatalf("home status=%d want %d", homeRR.Code, http.StatusOK)
	}
	body := homeRR.Body.String()
	if !strings.Contains(body, federalRiding) || !strings.Contains(body, provincialRiding) {
		t.Fatalf("expected unicode riding context from lookup cookies in home page body")
	}
}

func TestHandleHome_ShowsRecentBillVotesForSelectedRepresentatives(t *testing.T) {
	srv, st, conn := newTestServerWithConn(t)

	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "mp-1",
		Name:            "Jane MP",
		Riding:          "Ottawa Centre",
		Chamber:         "commons",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "federal",
	}); err != nil {
		t.Fatalf("UpsertMember mp-1: %v", err)
	}
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "mpp-1",
		Name:            "John MPP",
		Riding:          "Ottawa South",
		Chamber:         "ontario",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "provincial",
	}); err != nil {
		t.Fatalf("UpsertMember mpp-1: %v", err)
	}
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "other-1",
		Name:            "Another MP",
		Riding:          "Elsewhere",
		Chamber:         "commons",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "federal",
	}); err != nil {
		t.Fatalf("UpsertMember other-1: %v", err)
	}

	for _, bill := range []store.BillRecord{
		{ID: "bill-fed-1", Parliament: 45, Session: 1, Number: "C-1", Title: "Federal bill one", LastScraped: "2026-01-01T00:00:00Z"},
		{ID: "bill-fed-2", Parliament: 45, Session: 1, Number: "C-2", Title: "Federal bill two", LastScraped: "2026-01-01T00:00:00Z"},
		{ID: "bill-prov-1", Parliament: 1, Session: 1, Number: "ON-1", Title: "Provincial bill one", LastScraped: "2026-01-01T00:00:00Z"},
		{ID: "bill-other", Parliament: 45, Session: 1, Number: "C-999", Title: "Other bill", LastScraped: "2026-01-01T00:00:00Z"},
	} {
		if err := store.UpsertBill(conn, bill); err != nil {
			t.Fatalf("UpsertBill %s: %v", bill.ID, err)
		}
	}

	for _, div := range []store.DivisionRecord{
		{ID: "div-fed-old", Parliament: 45, Session: 1, Number: 1, Date: "2026-01-01", BillID: "bill-fed-1", Description: "Federal bill one older vote", Result: "Passed", Chamber: "commons", LastScraped: "2026-01-01T00:00:00Z"},
		{ID: "div-fed-new", Parliament: 45, Session: 1, Number: 2, Date: "2026-02-01", BillID: "bill-fed-1", Description: "Federal bill one latest vote", Result: "Passed", Chamber: "commons", LastScraped: "2026-01-01T00:00:00Z"},
		{ID: "div-fed-two", Parliament: 45, Session: 1, Number: 3, Date: "2026-01-15", BillID: "bill-fed-2", Description: "Federal bill two vote", Result: "Passed", Chamber: "commons", LastScraped: "2026-01-01T00:00:00Z"},
		{ID: "div-prov-one", Parliament: 1, Session: 1, Number: 1, Date: "2026-01-20", BillID: "bill-prov-1", Description: "Provincial bill vote", Result: "Passed", Chamber: "ontario", LastScraped: "2026-01-01T00:00:00Z"},
		{ID: "div-other", Parliament: 45, Session: 1, Number: 4, Date: "2026-02-03", BillID: "bill-other", Description: "Unrelated member vote", Result: "Passed", Chamber: "commons", LastScraped: "2026-01-01T00:00:00Z"},
	} {
		if err := store.UpsertDivision(conn, div); err != nil {
			t.Fatalf("UpsertDivision %s: %v", div.ID, err)
		}
	}

	for _, mv := range []struct {
		divID    string
		memberID string
		vote     string
	}{
		{divID: "div-fed-old", memberID: "mp-1", vote: "Yea"},
		{divID: "div-fed-new", memberID: "mp-1", vote: "Nay"},
		{divID: "div-fed-two", memberID: "mp-1", vote: "Yea"},
		{divID: "div-prov-one", memberID: "mpp-1", vote: "Yea"},
		{divID: "div-other", memberID: "other-1", vote: "Nay"},
	} {
		if err := store.UpsertMemberVote(conn, mv.divID, mv.memberID, mv.vote); err != nil {
			t.Fatalf("UpsertMemberVote %+v: %v", mv, err)
		}
	}

	u, err := st.UpsertUser("home-votes@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if _, err := st.UpdateUserLocation(u.ID, "Ottawa Centre", "Ottawa South"); err != nil {
		t.Fatalf("UpdateUserLocation: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	for _, expected := range []string{
		"Federal bill one latest vote",
		"Federal bill two vote",
		"Provincial bill vote",
		">Nay<",
		">Yea<",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected homepage to contain %q", expected)
		}
	}
	for _, excluded := range []string{
		"Federal bill one older vote",
		"Unrelated member vote",
	} {
		if strings.Contains(body, excluded) {
			t.Fatalf("did not expect homepage to contain %q", excluded)
		}
	}
}

func TestHandleHome_ShowsProvincialPlaceholderWhenOnlyFederalRidingSet(t *testing.T) {
	srv, st, conn := newTestServerWithConn(t)
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "mp-ottawa-centre-prov",
		Name:            "Yasir Naqvi",
		Party:           "Liberal",
		Riding:          "Ottawa Centre",
		Chamber:         "commons",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "federal",
	}); err != nil {
		t.Fatalf("UpsertMember: %v", err)
	}
	u, err := st.UpsertUser("home-fed-only@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if _, err := st.UpdateUserLocation(u.ID, "Ottawa Centre", ""); err != nil {
		t.Fatalf("UpdateUserLocation: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Provincial representative not found") {
		t.Fatalf("expected provincial placeholder when provincial riding is not set")
	}
	if strings.Contains(body, "Federal representative not found") {
		t.Fatalf("did not expect federal placeholder when federal riding is set")
	}
	if !strings.Contains(body, "Yasir Naqvi") {
		t.Fatalf("expected federal representative name to be shown")
	}
	if !strings.Contains(body, `href="/profile"`) {
		t.Fatalf("expected profile link in provincial placeholder for logged-in user with riding set")
	}
}

func TestHandleHome_ShowsFederalPlaceholderWhenOnlyProvincialRidingSet(t *testing.T) {
	srv, st, conn := newTestServerWithConn(t)
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID:              "mpp-ottawa-south-fed",
		Name:            "John Fraser",
		Party:           "Ontario Liberal Party",
		Riding:          "Ottawa South",
		Chamber:         "ontario",
		Active:          true,
		LastScraped:     "2026-01-01T00:00:00Z",
		GovernmentLevel: "provincial",
	}); err != nil {
		t.Fatalf("UpsertMember: %v", err)
	}
	u, err := st.UpsertUser("home-prov-only@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if _, err := st.UpdateUserLocation(u.ID, "", "Ottawa South"); err != nil {
		t.Fatalf("UpdateUserLocation: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Federal representative not found") {
		t.Fatalf("expected federal placeholder when federal riding is not set")
	}
	if strings.Contains(body, "Provincial representative not found") {
		t.Fatalf("did not expect provincial placeholder when provincial riding is set")
	}
	if !strings.Contains(body, "John Fraser") {
		t.Fatalf("expected provincial representative name to be shown")
	}
	if !strings.Contains(body, `href="/profile"`) {
		t.Fatalf("expected profile link in federal placeholder for logged-in user with riding set")
	}
}

func TestHandleHome_UnauthenticatedNoAddressHidesRepSections(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Find Your Riding") {
		t.Fatalf("expected hero section with Find Your Riding button for unauthenticated user with no address")
	}
	if strings.Contains(body, "representative not found") {
		t.Fatalf("expected rep-grid to be hidden for unauthenticated user with no address")
	}
	if strings.Contains(body, "Recent provincial bill votes") {
		t.Fatalf("expected Overall section to be hidden for unauthenticated user with no address")
	}
}

func TestRecentBillVotes_DeduplicatesAndFallsBackToNonBillDivisions(t *testing.T) {
	// Bill-linked votes come first and are deduplicated; non-bill votes fill remaining slots.
	got := recentBillVotes([]store.VoteRow{
		{BillID: "", DivisionID: "d-0", Description: "No bill"},
		{BillID: "b-1", DivisionID: "d-1", Description: "Latest for b-1"},
		{BillID: "b-1", DivisionID: "d-1b", Description: "Older for b-1"},
		{BillID: "b-2", DivisionID: "d-2", Description: "Only for b-2"},
	}, 5)

	if len(got) != 3 {
		t.Fatalf("len=%d want 3 (2 bill + 1 non-bill fallback)", len(got))
	}
	if got[0].Description != "Latest for b-1" {
		t.Fatalf("first description=%q want latest b-1 vote", got[0].Description)
	}
	if got[1].Description != "Only for b-2" {
		t.Fatalf("second description=%q want b-2 vote", got[1].Description)
	}
	if got[2].Description != "No bill" {
		t.Fatalf("third description=%q want fallback non-bill vote", got[2].Description)
	}

	// With a tight limit, non-bill votes should NOT appear if bill slots are full.
	got2 := recentBillVotes([]store.VoteRow{
		{BillID: "", DivisionID: "d-0", Description: "No bill"},
		{BillID: "b-1", DivisionID: "d-1", Description: "B1 vote"},
		{BillID: "b-2", DivisionID: "d-2", Description: "B2 vote"},
	}, 2)
	if len(got2) != 2 {
		t.Fatalf("len=%d want 2", len(got2))
	}
	for _, v := range got2 {
		if strings.TrimSpace(v.BillID) == "" {
			t.Fatalf("expected only bill votes when limit is filled by bills, got non-bill: %q", v.Description)
		}
	}

	// With only non-bill votes (e.g. provincial legislature), all slots fill from divisions.
	got3 := recentBillVotes([]store.VoteRow{
		{BillID: "", DivisionID: "d-a", Description: "Div A"},
		{BillID: "", DivisionID: "d-b", Description: "Div B"},
		{BillID: "", DivisionID: "d-a", Description: "Div A duplicate"},
	}, 5)
	if len(got3) != 2 {
		t.Fatalf("len=%d want 2 unique non-bill votes", len(got3))
	}

	got4 := recentBillVotes([]store.VoteRow{
		{BillID: "", DivisionID: "", Description: "No division ID"},
		{BillID: "", DivisionID: "d-z", Description: "Has division ID"},
	}, 5)
	if len(got4) != 1 || got4[0].DivisionID != "d-z" {
		t.Fatalf("expected only non-bill votes with division IDs, got %+v", got4)
	}
}

func TestDedupeBillDetailDivisions_DeduplicatesByID(t *testing.T) {
	// Two rows with the same ID (can happen if a query returns duplicate rows)
	// must be collapsed. Rows with distinct IDs must all be kept, even when
	// their date and description look equivalent after normalisation.
	got := dedupeBillDetailDivisions([]store.DivisionRow{
		{ID: "d-1", Date: "2026-04-28", Description: "Miscellaneous Statutes Amendment Act, 2026"},
		{ID: "d-1", Date: "2026-04-28", Description: "Miscellaneous Statutes Amendment Act, 2026"}, // duplicate ID
		{ID: "d-2", Date: "2026-04-28", Description: "  miscellaneous statutes amendment act,  2026  "},
		{ID: "d-3", Date: "2026-04-28", Description: "Miscellaneous Statutes Amendment Act, 2026 at third reading"},
		{ID: "d-4", Date: "2026-04-29", Description: "Miscellaneous Statutes Amendment Act, 2026"},
	})

	// d-1 appears twice but should only be kept once; the other three distinct
	// IDs are all kept regardless of how similar their descriptions are.
	if len(got) != 4 {
		t.Fatalf("len=%d want 4", len(got))
	}
	if got[0].ID != "d-1" {
		t.Fatalf("first kept id=%q want d-1", got[0].ID)
	}
	if got[1].ID != "d-2" {
		t.Fatalf("second kept id=%q want d-2", got[1].ID)
	}
	if got[2].ID != "d-3" {
		t.Fatalf("third kept id=%q want d-3", got[2].ID)
	}
	if got[3].ID != "d-4" {
		t.Fatalf("fourth kept id=%q want d-4", got[3].ID)
	}
}

func TestResolveRepresentativeMemberID_DoesNotFallbackToWrongGovernmentLevel(t *testing.T) {
	srv, _, conn := newTestServerWithConn(t)
	for _, member := range []store.MemberRecord{
		{
			ID:              "federal-ottawa-centre",
			Name:            "Federal Rep",
			Riding:          "Ottawa Centre",
			Chamber:         "commons",
			Active:          true,
			GovernmentLevel: "federal",
			LastScraped:     "2026-01-01T00:00:00Z",
		},
		{
			ID:              "provincial-ottawa-centre",
			Name:            "Provincial Rep",
			Riding:          "Ottawa Centre",
			Chamber:         "ontario",
			Active:          true,
			GovernmentLevel: "provincial",
			LastScraped:     "2026-01-01T00:00:00Z",
		},
	} {
		if err := store.UpsertMember(conn, member); err != nil {
			t.Fatalf("UpsertMember %s: %v", member.ID, err)
		}
	}

	federalID := srv.resolveRepresentativeMemberID(opennorth.Representative{
		Name:         "No Match",
		DistrictName: "Ottawa Centre",
	}, true)
	if federalID != "federal-ottawa-centre" {
		t.Fatalf("federalID=%q want federal-ottawa-centre", federalID)
	}

	provincialID := srv.resolveRepresentativeMemberID(opennorth.Representative{
		Name:         "No Match",
		DistrictName: "Ottawa Centre",
	}, false)
	if provincialID != "provincial-ottawa-centre" {
		t.Fatalf("provincialID=%q want provincial-ottawa-centre", provincialID)
	}

	missingLevelID := srv.resolveRepresentativeMemberID(opennorth.Representative{
		Name:         "No Match",
		DistrictName: "No Such Riding",
	}, false)
	if missingLevelID != "" {
		t.Fatalf("missingLevelID=%q want empty", missingLevelID)
	}
}

func TestHandleHealth_Returns200OK(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type=%q want application/json", ct)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != `{"status":"ok"}` {
		t.Fatalf("unexpected health response body: %s", body)
	}
}

func TestHTTPSRedirect_WhenTrustProxyAndHTTPSBaseURL(t *testing.T) {
	_, st := newTestServer(t)
	t.Setenv("OAUTH_BASE_URL", "https://opendocket.ca")
	t.Setenv("TRUST_PROXY", "true")
	srv := New(st)

	req := httptest.NewRequest(http.MethodGet, "/bills", nil)
	req.Host = "opendocket.ca"
	req.Header.Set("X-Forwarded-Proto", "http")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("status=%d want %d (redirect to HTTPS)", rr.Code, http.StatusMovedPermanently)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://opendocket.ca/") {
		t.Fatalf("redirect location %q should start with https://opendocket.ca/ (configured host, not spoofable)", loc)
	}
}

func TestHTTPSRedirect_UsesConfiguredHostNotRequestHost(t *testing.T) {
	// Ensure Host header injection is prevented: redirect must use the host
	// from OAUTH_BASE_URL, not the potentially attacker-controlled r.Host.
	_, st := newTestServer(t)
	t.Setenv("OAUTH_BASE_URL", "https://opendocket.ca")
	t.Setenv("TRUST_PROXY", "true")
	srv := New(st)

	req := httptest.NewRequest(http.MethodGet, "/bills", nil)
	req.Host = "evil.com" // attacker-controlled Host header
	req.Header.Set("X-Forwarded-Proto", "http")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusMovedPermanently)
	}
	loc := rr.Header().Get("Location")
	if strings.Contains(loc, "evil.com") {
		t.Fatalf("redirect location %q must not contain attacker Host header value", loc)
	}
	if !strings.HasPrefix(loc, "https://opendocket.ca/") {
		t.Fatalf("redirect location %q should use configured host opendocket.ca", loc)
	}
}

func TestHTTPSRedirect_SkipsHealthEndpoint(t *testing.T) {
	_, st := newTestServer(t)
	t.Setenv("OAUTH_BASE_URL", "https://opendocket.ca")
	t.Setenv("TRUST_PROXY", "true")
	srv := New(st)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = "opendocket.ca"
	req.Header.Set("X-Forwarded-Proto", "http")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d (health must not redirect)", rr.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != `{"status":"ok"}` {
		t.Fatalf("unexpected health body: %s", body)
	}
}

func TestHTTPSRedirect_SkipsHealthEndpointWithTrailingSlash(t *testing.T) {
	_, st := newTestServer(t)
	t.Setenv("OAUTH_BASE_URL", "https://opendocket.ca")
	t.Setenv("TRUST_PROXY", "true")
	srv := New(st)

	req := httptest.NewRequest(http.MethodGet, "/health/", nil)
	req.Host = "opendocket.ca"
	req.Header.Set("X-Forwarded-Proto", "http")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code == http.StatusMovedPermanently {
		t.Fatalf("status=%d: /health/ must not trigger HTTPS redirect", rr.Code)
	}
}

func TestHTTPSRedirect_NotAppliedWithoutTrustProxy(t *testing.T) {
	_, st := newTestServer(t)
	t.Setenv("OAUTH_BASE_URL", "https://opendocket.ca")
	t.Setenv("TRUST_PROXY", "false")
	srv := New(st)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "http")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code == http.StatusMovedPermanently {
		t.Fatalf("should not redirect when TRUST_PROXY=false")
	}
}

func TestHTTPSRedirect_NotAppliedWhenAlreadyHTTPS(t *testing.T) {
	_, st := newTestServer(t)
	t.Setenv("OAUTH_BASE_URL", "https://opendocket.ca")
	t.Setenv("TRUST_PROXY", "true")
	srv := New(st)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code == http.StatusMovedPermanently {
		t.Fatalf("should not redirect when X-Forwarded-Proto is already https")
	}
}

func TestHSTSHeader_SetWhenForwardedProtoHTTPS(t *testing.T) {
	_, st := newTestServer(t)
	t.Setenv("OAUTH_BASE_URL", "https://opendocket.ca")
	t.Setenv("TRUST_PROXY", "true")
	srv := New(st)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	hsts := rr.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Fatalf("expected Strict-Transport-Security header when X-Forwarded-Proto: https and TRUST_PROXY=true")
	}
	if !strings.Contains(hsts, "max-age=") {
		t.Fatalf("HSTS header missing max-age directive: %s", hsts)
	}
}

func TestHSTSHeader_NotSetWhenTrustProxyFalse(t *testing.T) {
	// TRUST_PROXY=false: X-Forwarded-Proto should be ignored; HSTS not set
	// (OAUTH_BASE_URL is http, default test env)
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if hsts := rr.Header().Get("Strict-Transport-Security"); hsts != "" {
		t.Fatalf("HSTS should not be set when TRUST_PROXY=false, got: %s", hsts)
	}
}

func buildSignedRequest(t *testing.T, secret string, payload map[string]any) string {
	t.Helper()
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal payload: %v", err)
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payloadJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payloadPart))
	sigPart := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return sigPart + "." + payloadPart
}

func TestHandleSubscribeBill_RequiresAuth(t *testing.T) {
	srv, _, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title) VALUES ('b1', 45, 1, 'C-1', 'Test Bill')`)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	form := url.Values{}
	form.Set("bill_id", "b1")
	req := httptest.NewRequest(http.MethodPost, "/api/subscribe-bill", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized && rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 401 or redirect for unauthenticated request, got %d", rr.Code)
	}
}

func TestHandleProfileInterests_RequiresAuth(t *testing.T) {
	srv, _ := newTestServer(t)

	form := url.Values{}
	form.Add("categories", "Housing")
	req := httptest.NewRequest(http.MethodPost, "/profile/interests", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect for unauthenticated request, got %d", rr.Code)
	}
	if !strings.HasSuffix(rr.Header().Get("Location"), "/auth/login") {
		t.Fatalf("expected redirect to /auth/login, got %s", rr.Header().Get("Location"))
	}
}

func TestHandleProfileInterests_SavesPreferences(t *testing.T) {
	srv, st, conn := newTestServerWithConn(t)

	u, err := st.AuthenticateOAuth("google", "cat-user", "cat@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sessionID, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	_ = conn // used by newTestServerWithConn

	form := url.Values{}
	form.Add("categories", "Housing")
	form.Add("categories", "Health")
	req := httptest.NewRequest(http.MethodPost, "/profile/interests", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sessionID})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d: %s", rr.Code, rr.Body.String())
	}

	cats, err := st.GetUserCategoryPreferences(u.ID)
	if err != nil {
		t.Fatalf("GetUserCategoryPreferences: %v", err)
	}
	if len(cats) != 2 {
		t.Errorf("expected 2 saved categories, got %d: %v", len(cats), cats)
	}
}

// ── pure helper functions ─────────────────────────────────────────────────────

func TestProvinceJurisdictionKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"AB", "provincial-AB"}, {"Alberta", "provincial-AB"},
		{"BC", "provincial-BC"}, {"British Columbia", "provincial-BC"},
		{"MB", "provincial-MB"}, {"Manitoba", "provincial-MB"},
		{"NB", "provincial-NB"}, {"New Brunswick", "provincial-NB"},
		{"NL", "provincial-NL"}, {"Newfoundland and Labrador", "provincial-NL"},
		{"NS", "provincial-NS"}, {"Nova Scotia", "provincial-NS"},
		{"ON", "provincial-ON"}, {"Ontario", "provincial-ON"},
		{"PE", "provincial-PE"}, {"PEI", "provincial-PE"}, {"Prince Edward Island", "provincial-PE"},
		{"QC", "provincial-QC"}, {"Quebec", "provincial-QC"}, {"Québec", "provincial-QC"},
		{"SK", "provincial-SK"}, {"Saskatchewan", "provincial-SK"},
		{"YT", "provincial-YT"}, {"Yukon", "provincial-YT"},
		{"NT", "provincial-NT"}, {"Northwest Territories", "provincial-NT"},
		{"NU", "provincial-NU"}, {"Nunavut", "provincial-NU"},
		{"", ""},
		{"Unknown", ""},
	}
	for _, tt := range tests {
		got := provinceJurisdictionKey(tt.input)
		if got != tt.want {
			t.Errorf("provinceJurisdictionKey(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParliamentStatusText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"in_session", "in session"},
		{"on_break", "on break"},
		{"status_unavailable", "status unavailable"},
		{"", "status unavailable"},
		{"unknown", "status unavailable"},
	}
	for _, tt := range tests {
		got := parliamentStatusText(tt.input)
		if got != tt.want {
			t.Errorf("parliamentStatusText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMemberMatchesLevel(t *testing.T) {
	tests := []struct {
		name   string
		member store.MemberRow
		level  string
		want   bool
	}{
		{
			name:   "federal by level",
			member: store.MemberRow{GovernmentLevel: "federal", Chamber: "commons"},
			level:  "federal",
			want:   true,
		},
		{
			name:   "federal commons by chamber",
			member: store.MemberRow{Chamber: "commons"},
			level:  "federal",
			want:   true,
		},
		{
			name:   "federal senate by chamber",
			member: store.MemberRow{Chamber: "senate"},
			level:  "federal",
			want:   true,
		},
		{
			name:   "provincial by level",
			member: store.MemberRow{GovernmentLevel: "provincial", Chamber: "ontario"},
			level:  "provincial",
			want:   true,
		},
		{
			name:   "provincial by non-federal chamber",
			member: store.MemberRow{Chamber: "ontario"},
			level:  "provincial",
			want:   true,
		},
		{
			name:   "commons member is not provincial",
			member: store.MemberRow{Chamber: "commons"},
			level:  "provincial",
			want:   false,
		},
		{
			name:   "senate member is not provincial",
			member: store.MemberRow{Chamber: "senate"},
			level:  "provincial",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := memberMatchesLevel(tt.member, tt.level)
			if got != tt.want {
				t.Errorf("memberMatchesLevel(%+v, %q) = %v, want %v", tt.member, tt.level, got, tt.want)
			}
		})
	}
}

func TestSelectLocalMemberByLevel(t *testing.T) {
	members := []store.MemberRow{
		{ID: "fed-1", Chamber: "commons", GovernmentLevel: "federal", Riding: "Ottawa Centre"},
		{ID: "prov-1", Chamber: "ontario", GovernmentLevel: "provincial", Riding: "Ottawa South"},
	}

	// Exact riding match at federal level.
	m, ok := selectLocalMemberByLevel(members, "Ottawa Centre", "federal")
	if !ok || m.ID != "fed-1" {
		t.Errorf("exact federal riding: got %v, ok=%v", m.ID, ok)
	}

	// Fallback to first match when riding doesn't match exactly.
	m, ok = selectLocalMemberByLevel(members, "Nonexistent Riding", "federal")
	if !ok || m.ID != "fed-1" {
		t.Errorf("fallback federal: got %v, ok=%v", m.ID, ok)
	}

	// Provincial match.
	m, ok = selectLocalMemberByLevel(members, "Ottawa South", "provincial")
	if !ok || m.ID != "prov-1" {
		t.Errorf("exact provincial riding: got %v, ok=%v", m.ID, ok)
	}

	// No match at all — empty slice.
	_, ok = selectLocalMemberByLevel(nil, "Ottawa Centre", "federal")
	if ok {
		t.Error("expected no match for empty member slice")
	}
}

// ── authenticated API endpoints ───────────────────────────────────────────────

func TestHandleFollow_AuthenticatedFollowsMember(t *testing.T) {
	srv, st, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, government_level)
		VALUES ('m-followed', 'Test MP', 'Liberal', 'Ottawa', 'Ontario', 'commons', 1, 'federal')`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}

	u, err := st.AuthenticateOAuth("google", "follow-user", "follower@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	form := url.Values{}
	form.Set("member_id", "m-followed")
	req := httptest.NewRequest(http.MethodPost, "/api/follow", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusSeeOther, rr.Body.String())
	}
}

func TestHandleFollow_MissingMemberID(t *testing.T) {
	srv, st := newTestServer(t)

	u, err := st.AuthenticateOAuth("google", "nomember-user", "nomember@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	form := url.Values{}
	form.Set("member_id", "  ") // whitespace-only → bad request
	req := httptest.NewRequest(http.MethodPost, "/api/follow", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleSubscribeBill_AuthenticatedToggles(t *testing.T) {
	srv, st, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title) VALUES ('b-sub', 45, 1, 'C-99', 'Subscribe Bill')`)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	u, err := st.AuthenticateOAuth("google", "sub-user", "subscriber@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	form := url.Values{}
	form.Set("bill_id", "b-sub")
	req := httptest.NewRequest(http.MethodPost, "/api/subscribe-bill", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("subscribe: status=%d want %d; body=%s", rr.Code, http.StatusSeeOther, rr.Body.String())
	}
}

func TestHandleReact_AuthenticatedReacts(t *testing.T) {
	t.Setenv("BILL_INTERACTION_RATE_LIMIT_PER_MINUTE", "100")
	srv, st, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title) VALUES ('b-react', 45, 1, 'C-88', 'React Bill')`)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	u, err := st.AuthenticateOAuth("google", "react-user2", "reactor@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	_ = conn

	form := url.Values{}
	form.Set("bill_id", "b-react")
	form.Set("reaction", "support")
	req := httptest.NewRequest(http.MethodPost, "/api/react", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("react: status=%d want %d; body=%s", rr.Code, http.StatusSeeOther, rr.Body.String())
	}
}

func TestHandleReact_MissingBillID(t *testing.T) {
	t.Setenv("BILL_INTERACTION_RATE_LIMIT_PER_MINUTE", "100")
	srv, st := newTestServer(t)

	u, err := st.AuthenticateOAuth("google", "react-nobill", "reactnobill@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	form := url.Values{}
	form.Set("bill_id", "")
	form.Set("reaction", "support")
	req := httptest.NewRequest(http.MethodPost, "/api/react", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing bill_id: status=%d want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleBills_RendersPage(t *testing.T) {
	srv, _, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber, last_activity_date)
		VALUES ('b1', 45, 1, 'C-1', 'Test Bill', 'Health', '1st_reading', 'commons', '2026-01-01')`)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/bills", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleVotes_RendersPage(t *testing.T) {
	srv, _, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO divisions (id, parliament, session, number, yeas, nays, paired) VALUES (1, 45, 1, 1, 10, 5, 0)`)
	if err != nil {
		t.Fatalf("insert division: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/votes", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleMembers_RendersPage(t *testing.T) {
	srv, _, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, government_level)
		VALUES ('m1', 'Test MP', 'Liberal', 'Ottawa Centre', 'Ontario', 'commons', 1, 'federal')`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleMemberProfile_RendersPage(t *testing.T) {
	srv, _, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, government_level)
		VALUES ('m-profile', 'Profile MP', 'NDP', 'Vancouver East', 'British Columbia', 'commons', 1, 'federal')`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/members/m-profile", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleBillDetail_RendersPage(t *testing.T) {
	srv, _, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber)
		VALUES ('b-detail', 45, 1, 'C-42', 'Detail Bill', 'General', '1st_reading', 'commons')`)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/bills/b-detail", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleBillDetail_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/bills/nonexistent", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandleBills_WithFilters(t *testing.T) {
	srv, _, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber, last_activity_date)
		VALUES ('b1', 45, 1, 'C-1', 'Health Bill', 'Health', '2nd_reading', 'commons', '2026-01-01'),
		       ('b2', 45, 1, 'C-2', 'Housing Bill', 'Housing', '1st_reading', 'senate', '2026-01-02')`)
	if err != nil {
		t.Fatalf("insert bills: %v", err)
	}

	for _, tc := range []struct {
		query string
		name  string
	}{
		{"/bills?stage=2nd_reading", "stage filter"},
		{"/bills?category=Health", "category filter"},
		{"/bills?chamber=commons", "chamber filter"},
		{"/bills?sort=stage", "sort=stage"},
		{"/bills?per_page=5", "per_page=5"},
		{"/bills?per_page=25", "per_page=25"},
		{"/bills?per_page=50", "per_page=50"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.query, nil)
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status=%d want %d", tc.name, rr.Code, http.StatusOK)
		}
	}
}

func TestHandleBills_AuthenticatedAutoSort(t *testing.T) {
	srv, st, conn := newTestServerWithConn(t)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber, last_activity_date)
		VALUES ('b1', 45, 1, 'C-1', 'Health Bill', 'Health', '1st_reading', 'commons', '2026-01-01')`)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}
	_ = conn

	u, err := st.AuthenticateOAuth("google", "bills-auto-user", "billsauto@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/bills?sort=auto", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleSubscribeBill_MissingBillID(t *testing.T) {
	srv, st := newTestServer(t)

	u, err := st.AuthenticateOAuth("google", "sub-nobill", "subnobill@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	form := url.Values{}
	form.Set("bill_id", "")
	req := httptest.NewRequest(http.MethodPost, "/api/subscribe-bill", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleProfile_Unauthenticated(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	if !strings.HasSuffix(rr.Header().Get("Location"), "/auth/login") {
		t.Fatalf("expected redirect to /auth/login, got %s", rr.Header().Get("Location"))
	}
}

func TestHandleMemberProfile_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/members/nonexistent-member", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Either 404 or a rendered page with empty state is acceptable.
	if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 200 or 404", rr.Code)
	}
}

func TestHandleCompare_WithSelectedMembers(t *testing.T) {
	srv := newCompareServerWithTestMembers(t)

	// Request compare with both members explicitly selected.
	req := httptest.NewRequest(http.MethodGet, "/compare?a=f-lib&b=f-con", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleCompare_DefaultLevelIsFederal(t *testing.T) {
	srv := newCompareServerWithTestMembers(t)

	// No level param → defaults to "federal".
	req := httptest.NewRequest(http.MethodGet, "/compare", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestPublicURLForRequest_FallsBackToHostHeader(t *testing.T) {
	t.Setenv("OAUTH_BASE_URL", "")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "example.com"
	got := publicURLForRequest(req, "/test")
	if got != "http://example.com/test" {
		t.Errorf("publicURLForRequest = %q, want %q", got, "http://example.com/test")
	}
}

func TestPublicURLForRequest_UsesForwardedProto(t *testing.T) {
	t.Setenv("OAUTH_BASE_URL", "")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	got := publicURLForRequest(req, "/test")
	if got != "https://example.com/test" {
		t.Errorf("publicURLForRequest HTTPS = %q, want %q", got, "https://example.com/test")
	}
}

func TestHandleRiding_NoAddress(t *testing.T) {
	srv, _ := newTestServer(t)

	// GET /riding with no address param — should render page without lookup error.
	req := httptest.NewRequest(http.MethodGet, "/riding", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestHandleRiding_LookupFailsWithGenericError(t *testing.T) {
	srv, _ := newTestServer(t)

	// Fail geocoding with a generic error → default lookupErr message.
	srv.riding.SetLookups(
		func(_ context.Context, _ string, _ string) (float64, float64, error) {
			return 0, 0, fmt.Errorf("geocode service unavailable")
		},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/riding?address=unknown+address", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
}

func TestHandleRiding_LookupFailsRepresentatives(t *testing.T) {
	srv, _ := newTestServer(t)

	// Geocoding succeeds but representatives lookup fails with "representatives:" prefix.
	srv.riding.SetLookups(
		func(_ context.Context, _ string, _ string) (float64, float64, error) {
			return 45.0, -75.0, nil
		},
		func(_ context.Context, _, _ float64) ([]opennorth.Representative, error) {
			return nil, fmt.Errorf("representatives: API down")
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/riding?address=123+Main+St", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
}
