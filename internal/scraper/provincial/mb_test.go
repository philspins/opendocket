package provincial

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/philspins/open-democracy/internal/db"
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

	if err := db.UpsertDivision(conn, db.Division{ID: "mb-43-3-2025-04-07-2", Parliament: 43, Session: 3, Number: 2, Date: "2025-04-07", SittingURL: "https://www.gov.mb.ca/legislature/business/43rd/2nd/votes_037.pdf", LastScraped: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("insert stale division: %v", err)
	}
	if err := db.UpsertDivision(conn, db.Division{ID: "mb-43-3-2025-10-07-44", Parliament: 43, Session: 3, Number: 44, Date: "2025-10-07", SittingURL: "https://www.gov.mb.ca/legislature/business/43rd/3rd/votes_044.pdf", LastScraped: "2026-01-01T00:00:00Z"}); err != nil {
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
