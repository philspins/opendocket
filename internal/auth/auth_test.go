package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/philspins/opendocket/internal/db"
	"github.com/philspins/opendocket/internal/store"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHandleSignupPage_RendersOAuthWidgetsAndFallbacks(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("FACEBOOK_CLIENT_ID", "fb-client")

	req := httptest.NewRequest(http.MethodGet, "/auth/signup", nil)
	rr := httptest.NewRecorder()

	svc.HandleSignupPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Create Your Account") {
		t.Fatalf("expected signup heading in page body")
	}
	if !strings.Contains(body, "google-signin-widget") {
		t.Fatalf("expected google widget container")
	}
	if !strings.Contains(body, "window.onGoogleLibraryLoad") {
		t.Fatalf("expected onGoogleLibraryLoad callback function")
	}
	if !strings.Contains(body, `src="https://accounts.google.com/gsi/client"`) {
		t.Fatalf("expected gsi/client loaded without custom onload param")
	}
	if !strings.Contains(body, "odFacebookWidgetLogin") {
		t.Fatalf("expected facebook sign-in button")
	}
	if !strings.Contains(body, "window.fbAsyncInit") {
		t.Fatalf("expected fbAsyncInit callback function")
	}
	if !strings.Contains(body, "/auth/google/login") || !strings.Contains(body, "/auth/facebook/login") {
		t.Fatalf("expected oauth fallback links")
	}
	if strings.Contains(body, "Email verification") {
		t.Fatalf("did not expect email verification box on signup page")
	}
}

func TestHandleSignupPage_RendersReCAPTCHAWidgetWhenConfigured(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("FACEBOOK_CLIENT_ID", "fb-client")
	t.Setenv("RECAPTCHA_SITE_KEY", "site-key")

	req := httptest.NewRequest(http.MethodGet, "/auth/signup", nil)
	rr := httptest.NewRecorder()

	svc.HandleSignupPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `recaptcha/api.js?onload=odSignupRecaptchaRun`) || !strings.Contains(body, `render=site-key`) {
		t.Fatalf("expected recaptcha onload callback and render key in same script URL")
	}
	if !strings.Contains(body, `id="signup-recaptcha-config" data-site-key="site-key"`) {
		t.Fatalf("expected recaptcha site key config on signup page")
	}
	if !strings.Contains(body, `id="signup-account-access" class="space-y-6 opacity-50 pointer-events-none select-none transition-opacity"`) {
		t.Fatalf("expected signup account section to be faded and disabled before recaptcha completion")
	}
	if !strings.Contains(body, `id="signup-recaptcha-gate" class="absolute inset-0 z-10 flex items-center justify-center"`) {
		t.Fatalf("expected recaptcha overlay gate on signup page")
	}
	if !strings.Contains(body, `window.grecaptcha.execute(siteKey, { action: "signup" })`) {
		t.Fatalf("expected recaptcha v3 execute call for signup action")
	}
	if !strings.Contains(body, `fetch("/auth/verify-recaptcha", {`) {
		t.Fatalf("expected recaptcha token verification request")
	}
	if !strings.Contains(body, `accountAccess.classList.remove("opacity-50", "pointer-events-none", "select-none")`) {
		t.Fatalf("expected recaptcha callback to enable account section")
	}
}

func TestHandleLoginPage_RendersLoginVariant(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("FACEBOOK_CLIENT_ID", "fb-client")

	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rr := httptest.NewRecorder()

	svc.HandleLoginPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Welcome Back") {
		t.Fatalf("expected login heading in page body")
	}
	if !strings.Contains(body, "Sign in with Google") {
		t.Fatalf("expected login-specific google button text")
	}
}

func TestHandleSignupPage_AuthenticatedUserRedirectsHome(t *testing.T) {
	svc, st := newTestService(t)
	u, err := st.UpsertUser("signed-in-signup@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sessionID, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/signup", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sessionID})
	rr := httptest.NewRecorder()

	svc.HandleSignupPage(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	if rr.Header().Get("Location") != "/" {
		t.Fatalf("location=%q want /", rr.Header().Get("Location"))
	}
}

func TestHandleLoginPage_AuthenticatedUserRedirectsHome(t *testing.T) {
	svc, st := newTestService(t)
	u, err := st.UpsertUser("signed-in-login@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sessionID, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sessionID})
	rr := httptest.NewRecorder()

	svc.HandleLoginPage(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	if rr.Header().Get("Location") != "/" {
		t.Fatalf("location=%q want /", rr.Header().Get("Location"))
	}
}

func newTestService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	t.Setenv("SES_FROM_EMAIL", "")
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	st := store.New(conn)
	svc := New(st, "http://127.0.0.1:8080")
	return svc, st
}

func TestClientIP_TrustProxyFalse_IgnoresForwardedHeaders(t *testing.T) {
	svc, _ := newTestService(t)
	// trustProxy defaults to false; forwarded headers must be ignored.

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")

	got := svc.clientIP(req)
	if got != "203.0.113.5" {
		t.Errorf("clientIP with trustProxy=false: got %q, want %q (203.0.113.5)", got, "203.0.113.5")
	}
}

func TestClientIP_TrustProxyTrue_ReadsXForwardedFor(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetTrustProxy(true)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9000" // internal proxy IP
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")

	got := svc.clientIP(req)
	if got != "1.2.3.4" {
		t.Errorf("clientIP with trustProxy=true: got %q, want %q (1.2.3.4)", got, "1.2.3.4")
	}
}

func TestClientIP_TrustProxyTrue_FallsBackToXRealIP(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetTrustProxy(true)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9000"
	req.Header.Set("X-Real-IP", "9.8.7.6")

	got := svc.clientIP(req)
	if got != "9.8.7.6" {
		t.Errorf("clientIP with X-Real-IP: got %q, want %q (9.8.7.6)", got, "9.8.7.6")
	}
}

func TestClientIP_TrustProxyTrue_FallsBackToRemoteAddr(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetTrustProxy(true)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.99:54321"

	got := svc.clientIP(req)
	if got != "192.0.2.99" {
		t.Errorf("clientIP fallback to RemoteAddr: got %q, want %q (192.0.2.99)", got, "192.0.2.99")
	}
}

func TestHandleGoogleLogin_SetsStateCookieAndRedirect(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")

	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	rr := httptest.NewRecorder()

	svc.HandleGoogleLogin(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusFound)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "accounts.google.com") {
		t.Fatalf("unexpected redirect location: %s", loc)
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	state := u.Query().Get("state")
	if state == "" {
		t.Fatalf("missing state in redirect")
	}

	cookies := rr.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "od_oauth_state" && c.Value == state {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected od_oauth_state cookie matching redirect state")
	}
}

func TestHandleGoogleLogin_UsesOAuthCallbackOverride(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("OAUTH_CALLBACK_URL", "https://opendocket.ca/auth/google/callback")

	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	rr := httptest.NewRecorder()

	svc.HandleGoogleLogin(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusFound)
	}
	loc := rr.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	if got, want := u.Query().Get("redirect_uri"), "https://opendocket.ca/auth/google/callback"; got != want {
		t.Fatalf("redirect_uri=%q want %q", got, want)
	}
}

func TestHandleLogout_DeletesSessionAndClearsCookie(t *testing.T) {
	svc, st := newTestService(t)
	u, err := st.UpsertUser("logout@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sessionID, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sessionID})
	rr := httptest.NewRecorder()

	svc.HandleLogout(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	_, err = st.GetUserBySession(sessionID)
	if err == nil {
		t.Fatalf("expected session to be deleted")
	}
	cleared := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "od_session" && c.MaxAge == -1 {
			cleared = true
			break
		}
	}
	if !cleared {
		t.Fatalf("expected od_session clearing cookie")
	}
}

func TestHandleVerifyEmail_ByCode_SetsSession(t *testing.T) {
	svc, st := newTestService(t)
	email := "verify-code-auth@example.com"
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

	svc.HandleVerifyEmail(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "od_session" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected od_session cookie to be set")
	}
}

func TestHandleRequestVerification_RequiresValidReCAPTCHAWhenConfigured(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("RECAPTCHA_SECRET_KEY", "secret")

	form := url.Values{}
	form.Set("email", "captcha-required@example.com")
	req := httptest.NewRequest(http.MethodPost, "/auth/request-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	svc.HandleRequestVerification(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing captcha token status=%d want %d", rr.Code, http.StatusBadRequest)
	}

	var sentBody string
	svc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			b, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("ReadAll: %v", err)
			}
			sentBody = string(b)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	})

	form.Set("g-recaptcha-response", "test-token")
	req = httptest.NewRequest(http.MethodPost, "/auth/request-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	svc.HandleRequestVerification(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(sentBody, "secret=secret") || !strings.Contains(sentBody, "response=test-token") {
		t.Fatalf("unexpected recaptcha verification request body: %s", sentBody)
	}
}

func TestHandleVerifyRecaptcha_RequiresValidTokenWhenConfigured(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("RECAPTCHA_SECRET_KEY", "secret")

	req := httptest.NewRequest(http.MethodPost, "/auth/verify-recaptcha", nil)
	rr := httptest.NewRecorder()
	svc.HandleVerifyRecaptcha(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing captcha token status=%d want %d", rr.Code, http.StatusBadRequest)
	}

	var sentBody string
	svc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			b, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("ReadAll: %v", err)
			}
			sentBody = string(b)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	})

	form := url.Values{}
	form.Set("g-recaptcha-response", "signup-token")
	req = httptest.NewRequest(http.MethodPost, "/auth/verify-recaptcha", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	svc.HandleVerifyRecaptcha(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(sentBody, "secret=secret") || !strings.Contains(sentBody, "response=signup-token") {
		t.Fatalf("unexpected recaptcha verification request body: %s", sentBody)
	}
}

// ── RequireVerifiedSessionUser ────────────────────────────────────────────────

func TestRequireVerifiedSessionUser_NoSession(t *testing.T) {
	svc, _ := newTestService(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	_, ok := svc.RequireVerifiedSessionUser(rr, req)

	if ok {
		t.Fatal("expected RequireVerifiedSessionUser to fail with no session")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rr.Body.String(), "authentication_required") {
		t.Fatalf("expected authentication_required in body, got: %s", rr.Body.String())
	}
}

func TestRequireVerifiedSessionUser_UnverifiedEmail(t *testing.T) {
	svc, st := newTestService(t)
	u, err := st.UpsertUser("unverified@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	_, ok := svc.RequireVerifiedSessionUser(rr, req)

	if ok {
		t.Fatal("expected RequireVerifiedSessionUser to fail for unverified user")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rr.Body.String(), "email_verification_required") {
		t.Fatalf("expected email_verification_required in body, got: %s", rr.Body.String())
	}
}

func TestRequireVerifiedSessionUser_VerifiedUser(t *testing.T) {
	svc, st := newTestService(t)
	u, err := st.AuthenticateOAuth("google", "verified-sub", "verified@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	got, ok := svc.RequireVerifiedSessionUser(rr, req)

	if !ok {
		t.Fatal("expected RequireVerifiedSessionUser to succeed for verified user")
	}
	if got.ID != u.ID {
		t.Fatalf("user ID mismatch: got %q want %q", got.ID, u.ID)
	}
}

// ── HandleWhoAmI ─────────────────────────────────────────────────────────────

func TestHandleWhoAmI_Unauthenticated(t *testing.T) {
	svc, _ := newTestService(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rr := httptest.NewRecorder()

	svc.HandleWhoAmI(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleWhoAmI_Authenticated(t *testing.T) {
	svc, st := newTestService(t)
	u, err := st.UpsertUser("whoami@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	svc.HandleWhoAmI(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	var decoded store.UserRow
	if err := json.NewDecoder(rr.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decoded.ID != u.ID {
		t.Fatalf("user ID mismatch: got %q want %q", decoded.ID, u.ID)
	}
}

// ── readOAuthState ────────────────────────────────────────────────────────────

func TestReadOAuthState_Valid(t *testing.T) {
	svc, _ := newTestService(t)
	state := "test-state-abc123"
	req := httptest.NewRequest(http.MethodGet, "/?state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "od_oauth_state", Value: state})

	if !svc.readOAuthState(req) {
		t.Fatal("expected readOAuthState to return true for matching state")
	}
}

func TestReadOAuthState_NoCookie(t *testing.T) {
	svc, _ := newTestService(t)
	req := httptest.NewRequest(http.MethodGet, "/?state=abc", nil)

	if svc.readOAuthState(req) {
		t.Fatal("expected readOAuthState to return false with no cookie")
	}
}

func TestReadOAuthState_Mismatch(t *testing.T) {
	svc, _ := newTestService(t)
	req := httptest.NewRequest(http.MethodGet, "/?state=state-abc", nil)
	req.AddCookie(&http.Cookie{Name: "od_oauth_state", Value: "state-xyz"})

	if svc.readOAuthState(req) {
		t.Fatal("expected readOAuthState to return false for mismatched state")
	}
}

// ── exchangeCode ─────────────────────────────────────────────────────────────

func TestExchangeCode_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"tok-123"}`)
	}))
	defer ts.Close()

	body, err := exchangeCode(&http.Client{}, ts.URL, url.Values{"code": {"abc"}})
	if err != nil {
		t.Fatalf("exchangeCode: %v", err)
	}
	if !strings.Contains(string(body), "tok-123") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestExchangeCode_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad credentials", http.StatusBadRequest)
	}))
	defer ts.Close()

	_, err := exchangeCode(&http.Client{}, ts.URL, url.Values{"code": {"bad"}})
	if err == nil {
		t.Fatal("expected error for 4xx response")
	}
}

// ── HandleGoogleCallback ──────────────────────────────────────────────────────

func TestHandleGoogleCallback_InvalidState(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=bad&code=abc", nil)
	rr := httptest.NewRecorder()

	svc.HandleGoogleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleGoogleCallback_HappyPath(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "google-secret")

	// Single test server handles both token exchange and userinfo.
	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = fmt.Fprint(w, `{"access_token":"test-access-tok"}`)
		default:
			_, _ = fmt.Fprint(w, `{"sub":"google-sub-1","email":"google@example.com","email_verified":true}`)
		}
	}))
	defer oauthServer.Close()

	// Redirect all outbound calls to the mock server.
	svc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Host = strings.TrimPrefix(oauthServer.URL, "http://")
			req.URL.Scheme = "http"
			return http.DefaultTransport.RoundTrip(req)
		}),
	})

	state := "test-google-state"
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state="+state+"&code=test-code", nil)
	req.AddCookie(&http.Cookie{Name: "od_oauth_state", Value: state})
	rr := httptest.NewRecorder()

	svc.HandleGoogleCallback(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusSeeOther, rr.Body.String())
	}
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "od_session" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected od_session cookie after google oauth callback")
	}
}

// ── HandleFacebookCallback ────────────────────────────────────────────────────

func TestHandleFacebookCallback_InvalidState(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("FACEBOOK_CLIENT_ID", "fb-client")

	req := httptest.NewRequest(http.MethodGet, "/auth/facebook/callback?state=bad&code=abc", nil)
	rr := httptest.NewRecorder()

	svc.HandleFacebookCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleFacebookCallback_HappyPath(t *testing.T) {
	svc, _ := newTestService(t)
	t.Setenv("FACEBOOK_CLIENT_ID", "fb-client")
	t.Setenv("FACEBOOK_CLIENT_SECRET", "fb-secret")

	// Single test server handles both token exchange and userinfo.
	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v19.0/oauth/access_token":
			_, _ = fmt.Fprint(w, `{"access_token":"fb-test-tok"}`)
		default:
			_, _ = fmt.Fprint(w, `{"id":"fb-user-1","email":"fb@example.com"}`)
		}
	}))
	defer oauthServer.Close()

	// Redirect all outbound calls to the mock server.
	svc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Host = strings.TrimPrefix(oauthServer.URL, "http://")
			req.URL.Scheme = "http"
			return http.DefaultTransport.RoundTrip(req)
		}),
	})

	state := "test-fb-state"
	req := httptest.NewRequest(http.MethodGet, "/auth/facebook/callback?state="+state+"&code=test-code", nil)
	req.AddCookie(&http.Cookie{Name: "od_oauth_state", Value: state})
	rr := httptest.NewRecorder()

	svc.HandleFacebookCallback(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusSeeOther, rr.Body.String())
	}
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "od_session" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected od_session cookie after facebook oauth callback")
	}
}

// ── RegisterRoutes ────────────────────────────────────────────────────────────

func TestRegisterRoutes_RegistersAllExpectedRoutes(t *testing.T) {
	svc, _ := newTestService(t)
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	for _, path := range []string{
		"GET /auth/signup",
		"GET /auth/login",
		"GET /auth/me",
		"GET /auth/google/login",
		"GET /auth/google/callback",
		"GET /auth/facebook/login",
		"GET /auth/facebook/callback",
	} {
		parts := strings.SplitN(path, " ", 2)
		req := httptest.NewRequest(parts[0], parts[1], nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code == http.StatusNotFound {
			t.Errorf("route %s returned 404 — not registered", path)
		}
	}
}

// ── SendVerificationEmail via mock emailer ────────────────────────────────────

type mockEmailer struct {
	lastEmail string
	lastURL   string
	lastCode  string
}

func (m *mockEmailer) SendVerificationEmail(_ context.Context, toEmail, verifyURL, code string) error {
	m.lastEmail = toEmail
	m.lastURL = verifyURL
	m.lastCode = code
	return nil
}

func TestHandleRequestVerification_CallsEmailerWhenConfigured(t *testing.T) {
	svc, _ := newTestService(t)
	mock := &mockEmailer{}
	svc.emailer = mock

	form := url.Values{}
	form.Set("email", "emailer@example.com")
	req := httptest.NewRequest(http.MethodPost, "/auth/request-verification", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	svc.HandleRequestVerification(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	if mock.lastEmail != "emailer@example.com" {
		t.Fatalf("emailer.lastEmail=%q want %q", mock.lastEmail, "emailer@example.com")
	}
	if !strings.Contains(mock.lastURL, "/auth/verify") {
		t.Fatalf("emailer.lastURL=%q should contain /auth/verify", mock.lastURL)
	}
	if mock.lastCode == "" {
		t.Fatal("emailer.lastCode should not be empty")
	}
}

func TestHandleRequestVerification_UsesSesEmailFromSession(t *testing.T) {
	svc, st := newTestService(t)
	u, err := st.UpsertUser("session-verify@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// POST with no email — should fall back to session user's email.
	req := httptest.NewRequest(http.MethodPost, "/auth/request-verification", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()

	svc.HandleRequestVerification(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
}

// ── HandleVerifyEmail via JSON ────────────────────────────────────────────────

func TestHandleVerifyEmail_ByTokenJSON(t *testing.T) {
	svc, st := newTestService(t)
	email := "json-token@example.com"
	token, _, err := st.CreateEmailVerification(email, time.Hour)
	if err != nil {
		t.Fatalf("CreateEmailVerification: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"token": token})
	req := httptest.NewRequest(http.MethodPost, "/auth/verify", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	svc.HandleVerifyEmail(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusSeeOther, rr.Body.String())
	}
}
