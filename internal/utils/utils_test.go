package utils_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/philspins/opendocket/internal/utils"
)

// ── ExtractBillID ─────────────────────────────────────────────────────────────

func TestExtractBillID(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://www.parl.ca/legisinfo/en/bill/45-1/c-47", "45-1-c-47"},
		{"https://www.parl.ca/legisinfo/en/bill/45-1/s-209", "45-1-s-209"},
		{"https://www.parl.ca/legisinfo/en/bill/45-1/C-47", "45-1-c-47"}, // normalise to lower
		{"https://www.parl.ca/legisinfo/en/bills/rss", ""},               // no bill path
		{"", ""},
	}
	for _, c := range cases {
		got := utils.ExtractBillID(c.url)
		if got != c.want {
			t.Errorf("ExtractBillID(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

// ── ExtractMemberID ───────────────────────────────────────────────────────────

func TestExtractMemberID(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		// Legacy numeric-only format
		{"https://www.ourcommons.ca/Members/en/123006", "123006"},
		{"https://www.ourcommons.ca/Members/en/123006?tab=votes", "123006"},
		{"/Members/en/99999", "99999"},
		// Current name(ID) format returned by ourcommons.ca and the Represent API
		{"https://www.ourcommons.ca/Members/en/parm-bains(111067)", "111067"},
		{"https://www.ourcommons.ca/Members/en/ziad-aboultaif(89156)", "89156"},
		{"/Members/en/jane-doe(111)", "111"},
		// No match cases
		{"https://www.ourcommons.ca/en/", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := utils.ExtractMemberID(c.url)
		if got != c.want {
			t.Errorf("ExtractMemberID(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

// ── DivisionID ────────────────────────────────────────────────────────────────

func TestDivisionID(t *testing.T) {
	if got := utils.DivisionID(45, 1, 892); got != "45-1-892" {
		t.Errorf("got %q, want 45-1-892", got)
	}
}

// ── BillIDFromParts ───────────────────────────────────────────────────────────

func TestBillIDFromParts(t *testing.T) {
	cases := []struct {
		parliament, session int
		billNumber          string
		want                string
	}{
		{45, 1, "C-47", "45-1-c-47"},
		{45, 1, "S-209", "45-1-s-209"},
		{45, 1, "c-47", "45-1-c-47"},     // already lowercase
		{45, 1, "  C-47  ", "45-1-c-47"}, // trims whitespace
		{45, 1, "", ""},                  // empty input → empty output
	}
	for _, c := range cases {
		got := utils.BillIDFromParts(c.parliament, c.session, c.billNumber)
		if got != c.want {
			t.Errorf("BillIDFromParts(%d, %d, %q) = %q, want %q", c.parliament, c.session, c.billNumber, got, c.want)
		}
	}
}

// ── ExtractBillNumber ─────────────────────────────────────────────────────────

func TestExtractBillNumber(t *testing.T) {
	cases := []struct{ text, want string }{
		{"Motion on C-47", "C-47"},
		{"Third reading of S-209", "S-209"},
		{"Second reading of Bill C-230, An Act respecting X", "C-230"},
		{"S-5 third reading", "S-5"},
		{"Procedural motion", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := utils.ExtractBillNumber(c.text)
		if got != c.want {
			t.Errorf("ExtractBillNumber(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// ── BillNumberFromID ──────────────────────────────────────────────────────────

func TestBillNumberFromID(t *testing.T) {
	cases := []struct{ id, want string }{
		{"45-1-c-47", "C-47"},
		{"45-1-s-209", "S-209"},
		{"45-1", ""},
	}
	for _, c := range cases {
		got := utils.BillNumberFromID(c.id)
		if got != c.want {
			t.Errorf("BillNumberFromID(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

// ── ParliamentSessionFromBillID ───────────────────────────────────────────────

func TestParliamentSessionFromBillID(t *testing.T) {
	p, s, ok := utils.ParliamentSessionFromBillID("45-1-c-47")
	if !ok || p != 45 || s != 1 {
		t.Errorf("got p=%d s=%d ok=%v, want 45 1 true", p, s, ok)
	}
	_, _, ok2 := utils.ParliamentSessionFromBillID("invalid")
	if ok2 {
		t.Error("expected ok=false for invalid input")
	}
}

// ── BillChamber ───────────────────────────────────────────────────────────────

func TestBillChamber(t *testing.T) {
	cases := []struct{ number, want string }{
		{"C-47", "commons"},
		{"c-47", "commons"},
		{"S-209", "senate"},
		{"s-5", "senate"},
		{"", "commons"},
	}
	for _, c := range cases {
		got := utils.BillChamber(c.number)
		if got != c.want {
			t.Errorf("BillChamber(%q) = %q, want %q", c.number, got, c.want)
		}
	}
}

// ── ParseDate ─────────────────────────────────────────────────────────────────

func TestParseDate(t *testing.T) {
	cases := []struct{ input, want string }{
		{"2024-04-03", "2024-04-03"},
		{"April 3, 2024", "2024-04-03"},
		{"3 April 2024", "2024-04-03"},
		{"Apr 3, 2024", "2024-04-03"},
		{"2024/04/03", "2024-04-03"},
		{"  2024-04-03  ", "2024-04-03"},
		{"", ""},
		{"not-a-date", ""},
	}
	for _, c := range cases {
		got := utils.ParseDate(c.input)
		if got != c.want {
			t.Errorf("ParseDate(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── FindDateInText ────────────────────────────────────────────────────────────

func TestFindDateInText(t *testing.T) {
	cases := []struct{ text, want string }{
		{"Passed on 2024-04-03 in committee", "2024-04-03"},
		{"Reading on April 3, 2024 was agreed to", "2024-04-03"},
		{"No date here at all", ""},
	}
	for _, c := range cases {
		got := utils.FindDateInText(c.text)
		if got != c.want {
			t.Errorf("FindDateInText(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// ── TodayISO / NowISO ────────────────────────────────────────────────────────

func TestTodayISO_ReturnsISODateFormat(t *testing.T) {
	got := utils.TodayISO()
	if len(got) != 10 || got[4] != '-' || got[7] != '-' {
		t.Errorf("TodayISO() = %q, want YYYY-MM-DD", got)
	}
	if _, err := time.Parse("2006-01-02", got); err != nil {
		t.Errorf("TodayISO() = %q is not a valid ISO date: %v", got, err)
	}
}

func TestNowISO_ReturnsISODateTimeFormat(t *testing.T) {
	got := utils.NowISO()
	if len(got) != 19 || got[4] != '-' || got[10] != 'T' || got[13] != ':' {
		t.Errorf("NowISO() = %q, want YYYY-MM-DDTHH:MM:SS", got)
	}
	if _, err := time.Parse("2006-01-02T15:04:05", got); err != nil {
		t.Errorf("NowISO() = %q is not valid: %v", got, err)
	}
}

// ── NewHTTPClient / uaTransport ───────────────────────────────────────────────

func TestNewHTTPClient_SetsUserAgentHeader(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := utils.NewHTTPClient()
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotUA != utils.AppUserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, utils.AppUserAgent)
	}
}

func TestNewHTTPClientWithTimeout_SetsUserAgentHeader(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := utils.NewHTTPClientWithTimeout(5 * time.Second)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotUA != utils.AppUserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, utils.AppUserAgent)
	}
}

func TestUATransport_SetsAcceptHeaderWhenAbsent(t *testing.T) {
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := utils.NewHTTPClient()
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotAccept == "" {
		t.Error("uaTransport should set Accept header when not already present")
	}
	if !strings.Contains(gotAccept, "text/html") {
		t.Errorf("Accept = %q, expected to contain text/html", gotAccept)
	}
}

func TestUATransport_PreservesExistingAcceptHeader(t *testing.T) {
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := utils.NewHTTPClient()
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json (pre-set header should not be overwritten)", gotAccept)
	}
}

// ── ParliamentSessionFromBillID (additional branches) ─────────────────────────

func TestParliamentSessionFromBillID_NonNumericParliament(t *testing.T) {
	_, _, ok := utils.ParliamentSessionFromBillID("abc-1-c-47")
	if ok {
		t.Error("expected ok=false when parliament is non-numeric")
	}
}

func TestParliamentSessionFromBillID_NonNumericSession(t *testing.T) {
	_, _, ok := utils.ParliamentSessionFromBillID("45-xyz-c-47")
	if ok {
		t.Error("expected ok=false when session is non-numeric")
	}
}

// ── LoadDotEnv ────────────────────────────────────────────────────────────────

func writeDotEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadDotEnv_FileNotFound(t *testing.T) {
	err := utils.LoadDotEnv("/tmp/nonexistent-opendocket-test-dotenv-xyz.env")
	if err != nil {
		t.Errorf("LoadDotEnv(missing file) = %v, want nil", err)
	}
}

func TestLoadDotEnv_SetsKeyValuePairs(t *testing.T) {
	path := writeDotEnv(t, "DOTENV_TEST_KEY1=hello\nDOTENV_TEST_KEY2=world\n")
	t.Cleanup(func() {
		os.Unsetenv("DOTENV_TEST_KEY1")
		os.Unsetenv("DOTENV_TEST_KEY2")
	})
	if err := utils.LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("DOTENV_TEST_KEY1"); got != "hello" {
		t.Errorf("DOTENV_TEST_KEY1 = %q, want hello", got)
	}
	if got := os.Getenv("DOTENV_TEST_KEY2"); got != "world" {
		t.Errorf("DOTENV_TEST_KEY2 = %q, want world", got)
	}
}

func TestLoadDotEnv_SkipsBlankLinesAndComments(t *testing.T) {
	path := writeDotEnv(t, "\n# This is a comment\nDOTENV_TEST_SKIP=set\n")
	t.Cleanup(func() { os.Unsetenv("DOTENV_TEST_SKIP") })
	if err := utils.LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("DOTENV_TEST_SKIP"); got != "set" {
		t.Errorf("DOTENV_TEST_SKIP = %q, want set", got)
	}
}

func TestLoadDotEnv_DoesNotOverwriteExisting(t *testing.T) {
	t.Setenv("DOTENV_TEST_EXISTING", "original")
	path := writeDotEnv(t, "DOTENV_TEST_EXISTING=overwritten\n")
	if err := utils.LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("DOTENV_TEST_EXISTING"); got != "original" {
		t.Errorf("DOTENV_TEST_EXISTING = %q, want original (existing vars must not be overwritten)", got)
	}
}

func TestLoadDotEnv_StripsInlineComments(t *testing.T) {
	path := writeDotEnv(t, "DOTENV_TEST_INLINE=value # inline comment\n")
	t.Cleanup(func() { os.Unsetenv("DOTENV_TEST_INLINE") })
	if err := utils.LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("DOTENV_TEST_INLINE"); got != "value" {
		t.Errorf("DOTENV_TEST_INLINE = %q, want value (inline comment not stripped)", got)
	}
}

func TestLoadDotEnv_StripsQuotes(t *testing.T) {
	path := writeDotEnv(t, "DOTENV_TEST_DQ=\"double quoted\"\nDOTENV_TEST_SQ='single quoted'\n")
	t.Cleanup(func() {
		os.Unsetenv("DOTENV_TEST_DQ")
		os.Unsetenv("DOTENV_TEST_SQ")
	})
	if err := utils.LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("DOTENV_TEST_DQ"); got != "double quoted" {
		t.Errorf("DOTENV_TEST_DQ = %q, want double quoted", got)
	}
	if got := os.Getenv("DOTENV_TEST_SQ"); got != "single quoted" {
		t.Errorf("DOTENV_TEST_SQ = %q, want single quoted", got)
	}
}

func TestLoadDotEnv_SkipsLinesWithoutEquals(t *testing.T) {
	path := writeDotEnv(t, "NOEQUALS\nDOTENV_TEST_AFTER=ok\n")
	t.Cleanup(func() { os.Unsetenv("DOTENV_TEST_AFTER") })
	if err := utils.LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("DOTENV_TEST_AFTER"); got != "ok" {
		t.Errorf("DOTENV_TEST_AFTER = %q, want ok", got)
	}
}
