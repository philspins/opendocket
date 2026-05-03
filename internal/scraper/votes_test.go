package scraper_test

import (
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"testing"

	"github.com/philspins/opendocket/internal/scraper"
)

// ── CrawlVotesIndex ───────────────────────────────────────────────────────────

// sampleVotesHTML mirrors the actual ourcommons.ca recorded-votes table structure:
// Col 0: vote number | Col 1: bill type (optional) | Col 2: description
// Col 3: "Yeas / Nays / Paired" | Col 4: result (with icon) | Col 5: date
const sampleVotesHTML = `<html><body>
  <table class="table">
    <thead><tr>
      <th>#</th><th>Vote type</th><th>Description</th>
      <th>Votes</th><th>Result</th><th>Date</th>
    </tr></thead>
    <tbody>
      <tr>
        <td><a href="/Members/en/votes/45/1/892">No. 892</a></td>
        <td>House Government Bill</td>
        <td>Motion on C-47</td>
        <td>172 / 148 / 5</td>
        <td><i class="icon"></i> Agreed to</td>
        <td>Wednesday, April 3, 2024</td>
      </tr>
      <tr>
        <td><a href="/Members/en/votes/45/1/891">No. 891</a></td>
        <td></td>
        <td>Motion on S-209</td>
        <td>100 / 90 / 0</td>
        <td><i class="icon"></i> Negatived</td>
        <td>Tuesday, April 2, 2024</td>
      </tr>
    </tbody>
  </table>
</body></html>`

func TestCrawlVotesIndex_ParsesRows(t *testing.T) {
	srv := newTestServer(sampleVotesHTML)
	defer srv.Close()

	divs, err := scraper.CrawlVotesIndex(srv.URL, 45, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlVotesIndex: %v", err)
	}
	if len(divs) != 2 {
		t.Errorf("len=%d, want 2", len(divs))
	}
}

func TestCrawlVotesIndex_ParsesFirstDivision(t *testing.T) {
	srv := newTestServer(sampleVotesHTML)
	defer srv.Close()

	divs, _ := scraper.CrawlVotesIndex(srv.URL, 45, 1, srv.Client())
	d := divs[0]
	if d.ID != "45-1-892" {
		t.Errorf("ID=%q want 45-1-892", d.ID)
	}
	if d.Number != 892 {
		t.Errorf("Number=%d want 892", d.Number)
	}
	if d.Yeas != 172 || d.Nays != 148 {
		t.Errorf("Yeas=%d Nays=%d want 172/148", d.Yeas, d.Nays)
	}
	if d.Paired != 5 {
		t.Errorf("Paired=%d want 5", d.Paired)
	}
	if d.Result != "Agreed to" {
		t.Errorf("Result=%q want Agreed to", d.Result)
	}
	if d.Date != "2024-04-03" {
		t.Errorf("Date=%q want 2024-04-03", d.Date)
	}
	if d.Chamber != "commons" {
		t.Errorf("Chamber=%q want commons", d.Chamber)
	}
}

func TestCrawlVotesIndex_ExtractsBillNumberFromDescription(t *testing.T) {
	srv := newTestServer(sampleVotesHTML)
	defer srv.Close()

	divs, _ := scraper.CrawlVotesIndex(srv.URL, 45, 1, srv.Client())
	// First row: description "Motion on C-47" → BillNumber should be "C-47"
	if divs[0].BillNumber != "C-47" {
		t.Errorf("divs[0].BillNumber=%q want C-47", divs[0].BillNumber)
	}
	// Second row: description "Motion on S-209" → BillNumber should be "S-209"
	if divs[1].BillNumber != "S-209" {
		t.Errorf("divs[1].BillNumber=%q want S-209", divs[1].BillNumber)
	}
}

func TestCrawlVotesIndex_ErrorOnBadServer(t *testing.T) {
	_, err := scraper.CrawlVotesIndex("http://localhost:0/no-server", 45, 1, nil)
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestCrawlVotesIndex_ErrorWhenNoTable(t *testing.T) {
	srv := newTestServer("<html><body><p>No table</p></body></html>")
	defer srv.Close()

	_, err := scraper.CrawlVotesIndex(srv.URL, 45, 1, srv.Client())
	if err == nil {
		t.Error("expected error when no table found")
	}
}

// ── CrawlDivisionDetail ───────────────────────────────────────────────────────

// sampleDivisionHTML reflects the current ourcommons.ca table layout
// (45th Parliament onwards): a single ce-mip-table-mobile table with four
// columns (Member link, Party, Vote, Paired).
const sampleDivisionHTML = `<html><body>
  <table class="table table-striped ce-mip-table-mobile">
    <thead>
      <tr><th>Member</th><th>Party</th><th>Member Voted</th><th>Paired</th></tr>
    </thead>
    <tbody>
      <tr>
        <td><a href="/members/en/111">Alice Smith</a></td>
        <td>Liberal</td>
        <td>Yea</td>
        <td></td>
      </tr>
      <tr>
        <td><a href="/members/en/222">Bob Jones</a></td>
        <td>Conservative</td>
        <td>Yea</td>
        <td></td>
      </tr>
      <tr>
        <td><a href="/members/en/333">Carol Brown</a></td>
        <td>NDP</td>
        <td>Nay</td>
        <td></td>
      </tr>
      <tr>
        <td><a href="/members/en/444">David White</a></td>
        <td>Bloc</td>
        <td>Paired</td>
        <td>&#x2713;</td>
      </tr>
    </tbody>
  </table>
</body></html>`

// sampleDivisionHTMLLegacy reflects the old ourcommons.ca layout used in
// prior parliaments; it exercises the fallback selector path.
const sampleDivisionHTMLLegacy = `<html><body>
  <div class="vote-yea">
    <ul>
      <li class="member-name"><a href="/Members/en/111">Alice Smith</a></li>
      <li class="member-name"><a href="/Members/en/222">Bob Jones</a></li>
    </ul>
  </div>
  <div class="vote-nay">
    <ul>
      <li class="member-name"><a href="/Members/en/333">Carol Brown</a></li>
    </ul>
  </div>
</body></html>`

func TestCrawlDivisionDetail_ParsesYeaVotes(t *testing.T) {
	srv := newTestServer(sampleDivisionHTML)
	defer srv.Close()

	votes, err := scraper.CrawlDivisionDetail("45-1-892", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("CrawlDivisionDetail: %v", err)
	}
	var yeas []scraper.MemberVote
	for _, v := range votes {
		if v.Vote == "Yea" {
			yeas = append(yeas, v)
		}
	}
	if len(yeas) != 2 {
		t.Errorf("len(yeas)=%d want 2", len(yeas))
	}
}

func TestCrawlDivisionDetail_ParsesNayVotes(t *testing.T) {
	srv := newTestServer(sampleDivisionHTML)
	defer srv.Close()

	votes, _ := scraper.CrawlDivisionDetail("45-1-892", srv.URL, srv.Client())
	var nays []scraper.MemberVote
	for _, v := range votes {
		if v.Vote == "Nay" {
			nays = append(nays, v)
		}
	}
	if len(nays) != 1 || nays[0].MemberID != "333" {
		t.Errorf("nay member_id mismatch: %+v", nays)
	}
}

func TestCrawlDivisionDetail_ParsesPairedVotes(t *testing.T) {
	srv := newTestServer(sampleDivisionHTML)
	defer srv.Close()

	votes, _ := scraper.CrawlDivisionDetail("45-1-892", srv.URL, srv.Client())
	var paired []scraper.MemberVote
	for _, v := range votes {
		if v.Vote == "Paired" {
			paired = append(paired, v)
		}
	}
	if len(paired) != 1 || paired[0].MemberID != "444" {
		t.Errorf("paired member_id mismatch: %+v", paired)
	}
}

func TestCrawlDivisionDetail_AllHaveDivisionID(t *testing.T) {
	srv := newTestServer(sampleDivisionHTML)
	defer srv.Close()

	votes, _ := scraper.CrawlDivisionDetail("45-1-892", srv.URL, srv.Client())
	for _, v := range votes {
		if v.DivisionID != "45-1-892" {
			t.Errorf("DivisionID=%q want 45-1-892", v.DivisionID)
		}
	}
}

func TestCrawlDivisionDetail_LegacyFallback(t *testing.T) {
	srv := newTestServer(sampleDivisionHTMLLegacy)
	defer srv.Close()

	votes, err := scraper.CrawlDivisionDetail("45-1-892", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("CrawlDivisionDetail legacy: %v", err)
	}
	var yeas, nays int
	for _, v := range votes {
		switch v.Vote {
		case "Yea":
			yeas++
		case "Nay":
			nays++
		}
	}
	if yeas != 2 {
		t.Errorf("legacy yeas=%d want 2", yeas)
	}
	if nays != 1 {
		t.Errorf("legacy nays=%d want 1", nays)
	}
}

func TestCrawlDivisionDetail_UsesCSVEndpoint(t *testing.T) {
	// CSV uses Person ID (numeric) in col 0 — not a URL.
	const divCSV = `Person ID,Member of Parliament,Political Affiliation,Member Voted,Paired
10001,Alice Smith (Foo),Liberal,Yea,
10002,Bob Jones (Bar),Conservative,Nay,
10003,Carol Brown (Baz),NDP,Yea,
10004,David White (Qux),Bloc,,paired
`
	emptyHTML := `<html><body>
  <table class="table table-striped ce-mip-table-mobile"><tbody></tbody></table>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/division/csv":
			w.Header().Set("Content-Type", "text/csv")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(divCSV))
		default:
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(emptyHTML))
		}
	}))
	defer srv.Close()

	votes, err := scraper.CrawlDivisionDetail("45-1-200", srv.URL+"/division", srv.Client())
	if err != nil {
		t.Fatalf("CrawlDivisionDetail: %v", err)
	}
	if len(votes) != 4 {
		t.Fatalf("len(votes)=%d want 4", len(votes))
	}
	byID := map[string]string{}
	for _, v := range votes {
		byID[v.MemberID] = v.Vote
		if v.DivisionID != "45-1-200" {
			t.Errorf("DivisionID=%q want 45-1-200", v.DivisionID)
		}
	}
	if byID["10001"] != "Yea" {
		t.Errorf("10001 vote=%q want Yea", byID["10001"])
	}
	if byID["10002"] != "Nay" {
		t.Errorf("10002 vote=%q want Nay", byID["10002"])
	}
	if byID["10003"] != "Yea" {
		t.Errorf("10003 vote=%q want Yea", byID["10003"])
	}
	if byID["10004"] != "Paired" {
		t.Errorf("10004 vote=%q want Paired", byID["10004"])
	}
}

func TestCrawlDivisionDetail_FollowsMotionTextJournalsLink(t *testing.T) {
	divisionHTML := `<html><body>
<a href="/documentviewer/en/house/latest-sitting">House Publications</a>
<h2>Motion Text</h2>
<p>See the published vote in the <a href="/DocumentViewer/en/14032878#DOC--14035119">Journals of Wednesday, April 22, 2026</a></p>
<table class="table table-striped ce-mip-table-mobile"><tbody></tbody></table>
</body></html>`

	// Real-style journal HTML: only surnames, with riding disambiguation for duplicates.
	// Two MPs with surname "Jones": Jones (Riding A) and Jones (Riding B).
	journalHTML := `<html><body>
<a name="DOC--14035119"></a>
<table>
  <tr><td class="DivisionType"><p class="DivisionType">YEAS -- POUR</p>
    <span class="DivisionItem">Smith</span>
    <span class="DivisionItem">Jones (Riding A)</span></td></tr>
  <tr><td class="DivisionType"><p class="DivisionType">NAYS -- CONTRE</p>
    <span class="DivisionItem">Brown</span>
    <span class="DivisionItem">Jones (Riding B)</span></td></tr>
  <tr><td class="DivisionType"><p class="DivisionType">PAIRED -- PAIRES</p>
    <span class="DivisionItem">White</span></td></tr>
</table>
</body></html>`

	// Member tiles HTML served at /Members/en/search.  Two MPs named "Jones"
	// with different riding constituencies to test disambiguation.
	memberTilesHTML := `<html><body>
<div id="mp-tile-person-id-1001">
  <a class="ce-mip-mp-tile-link" href="/Members/en/alice-smith(1001)"></a>
  <div class="ce-mip-mp-tile">
    <div class="ce-mip-mp-name">Alice Smith</div>
    <div class="ce-mip-mp-constituency">Smith Riding</div>
  </div>
</div>
<div id="mp-tile-person-id-1002">
  <a class="ce-mip-mp-tile-link" href="/Members/en/bob-jones(1002)"></a>
  <div class="ce-mip-mp-tile">
    <div class="ce-mip-mp-name">Bob Jones</div>
    <div class="ce-mip-mp-constituency">Riding A</div>
  </div>
</div>
<div id="mp-tile-person-id-1003">
  <a class="ce-mip-mp-tile-link" href="/Members/en/carol-brown(1003)"></a>
  <div class="ce-mip-mp-tile">
    <div class="ce-mip-mp-name">Carol Brown</div>
    <div class="ce-mip-mp-constituency">Brown Riding</div>
  </div>
</div>
<div id="mp-tile-person-id-1004">
  <a class="ce-mip-mp-tile-link" href="/Members/en/david-white(1004)"></a>
  <div class="ce-mip-mp-tile">
    <div class="ce-mip-mp-name">David White</div>
    <div class="ce-mip-mp-constituency">White Riding</div>
  </div>
</div>
<div id="mp-tile-person-id-1005">
  <a class="ce-mip-mp-tile-link" href="/Members/en/eve-jones(1005)"></a>
  <div class="ce-mip-mp-tile">
    <div class="ce-mip-mp-name">Eve Jones</div>
    <div class="ce-mip-mp-constituency">Riding B</div>
  </div>
</div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/division":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(divisionHTML))
		case "/DocumentViewer/en/14032878":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(journalHTML))
		case "/Members/en/search":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(memberTilesHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Use a custom transport that routes ALL requests (including to ourcommons.ca)
	// to the test server, so the member-index fetch is intercepted.
	srvURL, _ := neturl.Parse(srv.URL)
	client := &http.Client{
		Transport: &redirectTransport{host: srvURL.Host},
	}

	votes, err := scraper.CrawlDivisionDetail("45-1-100", srv.URL+"/division", client)
	if err != nil {
		t.Fatalf("CrawlDivisionDetail: %v", err)
	}
	if len(votes) != 5 {
		t.Fatalf("len(votes)=%d want 5", len(votes))
	}

	byID := map[string]string{}   // memberID → vote
	byName := map[string]string{} // memberName → vote
	for _, v := range votes {
		if v.MemberID != "" {
			byID[v.MemberID] = v.Vote
		}
		byName[v.MemberName] = v.Vote
	}
	if byID["1001"] != "Yea" {
		t.Errorf("Smith (1001) vote=%q want Yea", byID["1001"])
	}
	if byID["1002"] != "Yea" {
		t.Errorf("Jones/Riding A (1002) vote=%q want Yea", byID["1002"])
	}
	if byID["1003"] != "Nay" {
		t.Errorf("Brown (1003) vote=%q want Nay", byID["1003"])
	}
	if byID["1005"] != "Nay" {
		t.Errorf("Jones/Riding B (1005) vote=%q want Nay", byID["1005"])
	}
	if byID["1004"] != "Paired" {
		t.Errorf("White (1004) vote=%q want Paired", byID["1004"])
	}
}

func TestCrawlDivisionDetail_SeeListUnderDivision(t *testing.T) {
	// Division 19 has no own member list; its journal section says
	// "(SEE LIST UNDER DIVISION NO. 15)". The scraper must locate Division 15's
	// vote table earlier on the same journal page.
	divisionHTML := `<html><body>
<h2>Motion Text</h2>
<p>See the published vote in the <a href="/DocumentViewer/en/sitting17#DOC--99919">Journals of Tuesday, June 17, 2025</a></p>
<table class="table table-striped ce-mip-table-mobile"><tbody></tbody></table>
</body></html>`

	journalHTML := `<html><body>
<p>(Division No. 15 -- Vote no 15)</p>
<table>
  <tr><td class="DivisionType"><p class="DivisionType">YEAS -- POUR</p>
    <span class="DivisionItem">Smith</span>
    <span class="DivisionItem">Brown</span></td></tr>
  <tr><td class="DivisionType"><p class="DivisionType">NAYS -- CONTRE</p>
    <span class="DivisionItem">Jones</span></td></tr>
</table>
<a name="DOC--99919"></a>
<p>(Division No. 19 -- Vote no 19)</p>
<p>YEAS: 2, NAYS: 1</p>
<p>(SEE LIST UNDER DIVISION NO. 15)</p>
</body></html>`

	memberTilesHTML := `<html><body>
<div id="mp-tile-person-id-2001">
  <a class="ce-mip-mp-tile-link" href="/Members/en/alice-smith(2001)"></a>
  <div class="ce-mip-mp-constituency">Smith Riding</div>
</div>
<div id="mp-tile-person-id-2002">
  <a class="ce-mip-mp-tile-link" href="/Members/en/carol-brown(2002)"></a>
  <div class="ce-mip-mp-constituency">Brown Riding</div>
</div>
<div id="mp-tile-person-id-2003">
  <a class="ce-mip-mp-tile-link" href="/Members/en/bob-jones(2003)"></a>
  <div class="ce-mip-mp-constituency">Jones Riding</div>
</div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/division":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(divisionHTML))
		case "/DocumentViewer/en/sitting17":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(journalHTML))
		case "/Members/en/search":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(memberTilesHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	srvURL, _ := neturl.Parse(srv.URL)
	client := &http.Client{Transport: &redirectTransport{host: srvURL.Host}}

	votes, err := scraper.CrawlDivisionDetail("45-1-19", srv.URL+"/division", client)
	if err != nil {
		t.Fatalf("CrawlDivisionDetail: %v", err)
	}
	if len(votes) != 3 {
		t.Fatalf("len(votes)=%d want 3 (Smith+Brown yea, Jones nay)", len(votes))
	}
	byID := map[string]string{}
	for _, v := range votes {
		if v.MemberID != "" {
			byID[v.MemberID] = v.Vote
		}
	}
	if byID["2001"] != "Yea" {
		t.Errorf("Smith (2001) vote=%q want Yea", byID["2001"])
	}
	if byID["2002"] != "Yea" {
		t.Errorf("Brown (2002) vote=%q want Yea", byID["2002"])
	}
	if byID["2003"] != "Nay" {
		t.Errorf("Jones (2003) vote=%q want Nay", byID["2003"])
	}
}

// redirectTransport is an http.RoundTripper that routes all requests to the
// specified host, regardless of the original URL. Used in tests to intercept
// calls to external hosts (e.g. ourcommons.ca) and serve them from a local
// httptest.Server.
type redirectTransport struct{ host string }

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	r2.URL.Host = rt.host
	r2.URL.Scheme = "http"
	return http.DefaultTransport.RoundTrip(r2)
}

// ── CrawlSittingCalendar ──────────────────────────────────────────────────────

const sampleCalendarHTML = `<html><body>
  <table>
    <tbody>
      <tr>
        <td class="sitting" data-date="2024-04-03">3</td>
        <td class="sitting" data-date="2024-04-04">4</td>
        <td data-date="2024-04-06">6</td>
      </tr>
    </tbody>
  </table>
</body></html>`

const sampleCommonsCalendarHTML = `<html><body>
	<table class="chamber-calendar">
		<tbody>
			<tr>
				<td valign="top" class="2026-04-22 chamber-meeting">22</td>
				<td valign="top" class="2026-04-23 chamber-meeting">23</td>
				<td valign="top" class="adjournment-tabling">24</td>
			</tr>
		</tbody>
	</table>
</body></html>`

func TestCrawlSittingCalendar_ParsesDates(t *testing.T) {
	srv := newTestServer(sampleCalendarHTML)
	defer srv.Close()

	dates, err := scraper.CrawlSittingCalendar(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("CrawlSittingCalendar: %v", err)
	}
	found := make(map[string]bool)
	for _, d := range dates {
		found[d] = true
	}
	if !found["2024-04-03"] || !found["2024-04-04"] {
		t.Errorf("expected 2024-04-03 and 2024-04-04, got %v", dates)
	}
}

func TestCrawlSittingCalendar_Sorted(t *testing.T) {
	srv := newTestServer(sampleCalendarHTML)
	defer srv.Close()

	dates, _ := scraper.CrawlSittingCalendar(srv.URL, srv.Client())
	for i := 1; i < len(dates); i++ {
		if dates[i] < dates[i-1] {
			t.Errorf("dates not sorted: %v", dates)
		}
	}
}

func TestCrawlSittingCalendar_ParsesCommonsChamberMeetingClasses(t *testing.T) {
	srv := newTestServer(sampleCommonsCalendarHTML)
	defer srv.Close()

	dates, err := scraper.CrawlSittingCalendar(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("CrawlSittingCalendar: %v", err)
	}
	found := make(map[string]bool)
	for _, d := range dates {
		found[d] = true
	}
	if !found["2026-04-22"] || !found["2026-04-23"] {
		t.Fatalf("expected chamber-meeting dates, got %v", dates)
	}
	if found["2026-04-24"] {
		t.Fatalf("did not expect adjournment date in sitting dates: %v", dates)
	}
}

// ── ParliamentIsSitting ───────────────────────────────────────────────────────

func TestParliamentIsSitting_TrueWhenInList(t *testing.T) {
	dates := []string{"2024-04-03", "2024-04-04"}
	if !scraper.ParliamentIsSitting(dates, "2024-04-03") {
		t.Error("expected true")
	}
}

func TestParliamentIsSitting_FalseWhenNotInList(t *testing.T) {
	dates := []string{"2024-04-03", "2024-04-04"}
	if scraper.ParliamentIsSitting(dates, "2024-04-05") {
		t.Error("expected false")
	}
}

func TestParliamentIsSitting_FalseForEmpty(t *testing.T) {
	if scraper.ParliamentIsSitting(nil, "2024-04-03") {
		t.Error("expected false for empty list")
	}
}
