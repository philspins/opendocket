package provincial

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/philspins/opendocket/internal/store"
)

func TestSplitCalendarDayToken(t *testing.T) {
	got := splitCalendarDayToken("91011")
	want := []int{9, 10, 11}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitCalendarDayToken()=%v want %v", got, want)
		}
	}
}

func TestCrawlPrinceEdwardIslandBills_UsesWorkflowAPI(t *testing.T) {
	const billsJSON = `{"processInstanceId":"t1","messages":{"error":[]},"data":[{"id":"1","type":"TableV2","data":{},"children":[{"id":"2","type":"TableV2Row","data":{},"children":[{"id":"3","type":"TableV2Cell","data":{},"children":[{"id":"4","type":"LinkV2","data":{"text":"Bill 1 - An Act to Amend the Highway Traffic Act","routerLink":"../LegislativeAssemblyBillView","queryParams":{"id":"bill-doc-1"}},"children":[]}]},{"id":"5","type":"TableV2Cell","data":{"text":"1"},"children":[]},{"id":"6","type":"TableV2Cell","data":{"text":"First Reading"},"children":[]},{"id":"7","type":"TableV2Cell","data":{"text":"March 15, 2026"},"children":[]}]}]}]}`
	const detailJSON = `{"processInstanceId":"t2","messages":{"error":[]},"data":[{"id":"10","type":"Heading","data":{"text":"Bill no. 1 - An Act to Amend the Highway Traffic Act","size":2},"children":[]},{"id":"11","type":"TableV2","data":{},"children":[{"id":"12","type":"TableV2Row","data":{},"children":[{"id":"13","type":"TableV2Header","data":{"text":"Read Original Bill Text* (PDF)"},"children":[]},{"id":"14","type":"TableV2Cell","data":{"text":null},"children":[{"id":"15","type":"LinkV2","data":{"text":"An Act to Amend the Highway Traffic Act","href":"https://docs.assembly.pe.ca/download/dms?objectId=bill-doc-1&fileName=bill-1.pdf"},"children":[]}]}]}]}]}`

	mux := http.NewServeMux()
	mux.HandleFunc("/legislative-assembly/services/api/workflow", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		var req struct {
			QueryName string            `json:"queryName"`
			QueryVars map[string]string `json:"queryVars"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if req.QueryName == "LegislativeAssemblyBillView" && req.QueryVars["id"] == "bill-doc-1" {
			w.Write([]byte(detailJSON))
			return
		}
		if req.QueryVars["search"] != "assembly" {
			t.Fatalf("search=%q, want assembly", req.QueryVars["search"])
		}
		if req.QueryVars["general_assembly"] != "68" || req.QueryVars["session"] != "1" {
			t.Fatalf("assembly/session=(%q,%q), want (68,1)", req.QueryVars["general_assembly"], req.QueryVars["session"])
		}
		w.Write([]byte(billsJSON))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bills, err := CrawlPrinceEdwardIslandBills(srv.URL, 68, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlPrinceEdwardIslandBills: %v", err)
	}
	if len(bills) == 0 {
		t.Fatal("expected at least one bill from WDF API")
	}
	if bills[0].Number != "1" {
		t.Errorf("bill number: got %q, want %q", bills[0].Number, "1")
	}
	if bills[0].ProvinceCode != "pe" {
		t.Errorf("province: got %q, want %q", bills[0].ProvinceCode, "pe")
	}
	if bills[0].DetailURL != "https://docs.assembly.pe.ca/download/dms?objectId=bill-doc-1&fileName=bill-1.pdf" {
		t.Errorf("detail url: got %q", bills[0].DetailURL)
	}
}

func TestCrawlPrinceEdwardIslandBills_FallsBackOnWorkflowNon200(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/legislative-assembly/services/api/workflow", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><a href="/bills/5">Bill 5 - A Test Act</a></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bills, err := CrawlPrinceEdwardIslandBills(srv.URL, 68, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlPrinceEdwardIslandBills: %v", err)
	}
	if len(bills) == 0 {
		t.Fatal("expected bills from HTML fallback when WDF API returns non-200")
	}
}

func TestCrawlPrinceEdwardIslandVotes_UsesWorkflowAPI(t *testing.T) {
	const journalHTML = `<html><body>
<h3>Bill 1 second reading</h3>
<table><tr><td>Yeas: 15</td><td>Nays: 7</td></tr></table>
</body></html>`

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	journalsJSON := `{"processInstanceId":"t2","messages":{"error":[]},"data":[{"id":"1","type":"TableV2","data":{},"children":[{"id":"2","type":"TableV2Row","data":{},"children":[{"id":"3","type":"TableV2Cell","data":{},"children":[{"id":"4","type":"LinkV2","data":{"text":"Journal April 7 2026","href":"` + srv.URL + `/journals/2026-04-07"},"children":[]}]}]}]}]}`

	mux.HandleFunc("/legislative-assembly/services/api/workflow", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		var req struct {
			QueryVars map[string]string `json:"queryVars"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.QueryVars["search"] != "assembly" {
			t.Fatalf("search=%q, want assembly", req.QueryVars["search"])
		}
		if req.QueryVars["general_assembly"] != "68" || req.QueryVars["session"] != "1" {
			t.Fatalf("assembly/session=(%q,%q), want (68,1)", req.QueryVars["general_assembly"], req.QueryVars["session"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(journalsJSON))
	})
	mux.HandleFunc("/journals/2026-04-07", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(journalHTML))
	})

	divs, err := CrawlPrinceEdwardIslandVotes(srv.URL, 68, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlPrinceEdwardIslandVotes: %v", err)
	}
	if len(divs) == 0 {
		t.Fatal("expected at least one division from WDF journals API")
	}
	if divs[0].Division.Yeas != 15 || divs[0].Division.Nays != 7 {
		t.Errorf("counts=(%d,%d), want (15,7)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
}

func TestCrawlPrinceEdwardIslandVotes_FallsBackOnWorkflowNon200(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/legislative-assembly/services/api/workflow", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><a href="/votes/2026-04-07">Votes and Proceedings</a></body></html>`))
	})
	mux.HandleFunc("/votes/2026-04-07", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><table><tr><td>Yeas: 9</td><td>Nays: 2</td></tr></table></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	divs, err := CrawlPrinceEdwardIslandVotes(srv.URL, 68, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlPrinceEdwardIslandVotes: %v", err)
	}
	if len(divs) == 0 {
		t.Fatal("expected divisions from HTML fallback when WDF API returns non-200")
	}
}

func TestCrawlPrinceEdwardIslandVotes_HandlesCaptcha(t *testing.T) {
	const peiLegislature = 68
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><link rel="stylesheet" href="https://captcha.perfdrive.com/challenge.css"></body></html>`))
	}))
	defer srv.Close()

	divs, err := CrawlPrinceEdwardIslandVotes(srv.URL, peiLegislature, 1, srv.Client())
	if err != nil {
		t.Fatalf("expected no error on CAPTCHA, got: %v", err)
	}
	if len(divs) != 0 {
		t.Fatalf("expected 0 divisions on CAPTCHA, got %d", len(divs))
	}
}

func TestCrawlProvinceSource_PEICrawlsAllSessionsInCurrentLegislature(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/bills", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><h2>67th General Assembly 3rd Session</h2></body></html>`))
	})
	mux.HandleFunc("/journals", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><h2>67th General Assembly 3rd Session</h2></body></html>`))
	})
	workflowHandler := func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			QueryName string            `json:"queryName"`
			QueryVars map[string]string `json:"queryVars"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		session := req.QueryVars["session"]
		if req.QueryName != "LegislativeAssemblyBillView" {
			if req.QueryVars["search"] != "assembly" {
				t.Fatalf("search=%q, want assembly", req.QueryVars["search"])
			}
			if req.QueryVars["general_assembly"] != "67" {
				t.Fatalf("general_assembly=%q, want 67", req.QueryVars["general_assembly"])
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.QueryName {
		case "LegislativeAssemblyBillSearch":
			_, _ = fmt.Fprintf(w, `{"processInstanceId":"b%s","messages":{"error":[]},"data":[{"id":"1","type":"TableV2","data":{},"children":[{"id":"2","type":"TableV2Row","data":{},"children":[{"id":"3","type":"TableV2Cell","data":{},"children":[{"id":"4","type":"LinkV2","data":{"text":"Bill %s - Session %s Act","routerLink":"../LegislativeAssemblyBillView","queryParams":{"id":"bill-doc-%s"}},"children":[]}]},{"id":"5","type":"TableV2Cell","data":{"text":"%s"},"children":[]},{"id":"6","type":"TableV2Cell","data":{"text":"First Reading"},"children":[]},{"id":"7","type":"TableV2Cell","data":{"text":"March 15, 2026"},"children":[]}]}]}]}`, session, session, session, session, session)
		case "LegislativeAssemblyBillView":
			billDocID := req.QueryVars["id"]
			_, _ = fmt.Fprintf(w, `{"processInstanceId":"d%s","messages":{"error":[]},"data":[{"id":"10","type":"TableV2","data":{},"children":[{"id":"11","type":"TableV2Row","data":{},"children":[{"id":"12","type":"TableV2Header","data":{"text":"Read Original Bill Text* (PDF)"},"children":[]},{"id":"13","type":"TableV2Cell","data":{"text":null},"children":[{"id":"14","type":"LinkV2","data":{"text":"Bill Text","href":"https://docs.assembly.pe.ca/download/dms?objectId=%s&fileName=%s.pdf"},"children":[]}]}]}]}]}`, billDocID, billDocID, strings.TrimPrefix(billDocID, "bill-doc-"))
		case "LegislativeAssemblyJournalsSearch":
			_, _ = fmt.Fprintf(w, `{"processInstanceId":"j%s","messages":{"error":[]},"data":[{"id":"1","type":"TableV2","data":{},"children":[{"id":"2","type":"TableV2Row","data":{},"children":[{"id":"3","type":"TableV2Cell","data":{},"children":[{"id":"4","type":"LinkV2","data":{"text":"Journal Session %s","href":"%s/journals/session-%s"},"children":[]}]}]}]}]}`, session, session, srv.URL, session)
		default:
			http.Error(w, "unexpected query", http.StatusBadRequest)
		}
	}
	mux.HandleFunc("/bills/legislative-assembly/services/api/workflow", workflowHandler)
	mux.HandleFunc("/journals/legislative-assembly/services/api/workflow", workflowHandler)
	for session := 1; session <= 3; session++ {
		session := session
		mux.HandleFunc(fmt.Sprintf("/journals/session-%d", session), func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html><html><body>
<p>Bill %d carried on the following division:</p>
<table class="division">
<tr><td class="head" colspan="4">Yeas &#8212; 3</td></tr>
<tr><td>Alpha <br>Beta <br></td><td>Gamma <br></td><td></td><td></td></tr>
<tr><td class="head" colspan="4">Nays &#8212; 1</td></tr>
<tr><td>Delta <br></td><td></td><td></td><td></td></tr>
</table></body></html>`, session)))
		})
	}

	conn := newProvinceDB(t)
	for i := 0; i < 10; i++ {
		if err := store.UpsertMember(conn, store.MemberRecord{
			ID:              fmt.Sprintf("pe-member-%d", i),
			Name:            fmt.Sprintf("Member %d", i),
			Province:        "Prince Edward Island",
			Active:          true,
			GovernmentLevel: "provincial",
		}); err != nil {
			t.Fatalf("UpsertMember(%d): %v", i, err)
		}
	}

	enqueued := make(map[string]bool)
	enqueue := func(billID, _, _, _ string) {
		enqueued[billID] = true
	}
	src := ProvincialSource{
		Code:     "pe",
		Province: "Prince Edward Island",
		Chamber:  "pei",
		BillsURL: srv.URL + "/bills",
		VotesURL: srv.URL + "/journals",
	}

	if err := CrawlProvinceSource(conn, srv.Client(), noDelay, src, enqueue, nil); err != nil {
		t.Fatalf("CrawlProvinceSource: %v", err)
	}

	for session := 1; session <= 3; session++ {
		billID := fmt.Sprintf("pe-67-%d-%d", session, session)
		var billCount int
		if err := conn.QueryRow(`SELECT COUNT(1) FROM bills WHERE id = ?`, billID).Scan(&billCount); err != nil {
			t.Fatalf("bill count query for %s: %v", billID, err)
		}
		if billCount != 1 {
			t.Fatalf("expected bill %s to be stored once, got %d", billID, billCount)
		}
		if !enqueued[billID] {
			t.Fatalf("expected summary enqueue for %s", billID)
		}
	}

	var distinctSessions int
	if err := conn.QueryRow(`SELECT COUNT(DISTINCT session) FROM divisions WHERE id LIKE 'pe-67-%'`).Scan(&distinctSessions); err != nil {
		t.Fatalf("division session count query: %v", err)
	}
	if distinctSessions != 3 {
		t.Fatalf("expected divisions from 3 PEI sessions, got %d", distinctSessions)
	}
}

func TestParsePEIDatesFromCalendarText(t *testing.T) {
	text := `
		Parliamentary Calendar 2026
		Sitting Schedule
		In keeping with the Rules of the Legislative Assembly, the first day of the winter/spring sitting is the fourth Tuesday of February,
		and the first day of the fall sitting is the first Tuesday in November.
		Note on calendar update: The 2nd session of the 67th General Assembly was prorogued February 20, 2026,
		and the opening of the 3rd Session set for 1:00pm, Tuesday, March 24, 2026.
		Legislative Planning Weeks
		one legislative planning week is scheduled for the week prior to the winter/spring sitting and the fall sitting;
		one legislative planning week to coincide with March Break (March 16-20, 2026).
	`
	dates := ParsePEIDatesFromCalendarText(text, 2026)
	if len(dates) == 0 {
		t.Fatalf("expected generated PEI dates")
	}

	contains := func(needle string) bool {
		for _, d := range dates {
			if d == needle {
				return true
			}
		}
		return false
	}

	if !contains("2026-04-22") {
		t.Fatalf("expected 2026-04-22 to be included")
	}
	if contains("2026-03-17") {
		t.Fatalf("did not expect March break date 2026-03-17")
	}
	if contains("2026-03-23") {
		t.Fatalf("did not expect Monday non-sitting date 2026-03-23")
	}
	if !contains("2026-11-03") {
		t.Fatalf("expected fall sitting start 2026-11-03 to be included")
	}
}

func TestParsePEIJournalDivisions_YeasAndNays(t *testing.T) {
	text := `Hon. Premier moved the following Motion. Hon. Mr. Speaker put the Question. ` +
		`A Recorded Division being sought, the names were recorded by the Clerk as follows: ` +
		`Nays (2\ Leader of the Third Party Karla Bernard (Charlottetown - Victoria Park\ Gordon McNeilly (Charlottetown - West Royalty\ ` +
		`Yeas (3\ Hon. Darlene Compton (Land and Environment\ Hon. Premier Hon. Bloyce Thompson (Agriculture, Justice and Public Safety, Attorney General\ ` +
		`The Motion was CARRIED and resolved accordingly.`

	divs := ParsePEIJournalDivisionsForTest(text, "https://docs.assembly.pe.ca/test.pdf", 67, 3, 1, "2026-04-10")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	d := divs[0]
	if d.Division.Nays != 2 || d.Division.Yeas != 3 {
		t.Errorf("counts=(%d,%d), want (2,3)", d.Division.Nays, d.Division.Yeas)
	}
	if d.Division.Result != "Carried" {
		t.Errorf("result=%q, want Carried", d.Division.Result)
	}

	var yeas, nays []string
	for _, v := range d.Votes {
		if v.Vote == "Yea" {
			yeas = append(yeas, v.MemberName)
		} else {
			nays = append(nays, v.MemberName)
		}
	}
	if len(nays) != 2 {
		t.Errorf("nay voters=%v, want 2", nays)
	}
	if len(yeas) != 2 {
		t.Errorf("yea voters=%v, want 2", yeas)
	}
}

func TestParsePEIJournalDivisions_NaysFirst(t *testing.T) {
	text := `A Recorded Division being sought, the names were recorded by the Clerk as follows: ` +
		`Nays (12\ Hon. Darlene Compton (Land and Environment\ Hon. Jill Burridge (Finance and Affordability\ ` +
		`Hon. Bloyce Thompson (Agriculture, Justice and Public Safety, Attorney General\ Hon. Zack Bell (Workforce and Advanced Learning\ ` +
		`Hon. Ernie Hudson (Fisheries, Rural Development and Tourism\ Tyler DesRoches (Summerside - Wilmot\ ` +
		`Hon. Barb Ramsay (Social Development and Seniors\ Hon. Robin Croucher (Education and Early Years\ ` +
		`Hon. Jenn Redmond (Economic Development, Trade and Artificial Intelligence\ Hon. Kent Dollar (Housing and Communities\ ` +
		`Susie Dillon (Charlottetown - Belvedere\ Brendan Curran (Georgetown - Pownal\ ` +
		`Yeas (7\ Leader of the Third Party Karla Bernard (Charlottetown - Victoria Park\ Gordon McNeilly (Charlottetown - West Royalty\ ` +
		`Hon. Leader of the Opposition Peter Bevan - Baker (New Haven - Rocky Point\ ` +
		`Robert Henderson (O'Leary - Inverness\ Carolyn Simpson (Charlottetown - Hillsborough Park\ ` +
		`Motion resolved in the Negative.`

	divs := ParsePEIJournalDivisionsForTest(text, "https://docs.assembly.pe.ca/test.pdf", 67, 3, 1, "2026-04-07")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	d := divs[0]
	if d.Division.Nays != 12 || d.Division.Yeas != 7 {
		t.Errorf("counts=(%d,%d), want (12,7)", d.Division.Nays, d.Division.Yeas)
	}
	if d.Division.Result != "Negatived" {
		t.Errorf("result=%q, want Negatived", d.Division.Result)
	}
	if len(d.Votes) < 5 {
		t.Errorf("too few votes: %d", len(d.Votes))
	}
}

func TestParsePEIJournalDivisions_CountsWithoutParentheses(t *testing.T) {
	text := `Hon. Mr. Speaker put the Question. A Recorded Division being sought, the names were recorded by the Clerk as follows: ` +
		`Nays 12 \ Hon. Darlene Compton Land and Environment\ Hon. Jill Burridge Finance and Affordability\ ` +
		`Yea 7 \ Leader of the Third Party Karla Bernard Charlottetown - Victoria Park\ Gordon McNeilly Charlottetown - West Royalty\ ` +
		`Motion resolved in the Negative.`

	divs := ParsePEIJournalDivisionsForTest(text, "https://docs.assembly.pe.ca/test.pdf", 67, 3, 1, "2026-04-07")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	d := divs[0]
	if d.Division.Nays != 12 || d.Division.Yeas != 7 {
		t.Errorf("counts=(%d,%d), want (12,7)", d.Division.Nays, d.Division.Yeas)
	}
	if d.Division.Result != "Negatived" {
		t.Errorf("result=%q, want Negatived", d.Division.Result)
	}
}

func TestParsePEIJournalDivisions_PrefersMeaningfulContextOverBoilerplate(t *testing.T) {
	meaningful := `Bill 8, An Act Respecting Health Services, after extended debate and detailed clause-by-clause review and further amendment and further debate and additional remarks from members across the chamber, was read the second time`
	text := strings.Join(strings.Fields(meaningful+`. And the question being put on the motion. A Recorded Division being sought, the names were recorded by the Clerk as follows: Yeas 14 \ Member One \ Nays 6 \ Member Two \ Motion resolved in the Negative.`), " ")

	divs := ParsePEIJournalDivisionsForTest(text, "https://docs.assembly.pe.ca/test.pdf", 67, 3, 1, "2026-04-07")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if !strings.Contains(divs[0].Division.Description, "Bill 8") {
		t.Fatalf("expected bill context in description, got %q", divs[0].Division.Description)
	}
	if strings.Contains(strings.ToLower(divs[0].Division.Description), "question being put") {
		t.Fatalf("expected boilerplate to be stripped, got %q", divs[0].Division.Description)
	}
}

func TestIsPEICaptchaBody_CaseInsensitive(t *testing.T) {
	if !isPEICaptchaBody([]byte(`<html><head><link href="HTTPS://CAPTCHA.PERFDRIVE.COM/challenge.css"></head></html>`)) {
		t.Fatal("expected captcha signature to be detected case-insensitively")
	}
	if !isPEICaptchaBody([]byte(`<script src="https://cdn.perfdrive.com/aperture/aperture.js"></script>`)) {
		t.Fatal("expected generic perfdrive bot-manager signature to be detected")
	}
}
