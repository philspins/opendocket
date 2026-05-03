package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/templates"
)

const (
	feedbackRepo              = "philspins/opendocket"
	githubIssuesAPI           = "https://api.github.com/repos/" + feedbackRepo + "/issues"
	githubAPIVersion          = "2022-11-28"
	feedbackMaxSubjectLen     = 200
	feedbackMaxDescriptionLen = 5000
)

func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	ps := s.parliamentStatus()

	switch r.Method {
	case http.MethodGet:
		var userPtr *store.UserRow
		if u, ok := s.auth.SessionUser(r); ok {
			userPtr = &u
		}
		successMsg := ""
		issueURL := ""
		if r.URL.Query().Get("submitted") == "1" {
			successMsg = "Thank you! Your feedback has been submitted as a GitHub issue."
			issueURL = r.URL.Query().Get("issue_url")
		}
		_ = templates.FeedbackPage(ps, userPtr, successMsg, issueURL, "", feedbackMaxSubjectLen, feedbackMaxDescriptionLen).Render(r.Context(), w)

	case http.MethodPost:
		u, ok := s.auth.RequireVerifiedSessionUser(w, r)
		if !ok {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		category := strings.TrimSpace(r.FormValue("category"))
		priority := strings.TrimSpace(r.FormValue("priority"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		description := strings.TrimSpace(r.FormValue("description"))

		if category == "" || priority == "" || subject == "" || description == "" {
			_ = templates.FeedbackPage(ps, &u, "", "", "Please fill in all fields.", feedbackMaxSubjectLen, feedbackMaxDescriptionLen).Render(r.Context(), w)
			return
		}
		if len(subject) > feedbackMaxSubjectLen {
			_ = templates.FeedbackPage(ps, &u, "", "", fmt.Sprintf("Subject must be %d characters or fewer.", feedbackMaxSubjectLen), feedbackMaxSubjectLen, feedbackMaxDescriptionLen).Render(r.Context(), w)
			return
		}
		if len(description) > feedbackMaxDescriptionLen {
			_ = templates.FeedbackPage(ps, &u, "", "", fmt.Sprintf("Description must be %d characters or fewer.", feedbackMaxDescriptionLen), feedbackMaxSubjectLen, feedbackMaxDescriptionLen).Render(r.Context(), w)
			return
		}

		token := strings.TrimSpace(os.Getenv("GITHUB_FEEDBACK_TOKEN"))
		if token == "" {
			clog.Infof("feedback: GITHUB_FEEDBACK_TOKEN not configured; issue not created for user %s", u.Email)
			// Acknowledge silently so users are not blocked when the token is absent.
			http.Redirect(w, r, "/feedback?submitted=1", http.StatusSeeOther)
			return
		}

		body := buildIssueBody(u.Email, categoryLabel(category), priority, description)
		labels := []string{"feedback", categoryGitHubLabel(category)}

		issueURL, err := createGitHubIssue(r.Context(), token, subject, body, labels)
		if err != nil {
			clog.Infof("feedback: GitHub issue creation failed for user %s: %v", u.Email, err)
			_ = templates.FeedbackPage(ps, &u, "", "", "Failed to submit feedback. Please try again later.", feedbackMaxSubjectLen, feedbackMaxDescriptionLen).Render(r.Context(), w)
			return
		}

		redirectURL := "/feedback?submitted=1"
		if issueURL != "" {
			redirectURL += "&issue_url=" + url.QueryEscape(issueURL)
		}
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// createGitHubIssue opens a new issue in feedbackRepo via the GitHub REST API.
// It returns the HTML URL of the created issue.
func createGitHubIssue(ctx context.Context, token, title, body string, labels []string) (string, error) {
	payload := struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Labels []string `json:"labels"`
	}{
		Title:  title,
		Body:   body,
		Labels: labels,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal issue payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubIssuesAPI, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("post issue: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("GitHub API %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	return result.HTMLURL, nil
}

// buildIssueBody formats the feedback form fields as a GitHub Markdown issue body.
func buildIssueBody(email, category, priority, description string) string {
	return fmt.Sprintf(
		"## Feedback\n\n"+
			"| Field | Value |\n"+
			"|---|---|\n"+
			"| **Submitted by** | %s |\n"+
			"| **Category** | %s |\n"+
			"| **Priority** | %s |\n\n"+
			"### Description\n\n%s",
		email, category, priority, description,
	)
}

func categoryLabel(v string) string {
	switch v {
	case "bug":
		return "Bug"
	case "data":
		return "Data"
	case "ui":
		return "UI / Design"
	case "new_feature":
		return "New Feature"
	case "performance":
		return "Performance"
	case "other":
		return "Other"
	default:
		return v
	}
}

// categoryGitHubLabel returns the GitHub label name for a given category value.
func categoryGitHubLabel(v string) string {
	switch v {
	case "bug":
		return "bug"
	case "data":
		return "data"
	case "new_feature":
		return "feature"
	case "ui":
		return "ui"
	case "performance":
		return "performance"
	case "other":
		return "investigation"
	default:
		return v
	}
}
