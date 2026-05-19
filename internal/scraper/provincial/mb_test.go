package provincial

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/philspins/opendocket/internal/db"
	"github.com/philspins/opendocket/internal/store"
)

func TestParsePDFDivisionsYeasNays_ManitobaStyle(t *testing.T) {
	text := `VOTES AND PROCEEDINGS 43rd Legislature 3rd Session YEAS - 37 Balser Bailey Bereza Brar Bushie Clarke Cook NAYS - 18 Balcaen Byram Eichler Ewasko Goertzen`
	divs := ParsePDFDivisionsYeasNaysForTest(text, "https://example.com/votes_041.pdf", "mb", "manitoba", 43, 3, 1, "2024-02-20")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if divs[0].Division.Yeas != 37 || divs[0].Division.Nays != 18 {
		t.Fatalf("counts=(%d,%d), want (37,18)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
	if divs[0].Division.Result != "Carried" {
		t.Fatalf("result=%q, want Carried", divs[0].Division.Result)
	}
	if len(divs[0].Votes) < 5 {
		t.Fatalf("len(votes)=%d, want >=5", len(divs[0].Votes))
	}
}

func TestParsePDFDivisionsYeasNays_ManitobaStyleUppercaseNames(t *testing.T) {
	text := `VOTES AND PROCEEDINGS 43rd Legislature 3rd Session YEAS - 37 BALSER BAILEY BEREZA BRAR BUSHIE CLARKE COOK NAYS - 18 BALCAEN BYRAM EICHLER EWASKO GOERTZEN`
	divs := ParsePDFDivisionsYeasNaysForTest(text, "https://example.com/votes_041.pdf", "mb", "manitoba", 43, 3, 1, "2024-02-20")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if len(divs[0].Votes) < 5 {
		t.Fatalf("len(votes)=%d, want >=5", len(divs[0].Votes))
	}
	votesByName := map[string]string{}
	for _, v := range divs[0].Votes {
		votesByName[v.MemberName] = v.Vote
	}
	if got := votesByName["BALSER"]; got != "Yea" {
		t.Fatalf("vote[BALSER]=%q, want Yea", got)
	}
	if got := votesByName["EICHLER"]; got != "Nay" {
		t.Fatalf("vote[EICHLER]=%q, want Nay", got)
	}
}

func TestParseManitobaAyeNayDivisions_CurrentLayout(t *testing.T) {
	text := `Pursuant to sub-rule 24(7), the division on the proposed motion of MLA LAMOUREUX was deferred to take place today at 11:55 a.m. THAT Bill (No. 232) The Autism Strategy Act/Loi sur la strategie sur l'autisme, be now read a Second Time and be referred to a Committee of this House. And the Question being put. It was agreed to, on the following division: AYE BALCAEN BEREZA BLASHKO BRAR BUSHIE BYRAM CABLE COMPTON COOK CORBETT CROSS DELA CRUZ DEVGAN EWASKO GUENTER HIEBERT JOHNSON KENNEDY KHAN KING KOSTYSHYN LAMOUREUX MALOWAY MARCELINO MOROZ MOSES MOYES NARTH NESBITT OXENHAM PERCHOTTE REDHEAD ROBBINS SALA SANDHU SCHMIDT SCHOTT SCHULER SIMARD SMITH STONE WASYLIW WHARTON WIEBE WOWCHUK ..................................... 46 NAY ......................................................... 0 The Bill was accordingly read a Second Time and referred to a Committee of this House.`
	divs := ParseManitobaAyeNayDivisionsForTest(text, "https://example.com/votes_031.pdf", 43, 3, 1, "2026-03-19")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if divs[0].Division.Yeas != 46 || divs[0].Division.Nays != 0 {
		t.Fatalf("counts=(%d,%d), want (46,0)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
	if len(divs[0].Votes) < 10 {
		t.Fatalf("len(votes)=%d, want >=10", len(divs[0].Votes))
	}
	if !strings.Contains(divs[0].Division.Description, "Bill (No. 232)") {
		t.Fatalf("description=%q", divs[0].Division.Description)
	}
	seen := map[string]string{}
	for _, vote := range divs[0].Votes {
		seen[vote.MemberName] = vote.Vote
	}
	if seen["BALCAEN"] != "Yea" || seen["WOWCHUK"] != "Yea" || seen["DELA CRUZ"] != "Yea" {
		t.Fatalf("unexpected parsed votes: BALCAEN=%q WOWCHUK=%q DELA CRUZ=%q", seen["BALCAEN"], seen["WOWCHUK"], seen["DELA CRUZ"])
	}
}

func TestParseManitobaAyeNayDivisions_BillDescriptionAcrossAdjacentDivisions(t *testing.T) {
	// Reproduces the real-world case where two divisions appear on the same PDF page:
	// the second division's AYE marker is preceded by the first division's NAY voter
	// names (filling the 1200-char default window).  The bill description
	// ("THAT Bill (No. 5)...") is more than 1200 chars back from the second AYE,
	// so the wide-context search (3000 chars) must find it.
	// The NAY voter names must NOT appear as the second division's description.
	firstNAYNames := strings.Repeat("BALCAEN BYRAM COOK EWASKO GUENTER HIEBERT JOHNSON KHAN KING NARTH ", 6) // ~600 chars
	text := `THAT Bill (No. 5) – The Accessibility for Manitobans Amendment Act/Loi modifiant la Loi sur l'accessibilite pour les Manitobains, be now read a Third Time and passed. And the Question being put. It was agreed to, on the following division: AYE ASAGWARA BLASHKO BRAR BUSHIE CABLE ..................................... 33 NAY ` +
		firstNAYNames + `....20 And the Question being put on the next motion. It was agreed to, on the following division: AYE SMITH JONES BROWN .....3 NAY WILSON ....1`

	divs := ParseManitobaAyeNayDivisionsForTest(text, "https://example.com/votes_test.pdf", 43, 3, 1, "2026-04-09")
	if len(divs) < 2 {
		t.Fatalf("len(divs)=%d, want >=2", len(divs))
	}
	// First division: THAT + em-dash strips bill-number prefix, leaving act title.
	if !strings.Contains(divs[0].Division.Description, "Accessibility for Manitobans") {
		t.Errorf("div[0].description=%q; expected act title with 'Accessibility for Manitobans'", divs[0].Division.Description)
	}
	// Second division: all-caps NAY voter names must NOT be the description.
	if strings.Contains(divs[1].Division.Description, "BALCAEN") || strings.Contains(divs[1].Division.Description, "EWASKO") {
		t.Errorf("div[1].description=%q; voter names from adjacent division should not appear as description", divs[1].Division.Description)
	}
}

func TestParseManitobaAyeNayDivisions_LiveNoParensBillFormat(t *testing.T) {
	text := `Pursuant to sub-rule 24(7), the division on the proposed motion of MLA LAMOUREUX was deferred to take place today at 11:55 a.m. THAT Bill No. 232 The Autism Strategy Act/Loi sur la strategie sur l'autisme, be now read a Second Time and be referred to a Committee of this House. And the Question being put. It was agreed to, on the following division: AYE BALCAEN BEREZA BLASHKO BRAR BUSHIE BYRAM CABLE COMPTON COOK CORBETT CROSS DELA CRUZ DEVGAN EWASKO GUENTER HIEBERT JOHNSON KENNEDY KHAN KING KOSTYSHYN LAMOUREUX MALOWAY MARCELINO MOROZ MOSES MOYES NARTH NESBITT OXENHAM PERCHOTTE REDHEAD ROBBINS SALA SANDHU SCHMIDT SCHOTT SCHULER SIMARD SMITH STONE WASYLIW WHARTON WIEBE WOWCHUK ..................................... 46 NAY ......................................................... 0`
	divs := ParseManitobaAyeNayDivisionsForTest(text, "https://example.com/votes_031.pdf", 43, 3, 1, "2026-03-19")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if !strings.Contains(divs[0].Division.Description, "Bill No. 232") {
		t.Fatalf("description=%q", divs[0].Division.Description)
	}
	if ExtractProvincialBillNumber(divs[0].Division.Description) != "232" {
		t.Fatalf("bill number=%q", ExtractProvincialBillNumber(divs[0].Division.Description))
	}
}

func TestParseManitobaAyeNayDivisions_BillEmDashFormat(t *testing.T) {
	// Covers three real-world motion formats that lack a "THAT" prefix:
	//  1. Plain "Bill (No. X) – Act Title/Loi..." — keep bill reference in description.
	//  2. "amendment to Bill (No. X) – Act Title/Loi..." — capitalise and keep bill ref.
	//  3. Multiple bills in context — last match (closest to AYE) wins.
	ayeNay := func(yeas, nays int) string {
		yNames := strings.Repeat("SMITH JONES BROWN COOK ADAMS ", yeas/5+1)
		nNames := strings.Repeat("BAKER BYRAM EWASKO GUENTER ", nays/4+1)
		return "And the Question being put. It was agreed to, on the following division: AYE " +
			yNames + ".......... " + strconv.Itoa(yeas) +
			" NAY " + nNames + ".......... " + strconv.Itoa(nays)
	}

	tests := []struct {
		name    string
		text    string
		wantDesc string
	}{
		{
			name: "plain bill em-dash",
			text: "Bill (No. 47) – The Fair Trade in Canada (Internal Trade Mutual Recognition) Act" +
				"/Loi sur le commerce équitable au Canada. " + ayeNay(32, 19),
			wantDesc: "Bill (No. 47) – The Fair Trade in Canada (Internal Trade Mutual Recognition) Act",
		},
		{
			name: "amendment to bill",
			text: "amendment to Bill (No. 48) – The Protective Detention and Care of Intoxicated Persons Act" +
				"/Loi sur la détention des personnes. " + ayeNay(21, 32),
			wantDesc: "Amendment to Bill (No. 48) – The Protective Detention and Care of Intoxicated Persons Act",
		},
		{
			name: "multiple bills — last before AYE wins",
			// Bill (No. 29) appears closer to the AYE block than Bill (No. 47).
			text: "Bill (No. 47) – The Fair Trade Act/Loi whatever. " +
				"Bill (No. 29) – The Workplace Safety and Health Amendment Act/Loi santé. " +
				ayeNay(32, 20),
			wantDesc: "Bill (No. 29) – The Workplace Safety and Health Amendment Act",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			divs := ParseManitobaAyeNayDivisionsForTest(tc.text, "https://example.com/votes_test.pdf", 43, 3, 1, "2026-05-18")
			if len(divs) != 1 {
				t.Fatalf("len(divs)=%d, want 1", len(divs))
			}
			if divs[0].Division.Description != tc.wantDesc {
				t.Errorf("description:\n  got  %q\n  want %q", divs[0].Division.Description, tc.wantDesc)
			}
		})
	}
}

func TestExtractLegislatureSessionCandidates_ManitobaFormats(t *testing.T) {
	tests := []struct {
		text string
		want legislatureSession
	}{
		{
			text: "Current Session: 43 - 3 (2025- )",
			want: legislatureSession{Legislature: 43, Session: 3},
		},
		{
			text: "https://web2.gov.mb.ca/bills/43-3/index.php",
			want: legislatureSession{Legislature: 43, Session: 3},
		},
	}

	for _, tc := range tests {
		candidates := extractLegislatureSessionCandidates("mb", tc.text, 50)
		best, ok := maxLegislatureSession(candidates)
		if !ok {
			t.Fatalf("no candidates for %q", tc.text)
		}
		if best.Legislature != tc.want.Legislature || best.Session != tc.want.Session {
			t.Fatalf("best=%+v, want legislature=%d session=%d", best, tc.want.Legislature, tc.want.Session)
		}
	}
}

func TestMBSessionFromURL(t *testing.T) {
	tests := []struct {
		url      string
		wantLeg  int
		wantSess int
		wantOK   bool
	}{
		{"https://www.gov.mb.ca/legislature/business/43rd/43rd_2nd.html", 43, 2, true},
		{"https://www.gov.mb.ca/legislature/business/43rd/43rd_3rd.html", 43, 3, true},
		{"https://www.gov.mb.ca/legislature/business/42nd/42nd_5th.html", 42, 5, true},
		{"https://example.com/votes_063.pdf", 0, 0, false},
		{"https://example.com/no-match.html", 0, 0, false},
	}
	for _, tc := range tests {
		leg, sess, ok := mbSessionFromURL(tc.url)
		if ok != tc.wantOK || leg != tc.wantLeg || sess != tc.wantSess {
			t.Errorf("mbSessionFromURL(%q) = (%d, %d, %v), want (%d, %d, %v)",
				tc.url, leg, sess, ok, tc.wantLeg, tc.wantSess, tc.wantOK)
		}
	}
}

func TestManitobaSessionPageMatches(t *testing.T) {
	tests := []struct {
		href        string
		legislature int
		session     int
		want        bool
	}{
		{href: "43rd/43rd_3rd.html", legislature: 43, session: 3, want: true},
		{href: "43rd/43rd_2nd.html", legislature: 43, session: 3, want: false},
		{href: "42nd/42nd_5th.html", legislature: 43, session: 3, want: false},
	}

	for _, tc := range tests {
		if got := manitobaSessionPageMatches(tc.href, tc.legislature, tc.session); got != tc.want {
			t.Fatalf("manitobaSessionPageMatches(%q, %d, %d)=%v, want %v", tc.href, tc.legislature, tc.session, got, tc.want)
		}
	}
}

func TestCleanupManitobaStaleSessionDivisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	if err := store.UpsertDivision(conn, store.DivisionRecord{ID: "mb-43-3-2025-04-07-2", Parliament: 43, Session: 3, Number: 2, Date: "2025-04-07", Yeas: 30, Nays: 20, SittingURL: "https://www.gov.mb.ca/legislature/business/43rd/2nd/votes_037.pdf", LastScraped: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("insert stale division: %v", err)
	}
	if err := store.UpsertDivision(conn, store.DivisionRecord{ID: "mb-43-3-2025-10-07-44", Parliament: 43, Session: 3, Number: 44, Date: "2025-10-07", Yeas: 35, Nays: 18, SittingURL: "https://www.gov.mb.ca/legislature/business/43rd/3rd/votes_044.pdf", LastScraped: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("insert current division: %v", err)
	}
	_, err = conn.Exec(`INSERT INTO members (id, name, province, chamber, active, government_level) VALUES
		('m1', 'Member One', 'Manitoba', 'manitoba', 1, 'provincial'),
		('m2', 'Member Two', 'Manitoba', 'manitoba', 1, 'provincial')`)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}
	_, err = conn.Exec(`INSERT INTO member_votes (division_id, member_id, vote) VALUES ('mb-43-3-2025-04-07-2', 'm1', 'Yea'), ('mb-43-3-2025-10-07-44', 'm2', 'Nay')`)
	if err != nil {
		t.Fatalf("insert member votes: %v", err)
	}

	deleted, err := cleanupManitobaStaleSessionDivisions(conn, 43, 3)
	if err != nil {
		t.Fatalf("cleanupManitobaStaleSessionDivisions: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}

	var staleCount, currentCount, staleVoteCount int
	if err := conn.QueryRow(`SELECT COUNT(1) FROM divisions WHERE id='mb-43-3-2025-04-07-2'`).Scan(&staleCount); err != nil {
		t.Fatalf("query stale division: %v", err)
	}
	if err := conn.QueryRow(`SELECT COUNT(1) FROM divisions WHERE id='mb-43-3-2025-10-07-44'`).Scan(&currentCount); err != nil {
		t.Fatalf("query current division: %v", err)
	}
	if err := conn.QueryRow(`SELECT COUNT(1) FROM member_votes WHERE division_id='mb-43-3-2025-04-07-2'`).Scan(&staleVoteCount); err != nil {
		t.Fatalf("query stale votes: %v", err)
	}
	if staleCount != 0 || currentCount != 1 || staleVoteCount != 0 {
		t.Fatalf("staleCount=%d currentCount=%d staleVoteCount=%d", staleCount, currentCount, staleVoteCount)
	}
}

func TestCleanManitobaDescription(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{
			// Raw PDF text: backslashes, leading THAT, French after /Loi
			in:   `THAT Bill No. 210\ The Indigenous Veterans Day Act Commemoration of Days, Weeks and Months Act Amended\/Loi sur la Journ` + "\x0e" + `e des anciens combattants autochtones`,
			want: "Bill No. 210 The Indigenous Veterans Day Act Commemoration of Days, Weeks and Months Act Amended",
		},
		{
			// Parenthesised bill number, clean /Loi boundary
			in:   "THAT Bill (No. 232) The Autism Strategy Act/Loi sur la strategie sur l'autisme, be now read a Second Time",
			want: "Bill (No. 232) The Autism Strategy Act",
		},
		{
			// THAT + em-dash: strip both "THAT" and the bill-number prefix, leaving just the act title.
			in:   "THAT Bill (No. 5) – The Accessibility for Manitobans Amendment Act/Loi modifiant la Loi sur l'accessibilite, be now read a Third Time",
			want: "The Accessibility for Manitobans Amendment Act",
		},
		{
			// THAT + em-dash, longer act title with parenthesised clause.
			in:   "THAT Bill (No. 48) – The Protective Detention and Care of Intoxicated Persons Act/Loi sur la détention des personnes",
			want: "The Protective Detention and Care of Intoxicated Persons Act",
		},
		{
			// No THAT prefix: keep "Bill (No. X) –" so bill context is preserved.
			in:   "Bill (No. 47) – The Fair Trade in Canada (Internal Trade Mutual Recognition) Act/Loi sur le commerce",
			want: "Bill (No. 47) – The Fair Trade in Canada (Internal Trade Mutual Recognition) Act",
		},
		{
			// Amendment motion: capitalise first letter, keep bill reference.
			in:   "amendment to Bill (No. 48) – The Protective Detention and Care of Intoxicated Persons Act/Loi sur la détention",
			want: "Amendment to Bill (No. 48) – The Protective Detention and Care of Intoxicated Persons Act",
		},
		{
			// No French section — description unchanged except THAT strip
			in:   "THAT Resolution No. 1: Something be done",
			want: "Resolution No. 1: Something be done",
		},
	}
	for _, tc := range tests {
		got := cleanManitobaDescription(tc.in)
		if got != tc.want {
			t.Errorf("cleanManitobaDescription(%q)\n  got  %q\n  want %q", tc.in, got, tc.want)
		}
	}
}

func TestMBMonthFromGrid(t *testing.T) {
	tests := []struct {
		row  int
		col  int
		want int
	}{
		{0, 0, 3},
		{0, 1, 4},
		{1, 0, 5},
		{1, 1, 6},
		{2, 0, 9},
		{2, 1, 10},
		{3, 0, 11},
		{3, 1, 12},
		{4, 0, 0},
		{0, 2, 0},
		{-1, 0, 0},
	}
	for _, tc := range tests {
		if got := MBMonthFromGrid(tc.row, tc.col); got != tc.want {
			t.Fatalf("MBMonthFromGrid(%d,%d)=%d want %d", tc.row, tc.col, got, tc.want)
		}
	}
}
