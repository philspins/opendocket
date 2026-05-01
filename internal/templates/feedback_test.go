package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/philspins/opendocket/internal/store"
)

func TestFeedbackPage_UnauthenticatedShowsSignInLinks(t *testing.T) {
	var buf bytes.Buffer
	err := FeedbackPage(store.ParliamentStatus{}, nil, "", "", "", 200, 5000).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "Sign In") {
		t.Errorf("expected Sign In link for unauthenticated user")
	}
	if !strings.Contains(html, "Sign Up") {
		t.Errorf("expected Sign Up link for unauthenticated user")
	}
	if strings.Contains(html, `name="subject"`) {
		t.Errorf("form should not be rendered for unauthenticated user")
	}
}

func TestFeedbackPage_UnverifiedUserShowsVerifyPrompt(t *testing.T) {
	user := &store.UserRow{ID: "u1", Email: "unverified@example.com", EmailVerified: false}
	var buf bytes.Buffer
	err := FeedbackPage(store.ParliamentStatus{}, user, "", "", "", 200, 5000).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "verify your email") {
		t.Errorf("expected verify-email prompt for unverified user")
	}
	if strings.Contains(html, `name="subject"`) {
		t.Errorf("form should not be rendered for unverified user")
	}
}

func TestFeedbackPage_VerifiedUserShowsForm(t *testing.T) {
	user := &store.UserRow{ID: "u2", Email: "verified@example.com", EmailVerified: true}
	var buf bytes.Buffer
	err := FeedbackPage(store.ParliamentStatus{}, user, "", "", "", 200, 5000).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	for _, needle := range []string{
		`name="subject"`,
		`name="description"`,
		`name="category"`,
		`name="priority"`,
		`Submit Feedback`,
		`maxlength="200"`,
		`maxlength="5000"`,
	} {
		if !strings.Contains(html, needle) {
			t.Errorf("expected %q in form for verified user", needle)
		}
	}
}

func TestFeedbackPage_SuccessMessageWithIssueLink(t *testing.T) {
	user := &store.UserRow{ID: "u3", Email: "ok@example.com", EmailVerified: true}
	issueURL := "https://github.com/philspins/opendocket/issues/5"

	var buf bytes.Buffer
	err := FeedbackPage(store.ParliamentStatus{}, user, "Thank you! Your feedback has been submitted as a GitHub issue.", issueURL, "", 200, 5000).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "submitted as a GitHub issue") {
		t.Errorf("expected success message")
	}
	if !strings.Contains(html, "View it here") {
		t.Errorf("expected 'View it here' link")
	}
	if !strings.Contains(html, issueURL) {
		t.Errorf("expected issue URL %q in output", issueURL)
	}
	if !strings.Contains(html, `target="_blank"`) {
		t.Errorf("expected link to open in new tab")
	}
	// Form should not appear on the success screen.
	if strings.Contains(html, `name="subject"`) {
		t.Errorf("form should not be visible after successful submission")
	}
}

func TestFeedbackPage_ErrorMessage(t *testing.T) {
	user := &store.UserRow{ID: "u4", Email: "err@example.com", EmailVerified: true}
	var buf bytes.Buffer
	err := FeedbackPage(store.ParliamentStatus{}, user, "", "", "Please fill in all fields.", 200, 5000).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "Please fill in all fields.") {
		t.Errorf("expected error message in output")
	}
	// Form should still be rendered so the user can correct their input.
	if !strings.Contains(html, `name="subject"`) {
		t.Errorf("form should still be visible when there is an error")
	}
}

func TestFeedbackPage_SuccessHidesForm(t *testing.T) {
	user := &store.UserRow{ID: "u5", Email: "done@example.com", EmailVerified: true}
	var buf bytes.Buffer
	err := FeedbackPage(store.ParliamentStatus{}, user, "Submitted!", "", "", 200, 5000).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, `name="subject"`) {
		t.Errorf("form should not appear after successful submission")
	}
}
