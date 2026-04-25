package store_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/philspins/open-democracy/internal/db"
	"github.com/philspins/open-democracy/internal/store"
)

func tempDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("tempDB: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestListBills_Empty(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)
	bills, total, err := st.ListBills(store.BillFilter{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills: %v", err)
	}
	if total != 0 {
		t.Errorf("want total=0, got %d", total)
	}
	if len(bills) != 0 {
		t.Errorf("want 0 bills, got %d", len(bills))
	}
}

func TestListBills_Filter(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber)
		VALUES ('b1', 45, 1, 'C-1', 'Housing Act', 'Housing', '1st_reading', 'commons'),
		       ('b2', 45, 1, 'C-2', 'Health Act', 'Health', '1st_reading', 'commons')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	bills, total, err := st.ListBills(store.BillFilter{Category: "Housing", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills: %v", err)
	}
	if total != 1 {
		t.Errorf("want total=1, got %d", total)
	}
	if len(bills) != 1 || bills[0].ID != "b1" {
		t.Errorf("wrong bill returned: %+v", bills)
	}
}

func TestListBills_ChamberFilter(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber)
		VALUES ('b1', 45, 1, 'C-1', 'Commons Bill', 'Housing', '1st_reading', 'commons'),
		       ('b2', 45, 1, 'S-1', 'Senate Bill', 'Health', '1st_reading', 'senate')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	bills, total, err := st.ListBills(store.BillFilter{Chamber: "commons", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills: %v", err)
	}
	if total != 1 {
		t.Errorf("want total=1 for commons filter, got %d", total)
	}
	if len(bills) != 1 || bills[0].ID != "b1" {
		t.Errorf("wrong bill returned for commons filter: %+v", bills)
	}

	bills, total, err = st.ListBills(store.BillFilter{Chamber: "senate", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills senate: %v", err)
	}
	if total != 1 {
		t.Errorf("want total=1 for senate filter, got %d", total)
	}
	if len(bills) != 1 || bills[0].ID != "b2" {
		t.Errorf("wrong bill returned for senate filter: %+v", bills)
	}
}

func TestListBills_LevelFilter(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber)
		VALUES ('45-1-c-1', 45, 1, 'C-1', 'Federal Commons Bill', 'Housing', '1st_reading', 'commons'),
		       ('45-1-s-1', 45, 1, 'S-1', 'Federal Senate Bill', 'Health', '1st_reading', 'senate'),
		       ('on-43-1-12', 43, 1, '12', 'Ontario Bill', 'Housing', '1st_reading', 'ontario')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	bills, total, err := st.ListBills(store.BillFilter{Level: "federal", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills federal: %v", err)
	}
	if total != 2 || len(bills) != 2 {
		t.Fatalf("expected 2 federal bills, total=%d len=%d", total, len(bills))
	}
	for _, b := range bills {
		if b.ID == "on-43-1-12" {
			t.Fatalf("provincial bill included in federal filter: %+v", b)
		}
	}

	bills, total, err = st.ListBills(store.BillFilter{Level: "provincial", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills provincial: %v", err)
	}
	if total != 1 || len(bills) != 1 {
		t.Fatalf("expected 1 provincial bill, total=%d len=%d", total, len(bills))
	}
	if bills[0].ID != "on-43-1-12" {
		t.Fatalf("unexpected provincial result: %+v", bills[0])
	}
}

func TestListBills_ProvinceFilter(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber)
		VALUES ('on-43-1-12', 43, 1, '12', 'Ontario Bill', 'Housing', '1st_reading', 'ontario'),
		       ('bc-43-1-7', 43, 1, '7', 'British Columbia Bill', 'Health', '1st_reading', 'british_columbia'),
		       ('45-1-c-1', 45, 1, 'C-1', 'Federal Bill', 'Housing', '1st_reading', 'commons')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	bills, total, err := st.ListBills(store.BillFilter{Province: "Ontario", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills Ontario: %v", err)
	}
	if total != 1 || len(bills) != 1 || bills[0].ID != "on-43-1-12" {
		t.Fatalf("unexpected Ontario results: total=%d bills=%+v", total, bills)
	}

	bills, total, err = st.ListBills(store.BillFilter{Province: "BC", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills BC: %v", err)
	}
	if total != 1 || len(bills) != 1 || bills[0].ID != "bc-43-1-7" {
		t.Fatalf("unexpected BC results: total=%d bills=%+v", total, bills)
	}
	for _, b := range bills {
		if b.ID == "45-1-c-1" {
			t.Fatalf("federal bill included in province filter: %+v", b)
		}
	}
}

func TestListDistinctBillProvinces(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber)
		VALUES ('bc-43-1-7', 43, 1, '7', 'British Columbia Bill', 'Health', '1st_reading', 'british_columbia'),
		       ('on-43-1-12', 43, 1, '12', 'Ontario Bill', 'Housing', '1st_reading', 'ontario'),
		       ('45-1-c-1', 45, 1, 'C-1', 'Federal Bill', 'Housing', '1st_reading', 'commons')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	provinces, err := st.ListDistinctBillProvinces()
	if err != nil {
		t.Fatalf("ListDistinctBillProvinces: %v", err)
	}
	want := []string{"British Columbia", "Ontario"}
	if fmt.Sprint(provinces) != fmt.Sprint(want) {
		t.Fatalf("provinces=%v want %v", provinces, want)
	}
}

func TestGetBill_NotFound(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)
	_, err := st.GetBill("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent bill")
	}
}

func TestGetMember_NotFound(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)
	_, err := st.GetMember("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent member")
	}
}

func TestGetParliamentStatus_InSession(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)
	today := time.Now().Format("2006-01-02")
	_, err := conn.Exec(`INSERT INTO sitting_calendar (parliament, session, date) VALUES (45, 1, ?)`, today)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	ps, err := st.GetParliamentStatus(45, 1)
	if err != nil {
		t.Fatalf("GetParliamentStatus: %v", err)
	}
	if ps.Status != "in_session" {
		t.Errorf("want in_session, got %q", ps.Status)
	}
}

func TestGetParliamentStatus_OnBreak(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)
	// No sitting dates — should be on_break
	ps, err := st.GetParliamentStatus(45, 1)
	if err != nil {
		t.Fatalf("GetParliamentStatus: %v", err)
	}
	if ps.Status != "on_break" {
		t.Errorf("want on_break, got %q", ps.Status)
	}
}

func TestGetParliamentStatus_NextSitting(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)
	// Insert a future date
	futureDate := "2099-01-01"
	_, err := conn.Exec(`INSERT INTO sitting_calendar (parliament, session, date) VALUES (45, 1, ?)`, futureDate)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	ps, err := st.GetParliamentStatus(45, 1)
	if err != nil {
		t.Fatalf("GetParliamentStatus: %v", err)
	}
	if ps.Status != "on_break" {
		t.Errorf("want on_break for future date, got %q", ps.Status)
	}
	if ps.Detail != "Next sitting: "+futureDate {
		t.Errorf("unexpected detail: %q", ps.Detail)
	}
}

func TestGetJurisdictionStatus_InSessionWithNearbySchedule(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)
	today := time.Now().UTC().Format("2006-01-02")
	_, err := conn.Exec(`INSERT INTO legislature_calendar_dates (jurisdiction, date, last_scraped) VALUES ('provincial-ON', ?, '2026-01-01T00:00:00')`, today)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	status, err := st.GetJurisdictionStatus("provincial-ON")
	if err != nil {
		t.Fatalf("GetJurisdictionStatus: %v", err)
	}
	if status != "in_session" {
		t.Fatalf("status=%q want in_session", status)
	}
}

func TestGetCombinedJurisdictionStatus_AnyInSessionWins(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)
	today := time.Now().UTC().Format("2006-01-02")
	future := time.Now().UTC().AddDate(0, 3, 0).Format("2006-01-02")
	if _, err := conn.Exec(`INSERT INTO legislature_calendar_dates (jurisdiction, date, last_scraped) VALUES ('federal-commons', ?, '2026-01-01T00:00:00')`, future); err != nil {
		t.Fatalf("insert commons: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO legislature_calendar_dates (jurisdiction, date, last_scraped) VALUES ('federal-senate', ?, '2026-01-01T00:00:00')`, today); err != nil {
		t.Fatalf("insert senate: %v", err)
	}
	status, err := st.GetCombinedJurisdictionStatus("federal-commons", "federal-senate")
	if err != nil {
		t.Fatalf("GetCombinedJurisdictionStatus: %v", err)
	}
	if status != "in_session" {
		t.Fatalf("status=%q want in_session", status)
	}
}

func TestGetJurisdictionStatus_NoFutureDateRecentPastWindowIsTight(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)
	now := time.Now().UTC()

	recent := now.AddDate(0, 0, -2).Format("2006-01-02")
	if _, err := conn.Exec(`INSERT INTO legislature_calendar_dates (jurisdiction, date, last_scraped) VALUES ('provincial-ON', ?, '2026-01-01T00:00:00')`, recent); err != nil {
		t.Fatalf("insert recent date: %v", err)
	}
	status, err := st.GetJurisdictionStatus("provincial-ON")
	if err != nil {
		t.Fatalf("GetJurisdictionStatus recent: %v", err)
	}
	if status != "in_session" {
		t.Fatalf("status=%q want in_session for 2-day lookback", status)
	}

	old := now.AddDate(0, 0, -10).Format("2006-01-02")
	if _, err := conn.Exec(`INSERT INTO legislature_calendar_dates (jurisdiction, date, last_scraped) VALUES ('provincial-QC', ?, '2026-01-01T00:00:00')`, old); err != nil {
		t.Fatalf("insert old date: %v", err)
	}
	status, err = st.GetJurisdictionStatus("provincial-QC")
	if err != nil {
		t.Fatalf("GetJurisdictionStatus old: %v", err)
	}
	if status != "on_break" {
		t.Fatalf("status=%q want on_break for 10-day lookback with no future dates", status)
	}
}

func TestGetMemberStats_Basic(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active)
		VALUES ('m1', 'Alice Smith', 'Liberal', 'Ottawa Centre', 'ON', 'commons', 1),
		       ('m2', 'Bob Jones', 'Liberal', 'Ottawa West', 'ON', 'commons', 1)`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}

	for i := 1; i <= 3; i++ {
		_, err := conn.Exec(fmt.Sprintf(`INSERT INTO divisions (id, parliament, session, number, date, yeas, nays, result, chamber)
			VALUES (?, 45, 1, ?, '2025-01-0%d', 100, 50, 'Carried', 'commons')`, i),
			fmt.Sprintf("d%d", i), i)
		if err != nil {
			t.Fatalf("insert division: %v", err)
		}
	}

	// m1 votes: d1=Yea, d2=Yea, d3=Nay; m2 votes: d1=Yea, d2=Yea, d3=Yea
	for _, v := range []struct{ div, member, vote string }{
		{"d1", "m1", "Yea"}, {"d1", "m2", "Yea"},
		{"d2", "m1", "Yea"}, {"d2", "m2", "Yea"},
		{"d3", "m1", "Nay"}, {"d3", "m2", "Yea"},
	} {
		_, err := conn.Exec(`INSERT INTO member_votes (division_id, member_id, vote) VALUES (?,?,?)`,
			v.div, v.member, v.vote)
		if err != nil {
			t.Fatalf("insert vote: %v", err)
		}
	}

	stats, err := st.GetMemberStats("m1")
	if err != nil {
		t.Fatalf("GetMemberStats: %v", err)
	}
	if stats.TotalVotes != 3 {
		t.Errorf("want TotalVotes=3, got %d", stats.TotalVotes)
	}
	// m1 voted Yea in d1,d2 (party majority Yea) and Nay in d3 (party majority Yea from m2)
	// party line: 2/3 = 66%
	if stats.PartyLinePct < 60 || stats.PartyLinePct > 70 {
		t.Errorf("want PartyLinePct ~66, got %d", stats.PartyLinePct)
	}
}

func TestGetMemberStats_SolePartyMemberCountsAsPartyLine(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, government_level)
		VALUES ('m1', 'Elizabeth May', 'Green Party', 'Saanich--Gulf Islands', 'BC', 'commons', 1, 'federal'),
		       ('m2', 'Alex Other', 'Liberal', 'Ottawa Centre', 'ON', 'commons', 1, 'federal')`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}

	for i := 1; i <= 3; i++ {
		_, err := conn.Exec(fmt.Sprintf(`INSERT INTO divisions (id, parliament, session, number, date, yeas, nays, result, chamber)
			VALUES (?, 45, 1, ?, '2025-02-0%d', 100, 50, 'Carried', 'commons')`, i),
			fmt.Sprintf("d%d", i), i)
		if err != nil {
			t.Fatalf("insert division: %v", err)
		}
	}

	for _, v := range []struct{ div, vote string }{
		{"d1", "Yea"},
		{"d2", "Nay"},
		{"d3", "Yea"},
	} {
		_, err := conn.Exec(`INSERT INTO member_votes (division_id, member_id, vote) VALUES (?, 'm1', ?)`, v.div, v.vote)
		if err != nil {
			t.Fatalf("insert vote: %v", err)
		}
	}

	stats, err := st.GetMemberStats("m1")
	if err != nil {
		t.Fatalf("GetMemberStats: %v", err)
	}
	if stats.TotalVotes != 3 {
		t.Fatalf("want TotalVotes=3, got %d", stats.TotalVotes)
	}
	if stats.PartyLinePct != 100 {
		t.Fatalf("want PartyLinePct=100 for sole-party member, got %d", stats.PartyLinePct)
	}
	if stats.RebelPct != 0 {
		t.Fatalf("want RebelPct=0 for sole-party member, got %d", stats.RebelPct)
	}
}

func TestCompareMemberVotes(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO members (id, name, party, chamber, active)
		VALUES ('m1', 'Alice', 'Liberal', 'commons', 1),
		       ('m2', 'Bob', 'Conservative', 'commons', 1)`)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}

	for i := 1; i <= 3; i++ {
		_, err := conn.Exec(fmt.Sprintf(`INSERT INTO divisions (id, parliament, session, number, date, yeas, nays, result, chamber)
			VALUES (?, 45, 1, ?, '2025-01-0%d', 100, 50, 'Carried', 'commons')`, i),
			fmt.Sprintf("d%d", i), i)
		if err != nil {
			t.Fatalf("insert division: %v", err)
		}
	}

	// d1: both Yea (agree), d2: both Nay (agree), d3: m1 Yea m2 Nay (disagree)
	for _, v := range []struct{ div, member, vote string }{
		{"d1", "m1", "Yea"}, {"d1", "m2", "Yea"},
		{"d2", "m1", "Nay"}, {"d2", "m2", "Nay"},
		{"d3", "m1", "Yea"}, {"d3", "m2", "Nay"},
	} {
		_, err := conn.Exec(`INSERT INTO member_votes (division_id, member_id, vote) VALUES (?,?,?)`,
			v.div, v.member, v.vote)
		if err != nil {
			t.Fatalf("insert vote: %v", err)
		}
	}

	overlap, total, err := st.CompareMemberVotes("m1", "m2")
	if err != nil {
		t.Fatalf("CompareMemberVotes: %v", err)
	}
	if total != 3 {
		t.Errorf("want total=3, got %d", total)
	}
	if overlap != 2 {
		t.Errorf("want overlap=2, got %d", overlap)
	}
}

func TestGetSharedMemberVotes(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO members (id, name, party, chamber, active, government_level)
		VALUES ('m1', 'Alice', 'Liberal', 'commons', 1, 'federal'),
		       ('m2', 'Bob', 'Conservative', 'commons', 1, 'federal')`)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}

	_, err = conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, chamber)
		VALUES ('b1', 45, 1, 'C-1', 'Bill One', 'commons')`)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	for i := 1; i <= 3; i++ {
		_, err := conn.Exec(fmt.Sprintf(`INSERT INTO divisions (id, parliament, session, number, date, bill_id, yeas, nays, result, chamber)
			VALUES (?, 45, 1, ?, '2025-01-0%d', ?, 100, 50, 'Carried', 'commons')`, i),
			fmt.Sprintf("d%d", i), i, "b1")
		if err != nil {
			t.Fatalf("insert division: %v", err)
		}
	}

	// Shared on d1 and d2; only m1 voted on d3.
	for _, v := range []struct{ div, member, vote string }{
		{"d1", "m1", "Yea"}, {"d1", "m2", "Nay"},
		{"d2", "m1", "Nay"}, {"d2", "m2", "Nay"},
		{"d3", "m1", "Yea"},
	} {
		_, err := conn.Exec(`INSERT INTO member_votes (division_id, member_id, vote) VALUES (?,?,?)`,
			v.div, v.member, v.vote)
		if err != nil {
			t.Fatalf("insert vote: %v", err)
		}
	}

	shared, err := st.GetSharedMemberVotes("m1", "m2", 10)
	if err != nil {
		t.Fatalf("GetSharedMemberVotes: %v", err)
	}
	if len(shared) != 2 {
		t.Fatalf("want 2 shared votes, got %d", len(shared))
	}
	if shared[0].DivisionID != "d2" || shared[1].DivisionID != "d1" {
		t.Fatalf("unexpected shared vote order: %+v", shared)
	}
	if shared[1].BillNumber != "C-1" {
		t.Fatalf("expected bill number on d1, got %q", shared[1].BillNumber)
	}
	if shared[1].Member1Vote != "Yea" || shared[1].Member2Vote != "Nay" {
		t.Fatalf("unexpected shared vote values: %+v", shared[1])
	}
}

func TestUpsertUserAndFollowMember(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO members (id, name, chamber, active) VALUES ('m1', 'Jane MP', 'commons', 1)`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}

	u, err := st.UpsertUser("person@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if u.ID == "" || u.Email != "person@example.com" {
		t.Fatalf("unexpected user: %+v", u)
	}

	if err := st.FollowMember("person@example.com", "m1"); err != nil {
		t.Fatalf("FollowMember: %v", err)
	}

	var count int
	err = conn.QueryRow(`SELECT COUNT(*) FROM user_follows WHERE user_id=? AND member_id='m1'`, u.ID).Scan(&count)
	if err != nil {
		t.Fatalf("query follow: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 follow row, got %d", count)
	}
}

func TestReactToBillAndCounts(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title) VALUES ('b1', 45, 1, 'C-1', 'Test Bill')`)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	if err := st.ReactToBill("a@example.com", "b1", "support", "Looks good"); err != nil {
		t.Fatalf("ReactToBill support: %v", err)
	}
	if err := st.ReactToBill("b@example.com", "b1", "oppose", "Concerned"); err != nil {
		t.Fatalf("ReactToBill oppose: %v", err)
	}
	if err := st.ReactToBill("a@example.com", "b1", "neutral", "Updating vote"); err != nil {
		t.Fatalf("ReactToBill update: %v", err)
	}

	c, err := st.GetBillReactionCounts("b1")
	if err != nil {
		t.Fatalf("GetBillReactionCounts: %v", err)
	}
	if c.TotalReactions != 2 || c.SupportCount != 0 || c.OpposeCount != 1 || c.NeutralCount != 1 {
		t.Fatalf("unexpected counts: %+v", c)
	}
}

func TestLogPolicySubmission(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO members (id, name, chamber, active) VALUES ('m1', 'Jane MP', 'commons', 1)`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}

	err = st.LogPolicySubmission("person@example.com", "m1", "Housing support", "Please support this bill", "Housing")
	if err != nil {
		t.Fatalf("LogPolicySubmission: %v", err)
	}

	var count int
	err = conn.QueryRow(`SELECT COUNT(*) FROM policy_submissions`).Scan(&count)
	if err != nil {
		t.Fatalf("query submissions: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 submission row, got %d", count)
	}
}

func TestEmailVerificationFlow(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	token, _, err := st.CreateEmailVerification("verify@example.com", time.Hour)
	if err != nil {
		t.Fatalf("CreateEmailVerification: %v", err)
	}

	var storedToken string
	if err := conn.QueryRow(`SELECT token FROM email_verification_tokens WHERE email='verify@example.com' ORDER BY id DESC LIMIT 1`).Scan(&storedToken); err != nil {
		t.Fatalf("query stored token: %v", err)
	}
	if storedToken == token {
		t.Fatalf("expected token to be hashed at rest")
	}
	if len(storedToken) != 64 {
		t.Fatalf("expected sha256 hex token length 64, got %d", len(storedToken))
	}

	var storedCode string
	if err := conn.QueryRow(`SELECT code FROM email_verification_tokens WHERE email='verify@example.com' ORDER BY id DESC LIMIT 1`).Scan(&storedCode); err != nil {
		t.Fatalf("query stored code: %v", err)
	}
	if len(storedCode) != 64 {
		t.Fatalf("expected hashed code length 64, got %d", len(storedCode))
	}

	u, err := st.GetUserByEmail("verify@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if u.EmailVerified {
		t.Fatalf("expected user to be unverified before token verification")
	}

	u, err = st.VerifyEmailToken(token)
	if err != nil {
		t.Fatalf("VerifyEmailToken: %v", err)
	}
	if !u.EmailVerified {
		t.Fatalf("expected user to be verified after token verification")
	}

	if _, err := st.VerifyEmailToken(token); err == nil {
		t.Fatalf("expected second token use to fail")
	}
}

func TestEmailVerificationByCode(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, code, err := st.CreateEmailVerification("code@example.com", time.Hour)
	if err != nil {
		t.Fatalf("CreateEmailVerification: %v", err)
	}

	u, err := st.VerifyEmailCode("code@example.com", code)
	if err != nil {
		t.Fatalf("VerifyEmailCode: %v", err)
	}
	if !u.EmailVerified {
		t.Fatalf("expected code-verified user to be verified")
	}
}

func TestEmailVerificationCooldown(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	if _, _, err := st.CreateEmailVerification("cooldown@example.com", time.Hour); err != nil {
		t.Fatalf("first CreateEmailVerification: %v", err)
	}
	if _, _, err := st.CreateEmailVerification("cooldown@example.com", time.Hour); err == nil {
		t.Fatalf("expected cooldown error on rapid second verification request")
	}
}

func TestSessionLifecycle(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	u, err := st.UpsertUser("session@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	var storedSessionID string
	if err := conn.QueryRow(`SELECT id FROM user_sessions WHERE user_id = ?`, u.ID).Scan(&storedSessionID); err != nil {
		t.Fatalf("query stored session id: %v", err)
	}
	if storedSessionID == sid {
		t.Fatalf("expected session id to be hashed at rest")
	}
	if len(storedSessionID) != 64 {
		t.Fatalf("expected sha256 hex session id length 64, got %d", len(storedSessionID))
	}

	got, err := st.GetUserBySession(sid)
	if err != nil {
		t.Fatalf("GetUserBySession: %v", err)
	}
	if got.Email != "session@example.com" {
		t.Fatalf("unexpected session user: %+v", got)
	}

	if err := st.DeleteSession(sid); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := st.GetUserBySession(sid); err == nil {
		t.Fatalf("expected deleted session lookup to fail")
	}
}

func TestAuthenticateOAuthMarksVerified(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	u, err := st.AuthenticateOAuth("google", "provider-123", "oauth@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	if !u.EmailVerified {
		t.Fatalf("expected oauth-authenticated user to be verified")
	}

	// Re-auth on same provider identity should remain idempotent.
	u2, err := st.AuthenticateOAuth("google", "provider-123", "oauth@example.com", true)
	if err != nil {
		t.Fatalf("AuthenticateOAuth repeat: %v", err)
	}
	if u.ID != u2.ID {
		t.Fatalf("expected same user id on repeated oauth auth, got %q and %q", u.ID, u2.ID)
	}
}

func TestUpdateUserLocationPersistsOnlyRidings(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	u, err := st.UpsertUser("profile@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	updated, err := st.UpdateUserLocation(u.ID, "Ottawa Centre", "Ottawa South")
	if err != nil {
		t.Fatalf("UpdateUserLocation: %v", err)
	}
	if updated.Address != "" {
		t.Fatalf("Address=%q want empty", updated.Address)
	}
	if updated.FederalRidingID != "Ottawa Centre" || updated.ProvincialRidingID != "Ottawa South" {
		t.Fatalf("unexpected riding ids: %+v", updated)
	}

	reloaded, err := st.GetUserByEmail("profile@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if reloaded.Address != "" || reloaded.FederalRidingID != updated.FederalRidingID || reloaded.ProvincialRidingID != updated.ProvincialRidingID {
		t.Fatalf("reloaded user mismatch: %+v vs %+v", reloaded, updated)
	}
}

func TestListMembers_Filters(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, government_level)
		VALUES ('m1', 'Alice Smith', 'Liberal', 'Ottawa Centre', 'Ontario', 'commons', 1, 'federal'),
		       ('m2', 'Bob Jones', 'Conservative', 'Calgary East', 'Alberta', 'commons', 1, 'federal'),
		       ('m3', 'Carol White', 'NDP', 'Vancouver East', 'British Columbia', 'legislature', 1, 'provincial')`)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}

	tests := []struct {
		name            string
		search          string
		party           string
		province        string
		riding          string
		governmentLevel string
		wantIDs         []string
	}{
		{"no filter returns all", "", "", "", "", "", []string{"m1", "m2", "m3"}},
		{"name search exact", "Alice Smith", "", "", "", "", []string{"m1"}},
		{"name search partial", "alice", "", "", "", "", []string{"m1"}},
		{"name search case insensitive", "ALICE", "", "", "", "", []string{"m1"}},
		{"party exact match", "", "Liberal", "", "", "", []string{"m1"}},
		{"province exact match", "", "", "Ontario", "", "", []string{"m1"}},
		{"province abbreviation BC expands to British Columbia", "", "", "BC", "", "", []string{"m3"}},
		{"province abbreviation bc lowercase", "", "", "bc", "", "", []string{"m3"}},
		{"province abbreviation ON expands to Ontario", "", "", "ON", "", "", []string{"m1"}},
		{"province abbreviation AB expands to Alberta", "", "", "AB", "", "", []string{"m2"}},
		{"riding exact match", "", "", "", "Ottawa Centre", "", []string{"m1"}},
		{"name and party combined", "alice", "Liberal", "", "", "", []string{"m1"}},
		{"no match returns empty", "zzz", "", "", "", "", []string{}},
		{"federal filter returns two", "", "", "", "", "federal", []string{"m1", "m2"}},
		{"provincial filter returns one", "", "", "", "", "provincial", []string{"m3"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			members, err := st.ListMembers(tc.search, tc.party, tc.province, tc.riding, tc.governmentLevel)
			if err != nil {
				t.Fatalf("ListMembers: %v", err)
			}
			gotIDs := make([]string, len(members))
			for i, m := range members {
				gotIDs[i] = m.ID
			}
			if len(gotIDs) != len(tc.wantIDs) {
				t.Errorf("got %d members (%v), want %d (%v)", len(gotIDs), gotIDs, len(tc.wantIDs), tc.wantIDs)
				return
			}
			wantSet := make(map[string]bool)
			for _, id := range tc.wantIDs {
				wantSet[id] = true
			}
			for _, id := range gotIDs {
				if !wantSet[id] {
					t.Errorf("unexpected member ID %q in results %v", id, gotIDs)
				}
			}
		})
	}
}

func TestGetMemberCategoryScores(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	// Insert a member, two bills in different categories, two divisions, and votes.
	_, err := conn.Exec(`
		INSERT INTO members (id, name, party, riding, province, chamber, active, government_level)
		VALUES ('m1', 'Alice Smith', 'Liberal', 'Ottawa Centre', 'Ontario', 'commons', 1, 'federal')`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}
	_, err = conn.Exec(`
		INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber)
		VALUES ('b-housing', 45, 1, 'C-1', 'Housing Act', 'Housing', '1st_reading', 'commons'),
		       ('b-health',  45, 1, 'C-2', 'Health Act',  'Health',  '1st_reading', 'commons')`)
	if err != nil {
		t.Fatalf("insert bills: %v", err)
	}
	_, err = conn.Exec(`
		INSERT INTO divisions (id, parliament, session, number, date, bill_id, description, yeas, nays, result, chamber)
		VALUES ('d1', 45, 1, 1, '2024-01-01', 'b-housing', 'Housing vote 1', 100, 50, 'Passed', 'commons'),
		       ('d2', 45, 1, 2, '2024-01-02', 'b-housing', 'Housing vote 2', 80,  70, 'Passed', 'commons'),
		       ('d3', 45, 1, 3, '2024-01-03', 'b-health',  'Health vote 1',  90,  60, 'Passed', 'commons')`)
	if err != nil {
		t.Fatalf("insert divisions: %v", err)
	}
	// Alice votes Yea on both housing divisions and Nay on the health division.
	_, err = conn.Exec(`
		INSERT INTO member_votes (member_id, division_id, vote)
		VALUES ('m1', 'd1', 'Yea'),
		       ('m1', 'd2', 'Nay'),
		       ('m1', 'd3', 'Nay')`)
	if err != nil {
		t.Fatalf("insert member_votes: %v", err)
	}

	scores, err := st.GetMemberCategoryScores("m1")
	if err != nil {
		t.Fatalf("GetMemberCategoryScores: %v", err)
	}

	// Expect 2 categories; Housing has more votes so it comes first.
	if len(scores) != 2 {
		t.Fatalf("want 2 category scores, got %d (%+v)", len(scores), scores)
	}

	// Housing: 1 Yea + 1 Nay = total 2, YeaPct = 50
	hsc := scores[0]
	if hsc.Category != "Housing" {
		t.Errorf("first category: want Housing, got %q", hsc.Category)
	}
	if hsc.Total != 2 || hsc.Yeas != 1 || hsc.Nays != 1 {
		t.Errorf("Housing totals: want total=2 yeas=1 nays=1, got %+v", hsc)
	}
	if hsc.YeaPct != 50 {
		t.Errorf("Housing YeaPct: want 50, got %d", hsc.YeaPct)
	}

	// Health: 0 Yea + 1 Nay = total 1, YeaPct = 0
	hlt := scores[1]
	if hlt.Category != "Health" {
		t.Errorf("second category: want Health, got %q", hlt.Category)
	}
	if hlt.Total != 1 || hlt.Yeas != 0 || hlt.Nays != 1 {
		t.Errorf("Health totals: want total=1 yeas=0 nays=1, got %+v", hlt)
	}
	if hlt.YeaPct != 0 {
		t.Errorf("Health YeaPct: want 0, got %d", hlt.YeaPct)
	}
}

func TestGetMemberCategoryScores_Empty(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	scores, err := st.GetMemberCategoryScores("nonexistent")
	if err != nil {
		t.Fatalf("GetMemberCategoryScores: %v", err)
	}
	if len(scores) != 0 {
		t.Errorf("want 0 scores for unknown member, got %d", len(scores))
	}
}

func TestUserCategoryPreferences(t *testing.T) {
conn := tempDB(t)
st := store.New(conn)

// Create user
u, err := st.UpsertUser("prefs@example.com")
if err != nil {
t.Fatalf("UpsertUser: %v", err)
}

// Initially empty
cats, err := st.GetUserCategoryPreferences(u.ID)
if err != nil {
t.Fatalf("GetUserCategoryPreferences initial: %v", err)
}
if len(cats) != 0 {
t.Errorf("expected 0 prefs, got %d", len(cats))
}

// Save preferences
if err := st.SaveUserCategoryPreferences(u.ID, []string{"Housing", "Health", "Environment"}); err != nil {
t.Fatalf("SaveUserCategoryPreferences: %v", err)
}

cats, err = st.GetUserCategoryPreferences(u.ID)
if err != nil {
t.Fatalf("GetUserCategoryPreferences after save: %v", err)
}
if len(cats) != 3 {
t.Errorf("expected 3 prefs, got %d: %v", len(cats), cats)
}

// Update preferences (should replace, not append)
if err := st.SaveUserCategoryPreferences(u.ID, []string{"Budget"}); err != nil {
t.Fatalf("SaveUserCategoryPreferences update: %v", err)
}
cats, err = st.GetUserCategoryPreferences(u.ID)
if err != nil {
t.Fatalf("GetUserCategoryPreferences after update: %v", err)
}
if len(cats) != 1 || cats[0] != "Budget" {
t.Errorf("expected [Budget], got %v", cats)
}

// Clear preferences
if err := st.SaveUserCategoryPreferences(u.ID, []string{}); err != nil {
t.Fatalf("SaveUserCategoryPreferences clear: %v", err)
}
cats, err = st.GetUserCategoryPreferences(u.ID)
if err != nil {
t.Fatalf("GetUserCategoryPreferences after clear: %v", err)
}
if len(cats) != 0 {
t.Errorf("expected 0 prefs after clear, got %d", len(cats))
}
}

func TestUserBillSubscriptions(t *testing.T) {
conn := tempDB(t)
st := store.New(conn)

_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title) VALUES
('b1', 45, 1, 'C-1', 'Bill One'),
('b2', 45, 1, 'C-2', 'Bill Two')`)
if err != nil {
t.Fatalf("insert bills: %v", err)
}

u, err := st.UpsertUser("sub@example.com")
if err != nil {
t.Fatalf("UpsertUser: %v", err)
}

// Initially not subscribed
ok, err := st.IsUserSubscribedToBill(u.ID, "b1")
if err != nil {
t.Fatalf("IsUserSubscribedToBill: %v", err)
}
if ok {
t.Error("expected not subscribed initially")
}

// Subscribe
subscribed, err := st.ToggleBillSubscription(u.ID, "b1")
if err != nil {
t.Fatalf("ToggleBillSubscription subscribe: %v", err)
}
if !subscribed {
t.Error("expected subscribed=true after first toggle")
}

ok, err = st.IsUserSubscribedToBill(u.ID, "b1")
if err != nil {
t.Fatalf("IsUserSubscribedToBill after subscribe: %v", err)
}
if !ok {
t.Error("expected subscribed after toggle")
}

// GetUserBillSubscriptions
ids, err := st.GetUserBillSubscriptions(u.ID)
if err != nil {
t.Fatalf("GetUserBillSubscriptions: %v", err)
}
if len(ids) != 1 || ids[0] != "b1" {
t.Errorf("expected [b1], got %v", ids)
}

// Unsubscribe
subscribed, err = st.ToggleBillSubscription(u.ID, "b1")
if err != nil {
t.Fatalf("ToggleBillSubscription unsubscribe: %v", err)
}
if subscribed {
t.Error("expected subscribed=false after second toggle")
}

ids, err = st.GetUserBillSubscriptions(u.ID)
if err != nil {
t.Fatalf("GetUserBillSubscriptions after unsubscribe: %v", err)
}
if len(ids) != 0 {
t.Errorf("expected empty after unsubscribe, got %v", ids)
}
}

func TestListBills_SortOptions(t *testing.T) {
conn := tempDB(t)
st := store.New(conn)

_, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber, last_activity_date)
VALUES
('b1', 45, 1, 'C-1', 'Housing Act', 'Housing', '1st_reading', 'commons', '2024-01-01'),
('b2', 45, 1, 'C-2', 'Health Act', 'Health', '2nd_reading', 'commons', '2024-02-01'),
('b3', 45, 1, 'C-3', 'Budget Act', 'Budget', '3rd_reading', 'commons', '2024-03-01')`)
if err != nil {
t.Fatalf("insert bills: %v", err)
}

// Default sort: latest first
bills, _, err := st.ListBills(store.BillFilter{Page: 1, PerPage: 20})
if err != nil {
t.Fatalf("ListBills default: %v", err)
}
if len(bills) != 3 || bills[0].ID != "b3" {
t.Errorf("default sort should have b3 first (latest), got: %v", bills[0].ID)
}

// date_asc: oldest first
bills, _, err = st.ListBills(store.BillFilter{Sort: "date_asc", Page: 1, PerPage: 20})
if err != nil {
t.Fatalf("ListBills date_asc: %v", err)
}
if len(bills) != 3 || bills[0].ID != "b1" {
t.Errorf("date_asc sort should have b1 first (oldest), got: %v", bills[0].ID)
}

// category: alphabetical by category
bills, _, err = st.ListBills(store.BillFilter{Sort: "category", Page: 1, PerPage: 20})
if err != nil {
t.Fatalf("ListBills category: %v", err)
}
if len(bills) != 3 || bills[0].ID != "b3" { // Budget comes first alphabetically
t.Errorf("category sort should have b3 first (Budget), got: %v", bills[0].ID)
}

// auto: preferred category Housing first, then by date
bills, _, err = st.ListBills(store.BillFilter{
Sort:                "auto",
PreferredCategories: []string{"Housing"},
Page:                1,
PerPage:             20,
})
if err != nil {
t.Fatalf("ListBills auto: %v", err)
}
if len(bills) != 3 || bills[0].ID != "b1" { // Housing bill first
t.Errorf("auto sort should have b1 first (Housing preferred), got: %v", bills[0].ID)
}

// auto with subscribed: subscribed bill first, then preferred category, then rest
bills, _, err = st.ListBills(store.BillFilter{
Sort:                "auto",
PreferredCategories: []string{"Housing"},
SubscribedBillIDs:   []string{"b2"},
Page:                1,
PerPage:             20,
})
if err != nil {
t.Fatalf("ListBills auto with subscribed: %v", err)
}
if len(bills) != 3 || bills[0].ID != "b2" { // Subscribed bill first
t.Errorf("auto sort with subscription should have b2 first (subscribed), got: %v", bills[0].ID)
}
if bills[1].ID != "b1" { // Preferred category next
t.Errorf("auto sort second should be b1 (preferred Housing), got: %v", bills[1].ID)
}
}
