package provincial

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/opendocket/internal/db"
)

func mustDocFromHTML(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("goquery.NewDocumentFromReader: %v", err)
	}
	return doc
}

func TestHasPDFTextShowOperator(t *testing.T) {
	if !hasPDFTextShowOperator("BT /F9 7.999 Tf 0 0 0 rg 380.167 TL 242.496 325.155 Td (Kaeding, ) Tj T* ET") {
		t.Fatal("expected line with inline Tj operator to be detected")
	}
	if !hasPDFTextShowOperator("(Members\376\377\000'\000 )Tj") {
		t.Fatal("expected line ending in Tj without preceding space to be detected")
	}
	if !hasPDFTextShowOperator("[(Legislati)-6.2(v)1(e Assem)-6(b)2.2(ly )]TJ") {
		t.Fatal("expected line ending in TJ without preceding space to be detected")
	}
	if hasPDFTextShowOperator("q 16.622 368.075 754.532 14.0 re W n") {
		t.Fatal("expected non-text operator line to be ignored")
	}
}

func TestExtractPlainVoteNames_CollapsesSplitUppercaseSurnames(t *testing.T) {
	block := `AYE B ALCAEN B EREZA D ELA C RUZ W OWCHUK ................................ ..... 46 NAY ................................ 0`
	names := extractPlainVoteNames(block)
	want := []string{"BALCAEN", "BEREZA", "DELA CRUZ", "WOWCHUK"}
	if len(names) != len(want) {
		t.Fatalf("len(names)=%d, want %d (%v)", len(names), len(want), names)
	}
	for i, got := range names {
		if got != want[i] {
			t.Fatalf("names[%d]=%q, want %q", i, got, want[i])
		}
	}
}

func TestExtractProvincialBillNumber(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Bill 12 - An Act", "12"},
		{"BILL A-23 respecting schools", "A-23"},
		{"Bill (No. M 201) intituled Example Act", "M-201"},
		{"Motion on C-47", "C-47"},
		{"No bill here", ""},
	}
	for _, c := range cases {
		got := ExtractProvincialBillNumber(c.in)
		if got != c.want {
			t.Fatalf("ExtractProvincialBillNumber(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestCrawlProvincialBillsFromIndex_ParsesBillLinks(t *testing.T) {
	body := `<html><body>
  <ul>
    <li><a href="/bills/12">Bill 12 - Health Statute Amendment Act</a> (April 7, 2026)</li>
    <li><a href="/bills/13">Bill 13 - Education Modernization Act</a></li>
  </ul>
</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	bills, err := CrawlProvincialBillsFromIndex(srv.URL, "ab", 31, 1, "alberta", srv.Client())
	if err != nil {
		t.Fatalf("CrawlProvincialBillsFromIndex: %v", err)
	}
	if len(bills) != 2 {
		t.Fatalf("len(bills)=%d want 2", len(bills))
	}
	if bills[0].ID == "" || bills[0].Number == "" {
		t.Fatalf("expected non-empty bill id and number: %+v", bills[0])
	}
}

func TestCrawlGenericProvincialVotes_ParsesCounts(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><a href="/votes/2026-04-07">Votes and Proceedings</a></body></html>`))
	})
	mux.HandleFunc("/votes/2026-04-07", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body>
      <h3>Bill 12 second reading</h3>
      <table>
        <tr><td>Yeas: 5</td><td>Nays: 2</td></tr>
      </table>
    </body></html>`))
	})

	divs, err := CrawlGenericProvincialVotes(srv.URL, "ab", "alberta", 31, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlGenericProvincialVotes: %v", err)
	}
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d want 1", len(divs))
	}
	if divs[0].Division.Yeas != 5 || divs[0].Division.Nays != 2 {
		t.Fatalf("counts=%d/%d want 5/2", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
}

func TestProvinceSpecificBillCrawlerEntryPoints(t *testing.T) {
	type billCrawler func(string, int, int, *http.Client) ([]ProvincialBillStub, error)

	index := `<html><body>
  <a href="/bills/10">Bill 10 - Test Bill</a>
</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(index))
	}))
	defer srv.Close()

	cases := []struct {
		name string
		fn   billCrawler
	}{
		{"alberta", CrawlAlbertaBills},
		{"bc", CrawlBritishColumbiaBills},
		{"manitoba", CrawlManitobaBills},
		{"new_brunswick", CrawlNewBrunswickBills},
		{"newfoundland_labrador", CrawlNewfoundlandAndLabradorBills},
		{"nova_scotia", CrawlNovaScotiaBills},
		{"ontario", CrawlOntarioBills},
		{"pei", CrawlPrinceEdwardIslandBills},
		{"quebec", CrawlQuebecBills},
		{"saskatchewan", CrawlSaskatchewanBills},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bills, err := tc.fn(srv.URL, 1, 1, srv.Client())
			if err != nil {
				t.Fatalf("crawler returned error: %v", err)
			}
			if len(bills) == 0 {
				t.Fatal("expected at least one bill parsed")
			}
		})
	}
}

func TestProvinceSpecificVoteCrawlerEntryPoints(t *testing.T) {
	type voteCrawler func(string, int, int, *http.Client) ([]ProvincialDivisionResult, error)

	vpHTML := `<!DOCTYPE html><html><body>
<p>Motion agreed to:</p>
<table class="division">
<tr><td class="head" colspan="4">Yeas &#8212; 9</td></tr>
<tr><td>Smith <br>Jones <br>Brown <br></td><td>Davis <br>Wilson <br>Taylor <br></td><td>Allen <br>Foster <br>Mok <br></td><td></td></tr>
<tr><td class="head" colspan="4">Nays &#8212; 2</td></tr>
<tr><td>Lee <br></td><td>Park <br></td><td></td><td></td></tr>
</table></body></html>`

	limsJSON := `{"allParliamentaryFileAttributes":{"nodes":[{"fileName":"test.htm","filePath":"/ldp/1st1st/votes/","published":true,"date":"2026-04-07T00:00:00","votesAttributesByFileId":{"nodes":[{"voteNumbers":"1"}]}}]}}`

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><a href="/votes/2026-04-07">Votes and Proceedings</a></body></html>`))
	})
	mux.HandleFunc("/votes/2026-04-07", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><table><tr><td>Yeas: 9</td><td>Nays: 2</td></tr></table></body></html>`))
	})
	mux.HandleFunc("/pdms/votes-and-proceedings/1st1st", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(limsJSON))
	})
	mux.HandleFunc("/pdms/ldp/1st1st/votes/test.htm", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(vpHTML))
	})

	cases := []struct {
		name string
		fn   voteCrawler
	}{
		{"bc", CrawlBritishColumbiaVotes},
		{"manitoba", CrawlManitobaVotes},
		{"new_brunswick", CrawlNewBrunswickVotes},
		{"newfoundland_labrador", CrawlNewfoundlandAndLabradorVotes},
		{"pei", CrawlPrinceEdwardIslandVotes},
		{"quebec", CrawlQuebecVotes},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			divs, err := tc.fn(srv.URL, 1, 1, srv.Client())
			if err != nil {
				t.Fatalf("crawler returned error: %v", err)
			}
			if len(divs) == 0 {
				t.Fatal("expected at least one division parsed")
			}
		})
	}
}

func TestResolveProvincialMemberID_StripsTitlesAndMatchesInitialPlusSurname(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	_, err = conn.Exec(`INSERT INTO members (id, name, province, chamber, active, government_level) VALUES
		('nb-legislature-wilson-sherry', 'Sherry Wilson', 'New Brunswick', 'new_brunswick', 1, 'provincial'),
		('nb-legislature-wilson-mary', 'Mary Wilson', 'New Brunswick', 'new_brunswick', 1, 'provincial'),
		('nb-legislature-savoie-glen', 'Glen Savoie', 'New Brunswick', 'new_brunswick', 1, 'provincial'),
		('nb-legislature-chiasson-chuck', 'Chuck Chiasson', 'New Brunswick', 'new_brunswick', 1, 'provincial'),
		('manitoba-legislature-dela-cruz-nellie', 'Nellie Kennedy Dela Cruz', 'Manitoba', 'manitoba', 1, 'provincial')`)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}

	tests := []struct {
		province   string
		sourceName string
		wantID     string
	}{
		{"New Brunswick", "Hon. Ms. S. Wilson", "nb-legislature-wilson-sherry"},
		{"New Brunswick", "Hon. Ms. M. Wilson", "nb-legislature-wilson-mary"},
		{"New Brunswick", "Hon. Mr. G. Savoie", "nb-legislature-savoie-glen"},
		{"New Brunswick", "Mr. C. Chiasson", "nb-legislature-chiasson-chuck"},
		{"Manitoba", "DELA CRUZ", "manitoba-legislature-dela-cruz-nellie"},
	}

	for _, tc := range tests {
		got, err := resolveProvincialMemberID(conn, tc.province, tc.sourceName)
		if err != nil {
			t.Fatalf("resolveProvincialMemberID(%q): %v", tc.sourceName, err)
		}
		if got != tc.wantID {
			t.Fatalf("resolveProvincialMemberID(%q)=%q, want %q", tc.sourceName, got, tc.wantID)
		}
	}
}

const noDelay = 0 * time.Millisecond

func newProvinceDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	dbConn, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	return dbConn
}

func TestCrawlProvinceSource_PersistsBillsAndDivisions(t *testing.T) {
	vpHTML := `<!DOCTYPE html><html><body>
<p>On the motion that Bill (No. 12) intituled Test Act be now read a second time, the House divided.</p>
<p>Motion carried on the following division:</p>
<table class="division">
<tr><td class="head" colspan="4">Yeas &#8212; 8</td></tr>
<tr><td>Smith <br>Jones <br></td><td>Brown <br>Davis <br></td><td>Wilson <br>Taylor <br></td><td>Allen <br>Foster <br></td></tr>
<tr><td class="head" colspan="4">Nays &#8212; 3</td></tr>
<tr><td>Lee <br></td><td>Chen <br></td><td>Park <br></td><td></td></tr>
</table></body></html>`

	limsJSON := `{"allParliamentaryFileAttributes":{"nodes":[{"fileName":"v260407.htm","filePath":"/ldp/31st1st/votes/","published":true,"date":"2026-04-07T00:00:00","votesAttributesByFileId":{"nodes":[{"voteNumbers":"1"}]}}]}}`

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/bills", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
      <h2>31st Legislature 1st Session</h2>
      <a href="/archives/99th-parliament/1st-session">Archive 99th Parliament, 1st Session</a>
      <a href="/bill/12">Bill 12 - Test Act</a>
    </body></html>`))
	})
	mux.HandleFunc("/pdms/votes-and-proceedings/31st1st", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(limsJSON))
	})
	mux.HandleFunc("/pdms/ldp/31st1st/votes/v260407.htm", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(vpHTML))
	})

	conn := newProvinceDB(t)
	src := ProvincialSource{
		Code:     "bc",
		Province: "British Columbia",
		Chamber:  "british_columbia",
		BillsURL: srv.URL + "/bills",
		VotesURL: srv.URL,
	}

	if err := CrawlProvinceSource(conn, srv.Client(), noDelay, src, nil, nil); err != nil {
		t.Fatalf("CrawlProvinceSource: %v", err)
	}

	var billCount int
	if err := conn.QueryRow(`SELECT COUNT(1) FROM bills WHERE id='bc-31-1-12'`).Scan(&billCount); err != nil {
		t.Fatalf("bill count query: %v", err)
	}
	if billCount != 1 {
		t.Fatalf("expected bill bc-31-1-12, count=%d", billCount)
	}

	var divCount int
	if err := conn.QueryRow(`SELECT COUNT(1) FROM divisions WHERE chamber='british_columbia'`).Scan(&divCount); err != nil {
		t.Fatalf("division count query: %v", err)
	}
	if divCount == 0 {
		t.Fatal("expected at least one british_columbia division")
	}

	var description string
	if err := conn.QueryRow(`SELECT description FROM divisions WHERE chamber='british_columbia' LIMIT 1`).Scan(&description); err != nil {
		t.Fatalf("division description query: %v", err)
	}
	if !strings.Contains(strings.ToLower(description), "read a second time") {
		t.Fatalf("expected stored division description to preserve vote context, got %q", description)
	}
	if description == "Test Act" {
		t.Fatalf("expected division description not to be overwritten by bill title, got %q", description)
	}
}

func TestCrawlProvinceSource_FallsBackToPreviousSessionWhenDBEmpty(t *testing.T) {
	vpHTML := `<!DOCTYPE html><html><body>
<p>Bill 5 carried on the following division:</p>
<table class="division">
<tr><td class="head" colspan="4">Yeas &#8212; 4</td></tr>
<tr><td>Alpha <br>Beta <br></td><td>Gamma <br>Delta <br></td><td></td><td></td></tr>
<tr><td class="head" colspan="4">Nays &#8212; 2</td></tr>
<tr><td>Zeta <br></td><td>Eta <br></td><td></td><td></td></tr>
</table></body></html>`

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/bills", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
		  <h2>5th Legislature 2nd Session</h2>
		</body></html>`))
	})

	voteCalls := 0
	mux.HandleFunc("/votes", func(w http.ResponseWriter, _ *http.Request) {
		voteCalls++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if voteCalls <= 1 {
			_, _ = w.Write([]byte(`<html><body><p>No recorded divisions.</p></body></html>`))
		} else {
			_, _ = w.Write([]byte(vpHTML))
		}
	})

	conn := newProvinceDB(t)
	src := ProvincialSource{
		Code:     "nb",
		Province: "New Brunswick",
		Chamber:  "new_brunswick",
		BillsURL: srv.URL + "/bills",
		VotesURL: srv.URL + "/votes",
	}

	if err := CrawlProvinceSource(conn, srv.Client(), noDelay, src, nil, nil); err != nil {
		t.Fatalf("CrawlProvinceSource: %v", err)
	}

	var divCount int
	if err := conn.QueryRow(`SELECT COUNT(1) FROM divisions WHERE id LIKE 'nb-%'`).Scan(&divCount); err != nil {
		t.Fatalf("division count query: %v", err)
	}
	if divCount == 0 {
		t.Fatal("expected at least one NB division from the previous-session fallback, got 0")
	}
}

func TestParliamentOrdinal(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "1st"},
		{2, "2nd"},
		{3, "3rd"},
		{4, "4th"},
		{11, "11th"},
		{12, "12th"},
		{13, "13th"},
		{21, "21st"},
		{43, "43rd"},
	}
	for _, c := range cases {
		got := ParliamentOrdinalForTest(c.n)
		if got != c.want {
			t.Errorf("parliamentOrdinal(%d)=%q, want %q", c.n, got, c.want)
		}
	}
}
