package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ---- helper: spin up a fake GitHub Issues API ----

func newFakeGitHubServer(t *testing.T, statusCode int, responseBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---- helper: create a verified session user ----

func verifiedSessionCookie(t *testing.T, srv *Server, email string) *http.Cookie {
	t.Helper()
	u, err := srv.store.AuthenticateOAuth("google", email, email, true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := srv.store.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return &http.Cookie{Name: "od_session", Value: sid}
}

// ---- GET /feedback ----

func TestHandleFeedback_GET_Unauthenticated_ShowsSignInPrompt(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/feedback", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Sign In") {
		t.Fatalf("expected sign-in prompt, got: %s", body)
	}
	if strings.Contains(body, `name="subject"`) {
		t.Fatalf("form should not be visible to unauthenticated users")
	}
}

func TestHandleFeedback_GET_UnverifiedUser_ShowsVerifyPrompt(t *testing.T) {
	srv, st := newTestServer(t)

	// Create a user without email verification.
	u, err := st.AuthenticateOAuth("google", "unverified@example.com", "unverified@example.com", false)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/feedback", nil)
	req.AddCookie(&http.Cookie{Name: "od_session", Value: sid})
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "verify your email") {
		t.Fatalf("expected email-verification prompt, got: %s", body)
	}
	if strings.Contains(body, `name="subject"`) {
		t.Fatalf("form should not be visible to unverified users")
	}
}

func TestHandleFeedback_GET_VerifiedUser_ShowsForm(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := verifiedSessionCookie(t, srv, "verified@example.com")

	req := httptest.NewRequest(http.MethodGet, "/feedback", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	for _, needle := range []string{
		`name="subject"`,
		`name="description"`,
		`name="category"`,
		`name="priority"`,
		`Submit Feedback`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in response, got: %s", needle, body)
		}
	}
}

func TestHandleFeedback_GET_Submitted_ShowsSuccessMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := verifiedSessionCookie(t, srv, "success@example.com")

	req := httptest.NewRequest(http.MethodGet, "/feedback?submitted=1&issue_url=https://github.com/philspins/opendocket/issues/99", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "submitted as a GitHub issue") {
		t.Fatalf("expected success message, got: %s", body)
	}
	if !strings.Contains(body, "View it here") {
		t.Fatalf("expected 'View it here' link, got: %s", body)
	}
	if !strings.Contains(body, "https://github.com/philspins/opendocket/issues/99") {
		t.Fatalf("expected issue URL in link, got: %s", body)
	}
}

// ---- POST /feedback ----

func TestHandleFeedback_POST_Unauthenticated_Returns401(t *testing.T) {
	srv, _ := newTestServer(t)

	form := url.Values{}
	form.Set("category", "bug")
	form.Set("priority", "low")
	form.Set("subject", "Test subject")
	form.Set("description", "Test description")

	req := httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleFeedback_POST_MissingFields_ShowsError(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := verifiedSessionCookie(t, srv, "missing@example.com")

	// Missing description.
	form := url.Values{}
	form.Set("category", "bug")
	form.Set("priority", "high")
	form.Set("subject", "Some subject")

	req := httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "Please fill in all fields") {
		t.Fatalf("expected validation error, got: %s", rr.Body.String())
	}
}

func TestHandleFeedback_POST_SubjectTooLong_ShowsError(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := verifiedSessionCookie(t, srv, "toolong@example.com")

	form := url.Values{}
	form.Set("category", "bug")
	form.Set("priority", "low")
	form.Set("subject", strings.Repeat("x", feedbackMaxSubjectLen+1))
	form.Set("description", "Valid description")

	req := httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "Subject must be") {
		t.Fatalf("expected subject-length error, got: %s", rr.Body.String())
	}
}

func TestHandleFeedback_POST_DescriptionTooLong_ShowsError(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := verifiedSessionCookie(t, srv, "desclong@example.com")

	form := url.Values{}
	form.Set("category", "bug")
	form.Set("priority", "low")
	form.Set("subject", "Valid subject")
	form.Set("description", strings.Repeat("y", feedbackMaxDescriptionLen+1))

	req := httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "Description must be") {
		t.Fatalf("expected description-length error, got: %s", rr.Body.String())
	}
}

func TestHandleFeedback_POST_NoToken_RedirectsToSuccess(t *testing.T) {
	t.Setenv("GITHUB_FEEDBACK_TOKEN", "")
	srv, _ := newTestServer(t)
	cookie := verifiedSessionCookie(t, srv, "notoken@example.com")

	form := url.Values{}
	form.Set("category", "bug")
	form.Set("priority", "low")
	form.Set("subject", "Test subject")
	form.Set("description", "Test description")

	req := httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusSeeOther)
	}
	if !strings.Contains(rr.Header().Get("Location"), "/feedback?submitted=1") {
		t.Fatalf("expected redirect to /feedback?submitted=1, got: %s", rr.Header().Get("Location"))
	}
}

func TestHandleFeedback_POST_WithGitHubToken_RedirectsWithIssueURL(t *testing.T) {
	issueURL := "https://github.com/philspins/opendocket/issues/42"
	fakeGH := newFakeGitHubServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":42}`)

	// Swap the package-level HTTP client used by createGitHubIssue.
	orig := http.DefaultClient
	http.DefaultClient = fakeGH.Client()
	// Point the API at the fake server.
	origAPI := githubIssuesAPI
	// We can't reassign the const directly; patch it via a test-only var alias.
	// Instead, replace the target URL by overriding DefaultTransport with a
	// redirect to the fake server.
	http.DefaultClient.Transport = &rewriteTransport{target: fakeGH.URL}
	t.Cleanup(func() {
		http.DefaultClient = orig
		_ = origAPI
	})

	t.Setenv("GITHUB_FEEDBACK_TOKEN", "test-token")
	srv, _ := newTestServer(t)
	cookie := verifiedSessionCookie(t, srv, "withtoken@example.com")

	form := url.Values{}
	form.Set("category", "bug")
	form.Set("priority", "high")
	form.Set("subject", "Great issue")
	form.Set("description", "Detailed description")

	req := httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d; body=%s", rr.Code, http.StatusSeeOther, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "submitted=1") {
		t.Fatalf("expected submitted=1 in redirect, got: %s", loc)
	}
	if !strings.Contains(loc, url.QueryEscape(issueURL)) {
		t.Fatalf("expected issue_url in redirect, got: %s", loc)
	}
}

func TestHandleFeedback_POST_GitHubAPIError_ShowsError(t *testing.T) {
	fakeGH := newFakeGitHubServer(t, http.StatusUnprocessableEntity, `{"message":"Validation Failed"}`)
	http.DefaultClient.Transport = &rewriteTransport{target: fakeGH.URL}
	t.Cleanup(func() { http.DefaultClient.Transport = nil })

	t.Setenv("GITHUB_FEEDBACK_TOKEN", "test-token")
	srv, _ := newTestServer(t)
	cookie := verifiedSessionCookie(t, srv, "apierr@example.com")

	form := url.Values{}
	form.Set("category", "bug")
	form.Set("priority", "low")
	form.Set("subject", "Test subject")
	form.Set("description", "Test description")

	req := httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "Failed to submit feedback") {
		t.Fatalf("expected error message, got: %s", rr.Body.String())
	}
}

// rewriteTransport redirects all requests to a target URL (fake server).
type rewriteTransport struct {
	target string
}

func (rt *rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme = "http"
	r2.URL.Host = strings.TrimPrefix(rt.target, "http://")
	return http.DefaultTransport.RoundTrip(r2)
}

// ---- unit tests for helper functions ----

func TestBuildIssueBody_ContainsAllFields(t *testing.T) {
	body := buildIssueBody("user@example.com", "Bug", "high", "Something is broken")
	for _, want := range []string{"user@example.com", "Bug", "high", "Something is broken", "### Description"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in body, got: %s", want, body)
		}
	}
}

func TestCategoryLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"bug", "Bug"},
		{"data", "Data"},
		{"ui", "UI / Design"},
		{"new_feature", "New Feature"},
		{"performance", "Performance"},
		{"other", "Other"},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		if got := categoryLabel(c.in); got != c.want {
			t.Errorf("categoryLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCategoryGitHubLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"bug", "bug"},
		{"data", "data"},
		{"new_feature", "feature"},
		{"ui", "ui"},
		{"performance", "performance"},
		{"other", "investigation"},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		if got := categoryGitHubLabel(c.in); got != c.want {
			t.Errorf("categoryGitHubLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCreateGitHubIssue_ParsesHTMLURL(t *testing.T) {
	issueURL := "https://github.com/philspins/opendocket/issues/7"
	fakeGH := newFakeGitHubServer(t, http.StatusCreated, `{"html_url":"`+issueURL+`","number":7}`)
	http.DefaultClient.Transport = &rewriteTransport{target: fakeGH.URL}
	t.Cleanup(func() { http.DefaultClient.Transport = nil })

	got, err := createGitHubIssue(context.Background(), "tok", "title", "body", []string{"bug"})
	if err != nil {
		t.Fatalf("createGitHubIssue: %v", err)
	}
	if got != issueURL {
		t.Errorf("html_url = %q, want %q", got, issueURL)
	}
}

func TestCreateGitHubIssue_NonCreatedStatusReturnsError(t *testing.T) {
	fakeGH := newFakeGitHubServer(t, http.StatusUnprocessableEntity, `{"message":"Validation Failed"}`)
	http.DefaultClient.Transport = &rewriteTransport{target: fakeGH.URL}
	t.Cleanup(func() { http.DefaultClient.Transport = nil })

	_, err := createGitHubIssue(context.Background(), "tok", "title", "body", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
