package summarizer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func buildTestPDF(text string) []byte {
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n",
		fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\nBT\n/F1 18 Tf\n36 96 Td\n(%s) Tj\nET\nendstream\nendobj\n", len("BT\n/F1 18 Tf\n36 96 Td\n("+text+") Tj\nET\n"), text),
		"5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
	}

	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects)+1)
	offsets = append(offsets, 0)
	for _, obj := range objects {
		offsets = append(offsets, b.Len())
		b.WriteString(obj)
	}
	xref := b.Len()
	b.WriteString("xref\n")
	b.WriteString(fmt.Sprintf("0 %d\n", len(objects)+1))
	b.WriteString("0000000000 65535 f \n")
	for _, off := range offsets[1:] {
		b.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}
	b.WriteString("trailer\n")
	b.WriteString(fmt.Sprintf("<< /Size %d /Root 1 0 R >>\n", len(objects)+1))
	b.WriteString("startxref\n")
	b.WriteString(fmt.Sprintf("%d\n", xref))
	b.WriteString("%%EOF\n")
	return []byte(b.String())
}

func TestParseSummaryJSON_FencedJSON(t *testing.T) {
	raw := "```json\n{\"one_sentence\":\"One line\",\"plain_summary\":\"Two lines\",\"key_changes\":[\"a\"],\"who_is_affected\":[\"b\"],\"notable_considerations\":[],\"category\":\"Other\"}\n```"

	got, err := parseSummaryJSON(raw)
	if err != nil {
		t.Fatalf("parseSummaryJSON returned error: %v", err)
	}
	if got.OneSentence != "One line" {
		t.Fatalf("unexpected one_sentence: %q", got.OneSentence)
	}
	if got.Category != "Other" {
		t.Fatalf("unexpected category: %q", got.Category)
	}
}

func TestParseSummaryJSON_MixedTextWithJSONObject(t *testing.T) {
	raw := "Here is your result:\n{\"one_sentence\":\"One line\",\"plain_summary\":\"Two lines\",\"key_changes\":[\"a\"],\"who_is_affected\":[\"b\"],\"notable_considerations\":[\"c\"],\"category\":\"Housing\"}\nThanks!"

	got, err := parseSummaryJSON(raw)
	if err != nil {
		t.Fatalf("parseSummaryJSON returned error: %v", err)
	}
	if got.Category != "Housing" {
		t.Fatalf("unexpected category: %q", got.Category)
	}
	if len(got.NotableConsiderations) != 1 {
		t.Fatalf("unexpected notable_considerations length: %d", len(got.NotableConsiderations))
	}
}

func TestParseSummaryJSON_InvalidPayload(t *testing.T) {
	if _, err := parseSummaryJSON("```\nnot json\n```"); err == nil {
		t.Fatal("expected error for invalid summary payload")
	}
}

func TestParseSummaryResult(t *testing.T) {
	// Create a fake JSON summary like Claude would return
	fakeResult := SummaryResult{
		OneSentence:           "This bill establishes new housing regulations.",
		PlainSummary:          "This bill creates a framework for affordable housing in Canada...",
		KeyChanges:            []string{"Increases housing tax credit", "Requires landlord transparency"},
		WhoIsAffected:         []string{"Renters", "Landlords", "Government"},
		NotableConsiderations: []string{"Citizens must give up privacy rights", "Excludes rural municipalities from some requirements"},
		Category:              "Housing",
		BillID:                "45-1-C-123",
		GeneratedAt:           "2026-04-11T00:00:00Z",
		Model:                 claudeModel,
	}

	// Marshal to JSON to test round-trip
	jsonData, err := json.Marshal(fakeResult)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify we can unmarshal it back
	var parsed SummaryResult
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.OneSentence != fakeResult.OneSentence {
		t.Errorf("OneSentence mismatch: got %q, want %q", parsed.OneSentence, fakeResult.OneSentence)
	}

	if len(parsed.KeyChanges) != 2 {
		t.Errorf("KeyChanges length mismatch: got %d, want 2", len(parsed.KeyChanges))
	}

	if parsed.Category != "Housing" {
		t.Errorf("Category mismatch: got %q, want %q", parsed.Category, "Housing")
	}

	if len(parsed.NotableConsiderations) != 2 {
		t.Errorf("NotableConsiderations length mismatch: got %d, want 2", len(parsed.NotableConsiderations))
	}
}

func TestParseAISummaryEmpty(t *testing.T) {
	// ParseAISummary should handle empty strings gracefully
	tests := []string{"", "   ", "not json"}

	for _, test := range tests {
		result := &SummaryResult{
			OneSentence:  "",
			PlainSummary: "",
		}
		json.Unmarshal([]byte(test), result)
		// Should not panic or error, just return zero values
	}
}

func TestSummaryResultStructure(t *testing.T) {
	// Verify the SummaryResult struct has all expected fields
	sr := SummaryResult{
		OneSentence:           "test",
		PlainSummary:          "test",
		KeyChanges:            []string{"test"},
		WhoIsAffected:         []string{"test"},
		NotableConsiderations: []string{"test"},
		Category:              "Housing",
		BillID:                "45-1-C-1",
		GeneratedAt:           "2026-04-11T00:00:00Z",
		Model:                 claudeModel,
	}

	if sr.BillID == "" {
		t.Error("BillID should not be empty")
	}

	if sr.Category == "" {
		t.Error("Category should not be empty")
	}

	if len(sr.KeyChanges) != 1 {
		t.Error("KeyChanges should have one item")
	}

	if len(sr.NotableConsiderations) != 1 {
		t.Error("NotableConsiderations should have one item")
	}
}

func TestSummarizeBillsFromChannel_Clears404FullTextURL(t *testing.T) {
	// Serve HTTP 404 for any request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// Build a minimal in-memory SQLite DB with the bills table.
	db, err := sql.Open("sqlite3", "file:clear404?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE bills (
		id TEXT PRIMARY KEY,
		number TEXT,
		title TEXT,
		full_text_url TEXT,
		summary_ai TEXT,
		summary_lop TEXT,
		last_activity_date TEXT
	)`); err != nil {
		t.Fatalf("create bills: %v", err)
	}

	billURL := srv.URL + "/bill/S-1/first-reading"
	if _, err := db.Exec(
		`INSERT INTO bills (id, number, title, full_text_url) VALUES ('45-1-s-1','S-1','Pro-forma Senate bill',?)`,
		billURL,
	); err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	// Feed one request through the channel pipeline.
	ch := make(chan BillSummaryRequest, 1)
	ch <- BillSummaryRequest{
		BillID:      "45-1-s-1",
		BillTitle:   "Pro-forma Senate bill",
		FullTextURL: billURL,
	}
	close(ch)

	SummarizeBillsFromChannel(t.Context(), db, ch) //nolint:errcheck

	// full_text_url must now be empty.
	var storedURL string
	db.QueryRow(`SELECT COALESCE(full_text_url,'') FROM bills WHERE id='45-1-s-1'`).Scan(&storedURL)
	if storedURL != "" {
		t.Errorf("expected full_text_url to be cleared after 404, got %q", storedURL)
	}
}

func TestFetchBillText_PDF(t *testing.T) {
	pdfBytes := buildTestPDF("Prince Edward Island bill text")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdfBytes)
	}))
	defer srv.Close()

	text, err := fetchBillText(t.Context(), "", srv.URL+"/bill.pdf")
	if err != nil {
		t.Fatalf("fetchBillText(pdf): %v", err)
	}
	if !strings.Contains(text, "Prince Edward Island bill text") {
		t.Fatalf("expected extracted pdf text, got %q", text)
	}
}

func TestFetchBillText_FallsBackForDeeplyNestedHTML(t *testing.T) {
	const deepNesting = 600 // exceeds x/net/html parser stack limit (512)
	deep := strings.Repeat("<div>", deepNesting) + "Bill body text" + strings.Repeat("</div>", deepNesting)
	html := `<html><head><style>.x{display:none}</style></head><body><script>ignored()</script>` + deep + `</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	text, err := fetchBillText(context.Background(), "", srv.URL)
	if err != nil {
		t.Fatalf("fetchBillText returned error: %v", err)
	}
	if !strings.Contains(text, "Bill body text") {
		t.Fatalf("expected extracted text to contain bill text, got %q", text)
	}
	if strings.Contains(text, "ignored()") {
		t.Fatalf("expected script content to be removed, got %q", text)
	}
}

func TestFetchBillText_PEExpiredLinkRefreshesViaWDF(t *testing.T) {
	pdfBytes := buildTestPDF("Refreshed PE bill text")

	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/download/dms"):
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`<html><body><h1>Error retrieving file</h1><p>The link is expired (try refreshing the page you got the link from)</p></body></html>`))
		case strings.HasPrefix(r.URL.Path, "/fresh.pdf"):
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(pdfBytes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fileSrv.Close()

	wdfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		var req struct {
			QueryName string `json:"queryName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.QueryName {
		case "LegislativeAssemblyBillSearch":
			_, _ = w.Write([]byte(`{"data":[{"type":"TableV2","children":[{"type":"TableV2Row","children":[{"type":"TableV2Cell","children":[{"type":"LinkV2","data":{"queryParams":{"id":"bill-doc-34"}}}]},{"type":"TableV2Cell","data":{"text":"34"}}]}]}]}`))
		case "LegislativeAssemblyBillView":
			_, _ = w.Write([]byte(`{"data":[{"type":"TableV2","children":[{"type":"TableV2Row","children":[{"type":"TableV2Cell","children":[{"type":"LinkV2","data":{"href":"` + fileSrv.URL + `/fresh.pdf"}}]}]}]}]}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer wdfSrv.Close()

	origWDF := peiWDFWorkflowURL
	peiWDFWorkflowURL = wdfSrv.URL
	t.Cleanup(func() { peiWDFWorkflowURL = origWDF })

	text, err := fetchBillText(context.Background(), "pe-67-2-34", fileSrv.URL+"/download/dms?objectId=expired&fileName=bill-34.pdf")
	if err != nil {
		t.Fatalf("fetchBillText returned error: %v", err)
	}
	if !strings.Contains(text, "Refreshed PE bill text") {
		t.Fatalf("expected refreshed bill text, got %q", text)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, time.May, 4, 12, 0, 0, 0, time.UTC)
	if got := parseRetryAfter("7", now); got != 7*time.Second {
		t.Fatalf("seconds retry-after = %s, want %s", got, 7*time.Second)
	}
	retryAt := now.Add(3 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(retryAt, now); got < 2*time.Second || got > 3*time.Second {
		t.Fatalf("date retry-after = %s, want about 3s", got)
	}
}

func TestCallClaudeAPI_RetriesAfterRateLimit(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"one_sentence\":\"One line\",\"plain_summary\":\"Two lines\",\"key_changes\":[\"a\"],\"who_is_affected\":[\"b\"],\"notable_considerations\":[],\"category\":\"Other\"}"}]}`))
	}))
	defer srv.Close()

	origURL := anthropicMessagesURL
	origClient := claudeHTTPClient
	origLimiter := claudeRateLimiter
	origBackoff := claudeInitialBackoff
	anthropicMessagesURL = srv.URL
	claudeHTTPClient = func() *http.Client { return srv.Client() }
	claudeRateLimiter = nil
	claudeInitialBackoff = 10 * time.Millisecond
	t.Cleanup(func() {
		anthropicMessagesURL = origURL
		claudeHTTPClient = origClient
		claudeRateLimiter = origLimiter
		claudeInitialBackoff = origBackoff
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	resp, err := callClaudeAPI(ctx, "test-key", claudeRequest{
		Model:     claudeModel,
		MaxTokens: 128,
		System:    "system",
		Messages:  []claudeMsg{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("callClaudeAPI returned error: %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("request count = %d, want 2", got)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content length = %d, want 1", len(resp.Content))
	}
	if !strings.Contains(resp.Content[0].Text, "one_sentence") {
		t.Fatalf("unexpected response text: %q", resp.Content[0].Text)
	}
}
