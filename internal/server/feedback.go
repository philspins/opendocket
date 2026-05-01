package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/templates"
)

const (
	feedbackRepo    = "philspins/opendocket"
	githubIssuesAPI = "https://api.github.com/repos/" + feedbackRepo + "/issues"
	githubAPIVersion = "2022-11-28"
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
		if r.URL.Query().Get("submitted") == "1" {
			successMsg = "Thank you! Your feedback has been submitted as a GitHub issue."
		}
		_ = templates.FeedbackPage(ps, userPtr, successMsg, "").Render(r.Context(), w)

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
		description := strings.TrimSpace(r.FormValue("description"))

		if category == "" || priority == "" || description == "" {
			_ = templates.FeedbackPage(ps, &u, "", "Please fill in all fields.").Render(r.Context(), w)
			return
		}

		token := strings.TrimSpace(os.Getenv("GITHUB_FEEDBACK_TOKEN"))
		if token == "" {
			log.Printf("feedback: GITHUB_FEEDBACK_TOKEN not configured; issue not created for user %s", u.Email)
			// Acknowledge silently so users are not blocked when the token is absent.
			http.Redirect(w, r, "/feedback?submitted=1", http.StatusSeeOther)
			return
		}

		title := fmt.Sprintf("[Feedback] %s — %s priority", categoryLabel(category), priority)
		body := buildIssueBody(u.Email, categoryLabel(category), priority, description)
		labels := []string{"feedback", category}

		if err := createGitHubIssue(r.Context(), token, title, body, labels); err != nil {
			log.Printf("feedback: GitHub issue creation failed for user %s: %v", u.Email, err)
			_ = templates.FeedbackPage(ps, &u, "", "Failed to submit feedback. Please try again later.").Render(r.Context(), w)
			return
		}

		http.Redirect(w, r, "/feedback?submitted=1", http.StatusSeeOther)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// createGitHubIssue opens a new issue in feedbackRepo via the GitHub REST API.
func createGitHubIssue(ctx context.Context, token, title, body string, labels []string) error {
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
		return fmt.Errorf("marshal issue payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubIssuesAPI, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GitHub API %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
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
