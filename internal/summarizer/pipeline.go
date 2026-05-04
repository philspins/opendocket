// Package summarizer handles AI-powered bill summarization using Claude API.
package summarizer

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
	"golang.org/x/net/html"
)

// ErrBillTextNotFound is returned by fetchBillText when the remote server
// responds with HTTP 404.  The bill text simply doesn't exist at the stored
// URL (e.g. pro-forma Senate bills like S-1), so the caller should clear
// full_text_url in the database to avoid retrying on every crawl run.
var ErrBillTextNotFound = errors.New("bill text not found (HTTP 404)")

const (
	claudeModel              = "claude-sonnet-4-6"
	claudeURL                = "https://api.anthropic.com/v1/messages"
	maxBillTextResponseBytes = 8 * 1024 * 1024
	defaultAnthropicRPM      = 15
	claudeMaxAttempts        = 5
)

var (
	anthropicMessagesURL = claudeURL
	claudeInitialBackoff = 5 * time.Second
	peiWDFWorkflowURL    = "https://wdf.princeedwardisland.ca/legislative-assembly/services/api/workflow"
	claudeHTTPClient     = func() *http.Client {
		client := utils.NewHTTPClient()
		client.Timeout = 60 * time.Second
		return client
	}
	claudeRateLimiter = newTokenBucket(summarizerEnvInt("ANTHROPIC_REQUESTS_PER_MINUTE", defaultAnthropicRPM))
)

type tokenBucket struct {
	tokens chan struct{}
}

func newTokenBucket(rate int) *tokenBucket {
	if rate <= 0 {
		return nil
	}
	bucket := &tokenBucket{tokens: make(chan struct{}, rate)}
	for i := 0; i < rate; i++ {
		bucket.tokens <- struct{}{}
	}
	interval := time.Minute / time.Duration(rate)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			select {
			case bucket.tokens <- struct{}{}:
			default:
			}
		}
	}()
	return bucket
}

func (bucket *tokenBucket) wait(ctx context.Context) error {
	if bucket == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-bucket.tokens:
		return nil
	}
}

func summarizerEnvInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

var pdfParenTextRe = regexp.MustCompile(`\(([^()]*)\)`)

func decodePDFStringToken(token string) string {
	token = strings.ReplaceAll(token, `\\(`, "(")
	token = strings.ReplaceAll(token, `\\)`, ")")
	token = strings.ReplaceAll(token, `\\n`, " ")
	token = strings.ReplaceAll(token, `\\r`, " ")
	token = strings.ReplaceAll(token, `\\t`, " ")
	token = strings.ReplaceAll(token, `\\`, "")
	return token
}

func extractPDFText(data []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", "od-bill-*.pdf")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	contentDir, err := os.MkdirTemp("", "od-bill-content-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(contentDir)

	if err := api.ExtractContentFile(tmpPath, contentDir, nil, nil); err != nil {
		return "", err
	}

	files, err := filepath.Glob(filepath.Join(contentDir, "*_Content_page_*.txt"))
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	var text strings.Builder
	for _, contentPath := range files {
		fp, err := os.Open(contentPath)
		if err != nil {
			return "", err
		}
		scanner := bufio.NewScanner(fp)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasSuffix(line, "TJ") || strings.HasSuffix(line, "Tj") {
				for _, match := range pdfParenTextRe.FindAllStringSubmatch(line, -1) {
					if len(match) >= 2 {
						text.WriteString(decodePDFStringToken(match[1]))
					}
				}
				text.WriteByte(' ')
			}
		}
		if err := scanner.Err(); err != nil {
			fp.Close()
			return "", err
		}
		fp.Close()
		text.WriteByte('\f')
	}

	return strings.TrimSpace(collapseWhitespace(text.String())), nil
}

func selectedClaudeModels() []string {
	models := make([]string, 0, 5)
	seen := map[string]struct{}{}
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m == "" {
			return
		}
		if _, ok := seen[m]; ok {
			return
		}
		seen[m] = struct{}{}
		models = append(models, m)
	}

	// User override first.
	add(os.Getenv("ANTHROPIC_MODEL"))

	// Fallback candidates to tolerate model deprecations/availability changes.
	add(claudeModel)

	return models
}

func isModelNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not_found_error") || strings.Contains(msg, "model:")
}

func parseSummaryJSON(raw string) (SummaryResult, error) {
	text := strings.TrimSpace(raw)

	// Claude may occasionally wrap JSON in fenced blocks despite instructions.
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		if nl := strings.Index(text, "\n"); nl >= 0 {
			text = text[nl+1:]
		}
		if end := strings.LastIndex(text, "```"); end >= 0 {
			text = text[:end]
		}
		text = strings.TrimSpace(text)
	}

	var result SummaryResult
	if err := json.Unmarshal([]byte(text), &result); err == nil {
		return result, nil
	}

	// Fallback: extract the first JSON object from mixed/plaintext output.
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		candidate := text[start : end+1]
		if err := json.Unmarshal([]byte(candidate), &result); err == nil {
			return result, nil
		}
	}

	return SummaryResult{}, fmt.Errorf("invalid summary payload")
}

func parseRetryAfter(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryTime, err := http.ParseTime(header); err == nil {
		delay := retryTime.Sub(now)
		if delay > 0 {
			return delay
		}
	}
	return 0
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func summarizerParallelism() int {
	v := strings.TrimSpace(os.Getenv("SUMMARIZER_PARALLELISM"))
	if v == "" {
		return 1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

var systemPrompt = `You are a non-partisan Canadian civic education assistant.
Your job is to summarize bills from the Parliament of Canada in plain English.
You must be accurate, neutral, and clear. Never editorialize or express opinions.
Always write so a 13-year-old could follow it. Use short, plain sentences and avoid legal jargon.

In addition to the main summary, identify any notable considerations: exceptions,
side effects, carve-outs, enforcement details, trade-offs, or unrelated clauses that
may not be obvious at first read. Describe these neutrally and factually. If there are
no notable considerations, say so clearly.

Provide your response as valid JSON only (no markdown or extra text):
{
  "one_sentence": "One sentence (max 25 words) describing what this bill does.",
	"plain_summary": "2 short paragraphs in plain English. Explain what the bill does, who it affects, and why it was introduced.",
  "key_changes": ["List of 3–6 specific things this bill would change or create"],
  "who_is_affected": ["List of groups, industries, or people most affected"],
  "notable_considerations": ["List of 0–5 potential caveats, non-obvious trade-offs, or implementation considerations in neutral language"],
  "category": "One of: Budget, Criminal Justice, Environment, Health, Housing, Immigration, Indigenous, Infrastructure, Justice, Labour, National Security, Social Policy, Trade, Veterans"
}`

// SummaryResult holds the structured fields returned by Claude.
type SummaryResult struct {
	OneSentence           string   `json:"one_sentence"`
	PlainSummary          string   `json:"plain_summary"`
	KeyChanges            []string `json:"key_changes"`
	WhoIsAffected         []string `json:"who_is_affected"`
	NotableConsiderations []string `json:"notable_considerations"`
	Category              string   `json:"category"`
	BillID                string   `json:"bill_id"`
	GeneratedAt           string   `json:"generated_at"`
	Model                 string   `json:"model"`
}

// claudeRequest is the request body structure for Claude API.
type claudeRequest struct {
	Model       string      `json:"model"`
	MaxTokens   int         `json:"max_tokens"`
	System      string      `json:"system"`
	Messages    []claudeMsg `json:"messages"`
	Temperature float64     `json:"temperature,omitempty"`
}

type claudeMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse is the response from Claude API.
type claudeResponse struct {
	ID      string `json:"id"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// BillSummaryRequest carries the minimum metadata needed to summarize a bill.
type BillSummaryRequest struct {
	BillID           string
	BillTitle        string
	FullTextURL      string
	LastActivityDate string
}

type peWDFNode struct {
	Type     string          `json:"type"`
	Data     json.RawMessage `json:"data"`
	Children []peWDFNode     `json:"children"`
}

type peWDFTreeResponse struct {
	Data []peWDFNode `json:"data"`
}

type peWDFCellData struct {
	Text *string `json:"text"`
}

type peWDFLinkData struct {
	Href        *string           `json:"href"`
	QueryParams map[string]string `json:"queryParams"`
}

var peBillIDPartsRe = regexp.MustCompile(`(?i)^pe-(\d+)-(\d+)-([a-z0-9-]+)$`)

// shouldSummarizeBill returns true when a bill needs a fresh AI summary.
// Rules:
//  1. If a Library of Parliament summary exists, skip AI summarization.
//  2. If no previous AI summary exists, summarize.
//  3. If an AI summary exists, summarize only when bill last activity is newer
//     than the AI summary generation timestamp.
func shouldSummarizeBill(ctx context.Context, db *sql.DB, billID, incomingLastActivityDate string) (bool, error) {
	if db == nil {
		return true, nil
	}

	var (
		summaryAI        string
		lastActivityDate string
	)
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(summary_ai,''), COALESCE(last_activity_date,'')
		 FROM bills WHERE id = ?`,
		billID,
	).Scan(&summaryAI, &lastActivityDate)
	if err != nil {
		return true, fmt.Errorf("lookup bill %q: %w", billID, err)
	}

	if strings.TrimSpace(summaryAI) == "" {
		return true, nil
	}

	var previous SummaryResult
	if err := json.Unmarshal([]byte(summaryAI), &previous); err != nil {
		// If legacy/invalid JSON exists, regenerate a clean summary.
		return true, nil
	}
	if strings.TrimSpace(previous.GeneratedAt) == "" {
		return true, nil
	}
	generatedAt, err := time.Parse(time.RFC3339, previous.GeneratedAt)
	if err != nil {
		return true, nil
	}

	activity := strings.TrimSpace(incomingLastActivityDate)
	if activity == "" {
		activity = strings.TrimSpace(lastActivityDate)
	}
	if activity == "" {
		// No reliable activity timestamp; keep previous summary.
		return false, nil
	}

	billLastUpdated, err := time.Parse("2006-01-02", activity)
	if err != nil {
		return true, nil
	}

	return billLastUpdated.After(generatedAt.UTC()), nil
}

// SummarizeBill calls Claude API and returns a structured summary.
// It truncates very long bills (keeping first ~120k chars + last 30k chars).
func SummarizeBill(ctx context.Context, db *sql.DB, billID, billTitle, billText, lastActivityDate string) (*SummaryResult, error) {
	shouldSummarize, err := shouldSummarizeBill(ctx, db, billID, lastActivityDate)
	if err != nil {
		return nil, err
	}
	if !shouldSummarize {
		return nil, nil
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	// Truncate very long bills — keep first ~120k + last 30k characters (rune-safe).
	const maxRunes = 150_000
	if utf8.RuneCountInString(billText) > maxRunes {
		runes := []rune(billText)
		billText = string(runes[:120_000]) + "\n\n[...truncated...]\n\n" + string(runes[len(runes)-30_000:])
	}

	prompt := fmt.Sprintf(`Please summarize the following Canadian bill:

Bill ID: %s
Title: %s

Full text:
%s

Respond with only valid JSON, no markdown or extra text.`, billID, billTitle, billText)

	models := selectedClaudeModels()
	if len(models) == 0 {
		return nil, fmt.Errorf("no anthropic model configured")
	}
	model := models[0]
	req := claudeRequest{
		Model:       model,
		MaxTokens:   2048,
		System:      systemPrompt,
		Temperature: 0.3,
		Messages: []claudeMsg{
			{Role: "user", Content: prompt},
		},
	}

	callClaude := func(model string) (*claudeResponse, error) {
		req.Model = model
		return callClaudeAPI(ctx, apiKey, req)
	}

	var (
		apiResp *claudeResponse
		lastErr error
	)
	for i, candidate := range models {
		apiResp, lastErr = callClaude(candidate)
		if lastErr == nil {
			model = candidate
			break
		}
		if !isModelNotFoundError(lastErr) {
			return nil, lastErr
		}
		if i < len(models)-1 {
			clog.Infof("[summarizer] model %q unavailable; retrying with %q", candidate, models[i+1])
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("all configured models unavailable (%s): %w", strings.Join(models, ", "), lastErr)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("api error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from api")
	}

	// Parse the JSON response from Claude.
	result, err := parseSummaryJSON(apiResp.Content[0].Text)
	if err != nil {
		return nil, fmt.Errorf("parse summary JSON: %w", err)
	}

	result.BillID = billID
	result.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	result.Model = model

	return &result, nil
}

func callClaudeAPI(ctx context.Context, apiKey string, req claudeRequest) (*claudeResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	client := claudeHTTPClient()
	backoff := claudeInitialBackoff
	for attempt := 1; attempt <= claudeMaxAttempts; attempt++ {
		if err := claudeRateLimiter.wait(ctx); err != nil {
			return nil, fmt.Errorf("wait for rate limiter: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicMessagesURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("x-api-key", apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
		httpReq.Header.Set("content-type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			if attempt == claudeMaxAttempts {
				return nil, fmt.Errorf("api call attempt %d: %w", attempt, err)
			}
			if waitErr := waitForRetry(ctx, backoff); waitErr != nil {
				return nil, waitErr
			}
			backoff *= 2
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		if resp.StatusCode == http.StatusOK {
			var apiResp claudeResponse
			if err := json.Unmarshal(respBody, &apiResp); err != nil {
				return nil, fmt.Errorf("unmarshal response: %w", err)
			}
			return &apiResp, nil
		}

		var apiErr claudeResponse
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Error != nil {
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
				delay := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now().UTC())
				if delay <= 0 {
					delay = backoff
				}
				if resp.StatusCode == http.StatusTooManyRequests {
					clog.Infof("[summarizer] claude rate limited; retrying in %s (attempt %d/%d)", delay, attempt, claudeMaxAttempts)
				}
				if attempt == claudeMaxAttempts {
					return nil, fmt.Errorf("api returned %d (%s): %s", resp.StatusCode, apiErr.Error.Type, apiErr.Error.Message)
				}
				if waitErr := waitForRetry(ctx, delay); waitErr != nil {
					return nil, waitErr
				}
				backoff *= 2
				continue
			}
			return nil, fmt.Errorf("api returned %d (%s): %s", resp.StatusCode, apiErr.Error.Type, apiErr.Error.Message)
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			delay := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now().UTC())
			if delay <= 0 {
				delay = backoff
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				clog.Infof("[summarizer] claude rate limited; retrying in %s (attempt %d/%d)", delay, attempt, claudeMaxAttempts)
			}
			if attempt == claudeMaxAttempts {
				return nil, fmt.Errorf("api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
			}
			if waitErr := waitForRetry(ctx, delay); waitErr != nil {
				return nil, waitErr
			}
			backoff *= 2
			continue
		}

		return nil, fmt.Errorf("api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil, fmt.Errorf("claude api retry loop exhausted")
}

// SummarizeBillsFromChannel reads bill summary requests from a channel and
// pipes each request into SummarizeBill.
func SummarizeBillsFromChannel(ctx context.Context, db *sql.DB, requests <-chan BillSummaryRequest) (int, error) {
	workers := summarizerParallelism()
	if workers > 1 {
		clog.Infof("[summarizer] parallel workers: %d", workers)
	}

	var processed int64
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for req := range requests {
			if ctx.Err() != nil {
				return
			}
			if strings.TrimSpace(req.BillID) == "" || strings.TrimSpace(req.FullTextURL) == "" {
				continue
			}

			// Fetch the bill text first: this validates the URL and lets us
			// clear full_text_url immediately on 404 regardless of whether
			// an API key is configured.
			billText, err := fetchBillText(ctx, req.BillID, req.FullTextURL)
			if err != nil {
				if errors.Is(err, ErrBillTextNotFound) {
					// The bill text doesn't exist at the stored URL (e.g. pro-forma
					// Senate bills like S-1).  Clear full_text_url so we don't
					// retry on every future crawl run.
					clog.Infof("[summarizer] bill text unavailable (404) for %q; clearing full_text_url", req.BillID)
					db.ExecContext(ctx, `UPDATE bills SET full_text_url = '' WHERE id = ?`, req.BillID)
				} else {
					clog.Infof("[summarizer] fetch bill text %q: %v", req.BillID, err)
				}
				continue
			}

			// Gate summarization on the API key and staleness check.
			if os.Getenv("ANTHROPIC_API_KEY") == "" {
				clog.Infof("[summarizer] ANTHROPIC_API_KEY not set; skipping %q", req.BillID)
				continue
			}
			needed, err := shouldSummarizeBill(ctx, db, req.BillID, req.LastActivityDate)
			if err != nil {
				clog.Infof("[summarizer] check bill %q: %v", req.BillID, err)
				continue
			}
			if !needed {
				clog.Debugf("[summarizer] skip unchanged bill %q", req.BillID)
				continue
			}

			clog.Infof("[summarizer] summarizing bill %q (%s)...", req.BillID, req.BillTitle)
			summary, err := SummarizeBill(ctx, db, req.BillID, req.BillTitle, billText, req.LastActivityDate)
			if err != nil {
				clog.Infof("[summarizer] summarize error %q: %v", req.BillID, err)
				continue
			}
			if summary == nil {
				clog.Debugf("[summarizer] skip unchanged bill %q", req.BillID)
				continue
			}

			summaryJSON, _ := json.Marshal(summary)
			_, err = db.ExecContext(ctx,
				`UPDATE bills SET summary_ai = ?, category = ? WHERE id = ?`,
				string(summaryJSON), summary.Category, req.BillID)
			if err != nil {
				clog.Infof("[summarizer] store summary %q: %v", req.BillID, err)
				continue
			}

			atomic.AddInt64(&processed, 1)
			clog.Infof("[summarizer] ✓ stored summary for %q", req.BillID)

			select {
			case <-time.After(1 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}

	wg.Wait()
	if ctx.Err() != nil {
		return int(atomic.LoadInt64(&processed)), ctx.Err()
	}
	return int(atomic.LoadInt64(&processed)), nil
}

// SummarizeNewBills processes all bills that still lack AI summaries.
// This is meant to be called by a robfig/cron scheduler job.
func SummarizeNewBills(ctx context.Context, db *sql.DB, onlyMissing bool) (int, error) {
	// Find bills without AI summaries.
	query := `
		SELECT id, number, title, full_text_url
		FROM bills
		WHERE (summary_ai IS NULL OR summary_ai = '')
		ORDER BY introduced_date DESC
		LIMIT 50  -- Batch size to avoid API overload
	`
	if !onlyMissing {
		query = `
			SELECT id, number, title, full_text_url
			FROM bills
			ORDER BY introduced_date DESC
			LIMIT 50
		`
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("query bills: %w", err)
	}
	defer rows.Close()

	requests := make(chan BillSummaryRequest, 64)
	go func() {
		defer close(requests)
		for rows.Next() {
			var billID, number, title, fullTextURL string
			if err := rows.Scan(&billID, &number, &title, &fullTextURL); err != nil {
				clog.Infof("[summarizer] scan error: %v", err)
				continue
			}

			// Skip if no full text URL.
			if strings.TrimSpace(fullTextURL) == "" {
				continue
			}

			select {
			case requests <- BillSummaryRequest{
				BillID:      billID,
				BillTitle:   title,
				FullTextURL: fullTextURL,
			}:
			case <-ctx.Done():
				clog.Infof("[summarizer] producer shutting down: %v", ctx.Err())
				return
			}
		}
	}()

	processed, err := SummarizeBillsFromChannel(ctx, db, requests)
	if err != nil {
		return processed, err
	}
	if err := rows.Err(); err != nil {
		return processed, err
	}
	return processed, nil
}

// fetchBillText fetches and extracts plain text from a bill's HTML document using goquery.
func fetchBillText(ctx context.Context, billID, url string) (string, error) {
	return fetchBillTextWithFallback(ctx, billID, url, true)
}

func fetchBillTextWithFallback(ctx context.Context, billID, url string, allowPERefresh bool) (string, error) {
	client := utils.NewHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrBillTextNotFound
	}

	if resp.StatusCode != http.StatusOK {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		snippet := strings.TrimSpace(collapseWhitespace(string(preview)))
		if allowPERefresh && isPEExpiredLinkResponse(url, resp.StatusCode, snippet) {
			if freshURL, ferr := resolveFreshPEBillTextURL(ctx, billID); ferr == nil && strings.TrimSpace(freshURL) != "" && freshURL != url {
				clog.Infof("[summarizer] refreshed expired PE bill link for %q", billID)
				return fetchBillTextWithFallback(ctx, billID, freshURL, false)
			}
		}
		if len(snippet) > 220 {
			snippet = snippet[:220] + "..."
		}
		if snippet == "" {
			snippet = "<empty body>"
		}
		return "", fmt.Errorf(
			"GET %s: http %d %s (content-type=%q, body=%q)",
			url,
			resp.StatusCode,
			http.StatusText(resp.StatusCode),
			resp.Header.Get("Content-Type"),
			snippet,
		)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/pdf") || strings.HasSuffix(strings.ToLower(url), ".pdf") {
		pdfData, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return extractPDFText(pdfData)
	}

	// Cap read size at 8 MiB (8 * 1024 * 1024 bytes) to avoid unbounded memory
	// use on unexpectedly large responses while keeping enough headroom for
	// typical HTML bill text pages.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBillTextResponseBytes))
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	// Use goquery to parse HTML and extract text.
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		clog.Infof("[summarizer] goquery parse failed for %s: %v; falling back to tokenizer extraction", sanitizeLogURL(url), err)
		fallbackText := extractTextWithTokenizer(body)
		return strings.TrimSpace(collapseWhitespace(fallbackText)), nil
	}

	// Remove script and style tags to clean text.
	doc.Find("script, style").Remove()

	// Extract all text content.
	text := doc.Text()

	// Collapse whitespace.
	text = collapseWhitespace(text)

	return strings.TrimSpace(text), nil
}

func isPEExpiredLinkResponse(rawURL string, statusCode int, snippet string) bool {
	if statusCode != http.StatusInternalServerError {
		return false
	}
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	if host != "docs.assembly.pe.ca" && !strings.HasPrefix(host, "127.0.0.1") && !strings.HasPrefix(host, "localhost") {
		return false
	}
	if !strings.Contains(strings.ToLower(u.Path), "/download/dms") {
		return false
	}
	lower := strings.ToLower(snippet)
	return strings.Contains(lower, "error retrieving file") && strings.Contains(lower, "link is expired")
}

func resolveFreshPEBillTextURL(ctx context.Context, billID string) (string, error) {
	if strings.TrimSpace(billID) == "" {
		return "", fmt.Errorf("empty bill id")
	}
	matches := peBillIDPartsRe.FindStringSubmatch(strings.TrimSpace(billID))
	if len(matches) != 4 {
		return "", fmt.Errorf("not a PE bill id: %s", billID)
	}
	legislature, err := strconv.Atoi(matches[1])
	if err != nil {
		return "", err
	}
	session, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", err
	}
	billNumber := strings.ToUpper(strings.TrimSpace(matches[3]))
	if billNumber == "" {
		return "", fmt.Errorf("missing bill number")
	}

	searchPayload := map[string]interface{}{
		"appName":     "LegislativeAssemblyBillProgress",
		"featureName": "LegislativeAssemblyBillProgress",
		"metaVars":    map[string]interface{}{"service_id": nil, "save_location": nil},
		"queryVars": map[string]interface{}{
			"service":         "LegislativeAssemblyBillProgress",
			"activity":        "LegislativeAssemblyBillSearch",
			"search_bills":    "true",
			"wdf_url_query":   "true",
			"search":          "assembly",
			"general_assembly": strconv.Itoa(legislature),
			"session":         strconv.Itoa(session),
		},
		"queryName": "LegislativeAssemblyBillSearch",
	}

	body, err := postPEWorkflow(ctx, searchPayload)
	if err != nil {
		return "", err
	}
	var searchResp peWDFTreeResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", err
	}
	billDocID := findPEBillDocID(searchResp.Data, billNumber)
	if billDocID == "" {
		return "", fmt.Errorf("bill doc id not found for %s", billID)
	}

	viewPayload := map[string]interface{}{
		"appName":     "LegislativeAssemblyBillProgress",
		"featureName": "LegislativeAssemblyBillProgress",
		"metaVars":    map[string]interface{}{"service_id": nil, "save_location": nil},
		"queryVars": map[string]interface{}{
			"service":  "LegislativeAssemblyBillProgress",
			"activity": "LegislativeAssemblyBillView",
			"id":       billDocID,
		},
		"queryName": "LegislativeAssemblyBillView",
	}

	body, err = postPEWorkflow(ctx, viewPayload)
	if err != nil {
		return "", err
	}
	var viewResp peWDFTreeResponse
	if err := json.Unmarshal(body, &viewResp); err != nil {
		return "", err
	}
	return firstPEHref(viewResp.Data), nil
}

func postPEWorkflow(ctx context.Context, payload map[string]interface{}) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, peiWDFWorkflowURL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Client-Show-Status", "true")

	client := utils.NewHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("PE workflow http %d: %s", resp.StatusCode, strings.TrimSpace(collapseWhitespace(string(preview))))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func collectPEWDFRows(nodes []peWDFNode) []peWDFNode {
	rows := make([]peWDFNode, 0)
	for _, n := range nodes {
		if n.Type == "TableV2Row" {
			rows = append(rows, n)
		}
		rows = append(rows, collectPEWDFRows(n.Children)...)
	}
	return rows
}

func firstPEHref(nodes []peWDFNode) string {
	for _, node := range nodes {
		if node.Type == "LinkV2" {
			var ld peWDFLinkData
			if json.Unmarshal(node.Data, &ld) == nil && ld.Href != nil {
				if href := strings.TrimSpace(*ld.Href); href != "" {
					return href
				}
			}
		}
		if href := firstPEHref(node.Children); href != "" {
			return href
		}
	}
	return ""
}

func findPEBillDocID(nodes []peWDFNode, wantedBillNumber string) string {
	wantedBillNumber = strings.ToUpper(strings.TrimSpace(wantedBillNumber))
	for _, row := range collectPEWDFRows(nodes) {
		if len(row.Children) < 2 || len(row.Children[0].Children) == 0 {
			continue
		}
		var number string
		var cd peWDFCellData
		if json.Unmarshal(row.Children[1].Data, &cd) == nil && cd.Text != nil {
			number = strings.ToUpper(strings.TrimSpace(*cd.Text))
		}
		if number == "" || number != wantedBillNumber {
			continue
		}
		linkNode := row.Children[0].Children[0]
		if linkNode.Type != "LinkV2" {
			continue
		}
		var ld peWDFLinkData
		if json.Unmarshal(linkNode.Data, &ld) != nil {
			continue
		}
		if id := strings.TrimSpace(ld.QueryParams["id"]); id != "" {
			return id
		}
	}
	return ""
}

// extractTextWithTokenizer extracts visible text from HTML using a streaming
// tokenizer. It is used as a fallback when full DOM parsing fails on malformed
// input (for example, extremely deep nesting). script/style content is excluded
// by tracking whether the tokenizer is currently inside those tags.
func extractTextWithTokenizer(raw []byte) string {
	z := html.NewTokenizer(bytes.NewReader(raw))
	var b strings.Builder
	var skipDepth int

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return b.String()
		case html.StartTagToken:
			tok := z.Token()
			tag := strings.ToLower(tok.Data)
			if tag == "script" || tag == "style" {
				skipDepth++
			}
		case html.EndTagToken:
			tok := z.Token()
			tag := strings.ToLower(tok.Data)
			if (tag == "script" || tag == "style") && skipDepth > 0 {
				skipDepth--
			}
		case html.TextToken:
			if skipDepth > 0 {
				continue
			}
			b.WriteString(z.Token().Data)
			b.WriteByte(' ')
		}
	}
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func sanitizeLogURL(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return raw
	}
	return (&neturl.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   u.Path,
	}).String()
}
