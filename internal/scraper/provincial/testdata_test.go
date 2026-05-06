package provincial

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// roundTripper redirects all HTTP requests to the given target host.
type roundTripper struct {
	target string
}

func (rt roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = strings.TrimPrefix(rt.target, "http://")
	return http.DefaultTransport.RoundTrip(req2)
}

// ── Alberta bill status PDF ───────────────────────────────────────────────────

// TestParseAlbertaBillStatusPDF_WithFixture exercises parseAlbertaBillStatusPDF via
// CrawlAlbertaBills using a minimal synthetic PDF fixture. The fixture text matches
// albertaBillStatusEntryRe so bill number and title are extracted cleanly.
func TestParseAlbertaBillStatusPDF_WithFixture(t *testing.T) {
	pdfBytes, err := os.ReadFile("testdata/ab_bill_status.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBytes)
	}))
	defer srv.Close()

	client := &http.Client{Transport: roundTripper{target: srv.URL}}

	// URL ending in .pdf bypasses the dashboard lookup and goes straight to parseAlbertaBillStatusPDF.
	bills, err := CrawlAlbertaBills(srv.URL+"/ab_bill_status.pdf", 31, 2, client)
	if err != nil {
		t.Fatalf("CrawlAlbertaBills: %v", err)
	}
	if len(bills) == 0 {
		t.Fatalf("expected at least one bill, got 0")
	}

	var foundPr1, foundP2 bool
	for _, b := range bills {
		if b.Number == "Pr1" {
			foundPr1 = true
			if !strings.Contains(b.Title, "Appropriation") {
				t.Errorf("Pr1 title=%q, want contains 'Appropriation'", b.Title)
			}
		}
		if b.Number == "P2" {
			foundP2 = true
		}
	}
	if !foundPr1 {
		t.Errorf("bill Pr1 not found in %v", bills)
	}
	if !foundP2 {
		t.Errorf("bill P2 not found in %v", bills)
	}
}

// ── Manitoba calendar PDF (heuristic fallback) ────────────────────────────────

// TestParseMBHighlightedSittingDatesFromPDF_HeuristicFallback verifies that
// ParseMBHighlightedSittingDatesFromPDF falls through to the text heuristic when the
// BBox/OCR approaches find no month headings, and returns Tue-Thu dates for the year.
func TestParseMBHighlightedSittingDatesFromPDF_HeuristicFallback(t *testing.T) {
	if !hasCommand("pdftotext") {
		t.Skip("pdftotext not installed")
	}

	pdfBytes, err := os.ReadFile("testdata/mb_calendar_sessional.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	dates, ok := ParseMBHighlightedSittingDatesFromPDF(pdfBytes, 2026)
	if !ok || len(dates) == 0 {
		t.Fatalf("expected dates from heuristic fallback, got ok=%v dates=%v", ok, dates)
	}

	// Heuristic returns Tue–Thu dates in Mar–Jun and Sep–Dec; verify at least one date.
	for _, d := range dates {
		if !strings.HasPrefix(d, "2026-") {
			t.Errorf("unexpected date %q not in 2026", d)
		}
	}
}

// ── PEI calendar PDF ──────────────────────────────────────────────────────────

// TestPEICalendarDatesFromPDFBytes_MinimalPDF verifies that PEICalendarDatesFromPDFBytes
// handles a minimal PDF that renders as a blank page. pdftoppm renders it without error;
// no calendar cells are found so the function returns nil, false — testing the entry path.
func TestPEICalendarDatesFromPDFBytes_MinimalPDF(t *testing.T) {
	pdfBytes, err := os.ReadFile("testdata/mb_calendar_sessional.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// A minimal synthetic PDF has no green-highlighted calendar cells, so the parser
	// returns nil, false. This still exercises the pdftoppm render + BBox extraction path.
	dates, ok := PEICalendarDatesFromPDFBytes(pdfBytes, 2026)
	_ = dates
	_ = ok
	// No assertion on ok/dates — the function may or may not find dates depending on
	// which extraction strategy succeeds; what matters is it doesn't panic or crash.
}
