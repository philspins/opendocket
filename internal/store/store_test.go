package store_test

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/testutil"
)

func tempDB(t *testing.T) *sql.DB {
	t.Helper()
	return testutil.OpenDB(t)
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

func TestGetMemberStats_MissedPct_UsesTermDates(t *testing.T) {
	conn := tempDB(t)

	// term_start is 2024-02-01, so d-before (2024-01-15) must NOT count.
	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, term_start)
		VALUES ('m-term', 'Term MP', 'Liberal', 'Ottawa Centre', 'ON', 'commons', 1, '2024-02-01'),
		       ('m-lib2', 'Lib Colleague', 'Liberal', 'Ottawa West', 'ON', 'commons', 1, NULL)`)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}

	for _, row := range []struct {
		id   string
		date string
		num  int
	}{
		{"d-before", "2024-01-15", 1}, // before term — must NOT count toward denominator
		{"d-in1", "2024-02-05", 2},
		{"d-in2", "2024-02-10", 3},
	} {
		_, err = conn.Exec(`INSERT INTO divisions (id, parliament, session, number, date, yeas, nays, result, chamber)
			VALUES (?, 45, 1, ?, ?, 100, 50, 'Carried', 'commons')`,
			row.id, row.num, row.date)
		if err != nil {
			t.Fatalf("insert division %s: %v", row.id, err)
		}
	}

	// m-term votes only on d-in1; d-in2 is missed; d-before must be excluded
	_, err = conn.Exec(`INSERT INTO member_votes (division_id, member_id, vote) VALUES ('d-in1', 'm-term', 'Yea')`)
	if err != nil {
		t.Fatalf("insert vote: %v", err)
	}

	st := store.New(conn)
	stats, err := st.GetMemberStats("m-term")
	if err != nil {
		t.Fatalf("GetMemberStats: %v", err)
	}
	// 2 divisions in term, 1 voted, 1 missed → MissedPct = 50
	if stats.MissedPct != 50 {
		t.Errorf("want MissedPct=50, got %d", stats.MissedPct)
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

	_, err = conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, short_title, chamber)
		VALUES ('b1', 45, 1, 'C-1', 'Bill One', 'Short Bill One', 'commons')`)
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
	if shared[1].BillTitle != "Short Bill One" {
		t.Fatalf("expected bill title on d1, got %q", shared[1].BillTitle)
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

// ── write helpers ─────────────────────────────────────────────────────────────

func TestUpsertMemberViaStoreWrite(t *testing.T) {
	conn := tempDB(t)
	m := store.MemberRecord{
		ID: "m-write-1", Name: "Write Test MP", Party: "NDP",
		Riding: "Victoria", Province: "British Columbia",
		Chamber: "commons", Active: true, LastScraped: "2026-01-01", GovernmentLevel: "federal",
	}
	if err := store.UpsertMember(conn, m); err != nil {
		t.Fatalf("UpsertMember: %v", err)
	}
	st := store.New(conn)
	got, err := st.GetMember("m-write-1")
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if got.Name != "Write Test MP" || got.Party != "NDP" || got.GovernmentLevel != "federal" {
		t.Errorf("unexpected member: %+v", got)
	}
}

func TestUpsertBillViaStoreWrite(t *testing.T) {
	conn := tempDB(t)
	b := store.BillRecord{
		ID: "45-1-c-99", Parliament: 45, Session: 1, Number: "C-99",
		Title: "Write Test Bill", Chamber: "commons", Category: "Housing",
		CurrentStage: "1st_reading", LastScraped: "2026-01-01",
	}
	if err := store.UpsertBill(conn, b); err != nil {
		t.Fatalf("UpsertBill: %v", err)
	}
	st := store.New(conn)
	got, err := st.GetBill("45-1-c-99")
	if err != nil {
		t.Fatalf("GetBill: %v", err)
	}
	if got.Title != "Write Test Bill" || got.Category != "Housing" {
		t.Errorf("unexpected bill: %+v", got)
	}
}

func TestUpsertBillStageAndGetBillStages(t *testing.T) {
	conn := tempDB(t)
	if err := store.UpsertBill(conn, store.BillRecord{
		ID: "b-stage", Parliament: 45, Session: 1, Number: "C-1",
		Title: "Stage Bill", LastScraped: "2026-01-01",
	}); err != nil {
		t.Fatalf("UpsertBill: %v", err)
	}
	stages := []store.BillStageRecord{
		{BillID: "b-stage", Stage: "1st_reading", Chamber: "commons", Date: "2024-01-01"},
		{BillID: "b-stage", Stage: "2nd_reading", Chamber: "commons", Date: "2024-02-01"},
	}
	for _, s := range stages {
		if err := store.UpsertBillStage(conn, s); err != nil {
			t.Fatalf("UpsertBillStage: %v", err)
		}
	}
	st := store.New(conn)
	got, err := st.GetBillStages("b-stage")
	if err != nil {
		t.Fatalf("GetBillStages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 stages, got %d", len(got))
	}
	if got[0].Stage != "1st_reading" || got[1].Stage != "2nd_reading" {
		t.Errorf("unexpected stages: %+v", got)
	}
}

func TestUpsertDivisionAndGetBillDivisions(t *testing.T) {
	conn := tempDB(t)
	if err := store.UpsertBill(conn, store.BillRecord{
		ID: "b-div", Parliament: 45, Session: 1, Number: "C-2",
		Title: "Division Bill", LastScraped: "2026-01-01",
	}); err != nil {
		t.Fatalf("UpsertBill: %v", err)
	}
	div := store.DivisionRecord{
		ID: "45-1-10", Parliament: 45, Session: 1, Number: 10,
		Date: "2024-03-01", BillID: "b-div", Yeas: 150, Nays: 100,
		Result: "Agreed to", Chamber: "commons", LastScraped: "2026-01-01",
	}
	if err := store.UpsertDivision(conn, div); err != nil {
		t.Fatalf("UpsertDivision: %v", err)
	}
	st := store.New(conn)
	divs, err := st.GetBillDivisions("b-div")
	if err != nil {
		t.Fatalf("GetBillDivisions: %v", err)
	}
	if len(divs) != 1 || divs[0].ID != "45-1-10" || divs[0].Yeas != 150 {
		t.Fatalf("unexpected division: %+v", divs)
	}
}

func TestListDivisionsAndGetRecentDivisions(t *testing.T) {
	conn := tempDB(t)
	for i := 1; i <= 3; i++ {
		if err := store.UpsertDivision(conn, store.DivisionRecord{
			ID: fmt.Sprintf("d%d", i), Parliament: 45, Session: 1, Number: i,
			Date: fmt.Sprintf("2024-01-0%d", i), Yeas: 100, Nays: 50,
			Result: "Agreed to", Chamber: "commons", LastScraped: "2026-01-01",
		}); err != nil {
			t.Fatalf("UpsertDivision: %v", err)
		}
	}
	st := store.New(conn)
	divs, total, err := st.ListDivisions(1, 10)
	if err != nil {
		t.Fatalf("ListDivisions: %v", err)
	}
	if total != 3 || len(divs) != 3 {
		t.Fatalf("want 3 divisions, got total=%d len=%d", total, len(divs))
	}

	recent, err := st.GetRecentDivisions(2)
	if err != nil {
		t.Fatalf("GetRecentDivisions: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("want 2 recent divisions, got %d", len(recent))
	}
}

func TestGetRecentBillsUsesDefaultLimit(t *testing.T) {
	conn := tempDB(t)
	for i := 1; i <= 5; i++ {
		if err := store.UpsertBill(conn, store.BillRecord{
			ID: fmt.Sprintf("b%d", i), Parliament: 45, Session: 1,
			Number: fmt.Sprintf("C-%d", i), Title: fmt.Sprintf("Bill %d", i),
			LastScraped: "2026-01-01",
		}); err != nil {
			t.Fatalf("UpsertBill: %v", err)
		}
	}
	st := store.New(conn)
	bills, err := st.GetRecentBills(3)
	if err != nil {
		t.Fatalf("GetRecentBills: %v", err)
	}
	if len(bills) != 3 {
		t.Fatalf("want 3 recent bills, got %d", len(bills))
	}

	all, err := st.GetRecentBills(0) // 0 should default to 10
	if err != nil {
		t.Fatalf("GetRecentBills(0): %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("want 5 bills with limit=0 default, got %d", len(all))
	}
}

func TestListDistinctMemberHelpers(t *testing.T) {
	conn := tempDB(t)
	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, government_level)
		VALUES ('m1', 'Alice', 'Liberal', 'Ottawa Centre', 'Ontario', 'commons', 1, 'federal'),
		       ('m2', 'Bob', 'Conservative', 'Calgary East', 'Alberta', 'commons', 1, 'federal'),
		       ('m3', 'Carol', 'NDP', 'Vancouver East', 'British Columbia', 'legislature', 1, 'provincial')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	st := store.New(conn)

	parties, err := st.ListDistinctParties()
	if err != nil {
		t.Fatalf("ListDistinctParties: %v", err)
	}
	if len(parties) != 3 {
		t.Fatalf("want 3 parties, got %d: %v", len(parties), parties)
	}

	provinces, err := st.ListDistinctProvinces()
	if err != nil {
		t.Fatalf("ListDistinctProvinces: %v", err)
	}
	if len(provinces) != 3 {
		t.Fatalf("want 3 provinces, got %d: %v", len(provinces), provinces)
	}

	ridings, err := st.ListDistinctRidings()
	if err != nil {
		t.Fatalf("ListDistinctRidings: %v", err)
	}
	if len(ridings) != 3 {
		t.Fatalf("want 3 ridings, got %d: %v", len(ridings), ridings)
	}

	fedRidings, err := st.ListDistinctRidingsByLevel("federal")
	if err != nil {
		t.Fatalf("ListDistinctRidingsByLevel: %v", err)
	}
	if len(fedRidings) != 2 {
		t.Fatalf("want 2 federal ridings, got %d: %v", len(fedRidings), fedRidings)
	}
}

func TestGetMembersByRiding(t *testing.T) {
	conn := tempDB(t)
	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, government_level)
		VALUES ('m1', 'Alice', 'Liberal', 'Ottawa Centre', 'Ontario', 'commons', 1, 'federal'),
		       ('m2', 'Bob', 'Liberal', 'Ottawa South', 'Ontario', 'legislature', 1, 'provincial')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	st := store.New(conn)

	members, err := st.GetMembersByRiding("ottawa")
	if err != nil {
		t.Fatalf("GetMembersByRiding: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("want 2 members for 'ottawa', got %d", len(members))
	}

	members, err = st.GetMembersByRiding("nonexistent")
	if err != nil {
		t.Fatalf("GetMembersByRiding nonexistent: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("want 0 members for nonexistent riding, got %d", len(members))
	}
}

func TestGetMemberVotes(t *testing.T) {
	conn := tempDB(t)
	if err := store.UpsertMember(conn, store.MemberRecord{
		ID: "m-votes", Name: "Vote Test MP", Party: "Liberal",
		Active: true, LastScraped: "2026-01-01",
	}); err != nil {
		t.Fatalf("UpsertMember: %v", err)
	}
	if err := store.UpsertBill(conn, store.BillRecord{
		ID: "b-votes", Parliament: 45, Session: 1, Number: "C-10",
		Title: "Vote Bill", LastScraped: "2026-01-01",
	}); err != nil {
		t.Fatalf("UpsertBill: %v", err)
	}
	for i := 1; i <= 3; i++ {
		divID := fmt.Sprintf("d-votes-%d", i)
		if err := store.UpsertDivision(conn, store.DivisionRecord{
			ID: divID, Parliament: 45, Session: 1, Number: i,
			Date: fmt.Sprintf("2024-01-0%d", i), BillID: "b-votes",
			Yeas: 100, Nays: 50, Result: "Agreed to", Chamber: "commons",
			LastScraped: "2026-01-01",
		}); err != nil {
			t.Fatalf("UpsertDivision: %v", err)
		}
		vote := "Yea"
		if i == 3 {
			vote = "Nay"
		}
		if err := store.UpsertMemberVote(conn, divID, "m-votes", vote); err != nil {
			t.Fatalf("UpsertMemberVote: %v", err)
		}
	}

	st := store.New(conn)
	votes, err := st.GetMemberVotes("m-votes", 10)
	if err != nil {
		t.Fatalf("GetMemberVotes: %v", err)
	}
	if len(votes) != 3 {
		t.Fatalf("want 3 votes, got %d", len(votes))
	}

	// DivisionHasVotes
	has, err := store.DivisionHasVotes(conn, "d-votes-1")
	if err != nil || !has {
		t.Fatalf("DivisionHasVotes: has=%v err=%v", has, err)
	}
	noHas, err := store.DivisionHasVotes(conn, "nonexistent")
	if err != nil || noHas {
		t.Fatalf("DivisionHasVotes nonexistent: has=%v err=%v", noHas, err)
	}
}

func TestGetMissedVotes(t *testing.T) {
	conn := tempDB(t)

	// Member with term_start, party colleague for majority calculation
	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, term_start)
		VALUES ('m-missed', 'Test MP', 'NDP', 'Some Riding', 'BC', 'commons', 1, '2024-01-01')`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}
	_, err = conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active)
		VALUES ('m-ndp2', 'NDP Colleague', 'NDP', 'Other Riding', 'BC', 'commons', 1)`)
	if err != nil {
		t.Fatalf("insert party member: %v", err)
	}

	for i := 1; i <= 3; i++ {
		_, err := conn.Exec(
			fmt.Sprintf(`INSERT INTO divisions (id, parliament, session, number, date, yeas, nays, result, chamber)
				VALUES (?, 45, 1, ?, '2024-01-0%d', 100, 50, 'Carried', 'commons')`, i),
			fmt.Sprintf("dm%d", i), i)
		if err != nil {
			t.Fatalf("insert division: %v", err)
		}
	}

	// m-missed votes only on dm1; dm2 and dm3 are missed
	_, err = conn.Exec(`INSERT INTO member_votes (division_id, member_id, vote) VALUES ('dm1', 'm-missed', 'Yea')`)
	if err != nil {
		t.Fatalf("insert vote: %v", err)
	}
	// m-ndp2 votes Yea on all three — provides party majority
	for i := 1; i <= 3; i++ {
		_, err = conn.Exec(`INSERT INTO member_votes (division_id, member_id, vote) VALUES (?, 'm-ndp2', 'Yea')`,
			fmt.Sprintf("dm%d", i))
		if err != nil {
			t.Fatalf("insert ndp2 vote: %v", err)
		}
	}

	st := store.New(conn)
	missed, err := st.GetMissedVotes("m-missed", 50)
	if err != nil {
		t.Fatalf("GetMissedVotes: %v", err)
	}
	if len(missed) != 2 {
		t.Fatalf("want 2 missed votes, got %d", len(missed))
	}
	// Most recent first
	if missed[0].Date != "2024-01-03" {
		t.Errorf("want first missed date=2024-01-03, got %s", missed[0].Date)
	}
	// Party voted Yea (m-ndp2)
	if missed[0].PartyMajority != "Yea" {
		t.Errorf("want PartyMajority=Yea, got %s", missed[0].PartyMajority)
	}
}

func TestGetMissedVotes_Split(t *testing.T) {
	conn := tempDB(t)

	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, term_start)
		VALUES ('m-split', 'Split MP', 'Liberal', 'Some Riding', 'ON', 'commons', 1, '2024-01-01'),
		       ('m-lib2', 'Lib A', 'Liberal', 'Other Riding', 'ON', 'commons', 1, NULL),
		       ('m-lib3', 'Lib B', 'Liberal', 'Another Riding', 'ON', 'commons', 1, NULL)`)
	if err != nil {
		t.Fatalf("insert members: %v", err)
	}
	_, err = conn.Exec(`INSERT INTO divisions (id, parliament, session, number, date, yeas, nays, result, chamber)
		VALUES ('ds1', 45, 1, 1, '2024-01-10', 1, 1, 'Negatived', 'commons')`)
	if err != nil {
		t.Fatalf("insert division: %v", err)
	}
	// m-lib2=Yea, m-lib3=Nay: equal split; m-split does not vote
	for _, v := range []struct{ m, vote string }{{"m-lib2", "Yea"}, {"m-lib3", "Nay"}} {
		_, err = conn.Exec(`INSERT INTO member_votes (division_id, member_id, vote) VALUES ('ds1', ?, ?)`, v.m, v.vote)
		if err != nil {
			t.Fatalf("insert vote: %v", err)
		}
	}

	st := store.New(conn)
	missed, err := st.GetMissedVotes("m-split", 50)
	if err != nil {
		t.Fatalf("GetMissedVotes: %v", err)
	}
	if len(missed) != 1 {
		t.Fatalf("want 1 missed vote, got %d", len(missed))
	}
	if missed[0].PartyMajority != "Split" {
		t.Errorf("want PartyMajority=Split, got %s", missed[0].PartyMajority)
	}
}

func TestGetMissedVotes_NoTermStart(t *testing.T) {
	conn := tempDB(t)
	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active)
		VALUES ('m-noterm', 'No Term MP', 'NDP', 'Some Riding', 'BC', 'commons', 1)`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}
	st := store.New(conn)
	missed, err := st.GetMissedVotes("m-noterm", 50)
	if err != nil {
		t.Fatalf("GetMissedVotes: %v", err)
	}
	if missed != nil {
		t.Errorf("want nil for member with no term_start, got %v", missed)
	}
}

func TestGetMissedVotes_ExcludesDivisionsBeforeTerm(t *testing.T) {
	conn := tempDB(t)
	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, term_start)
		VALUES ('m-term', 'Term MP', 'NDP', 'Some Riding', 'BC', 'commons', 1, '2024-02-01')`)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}
	// d-before is before the term; d-after is within the term
	for _, row := range []struct {
		id   string
		date string
		num  int
	}{
		{"d-before", "2024-01-15", 1},
		{"d-after", "2024-02-10", 2},
	} {
		_, err = conn.Exec(`INSERT INTO divisions (id, parliament, session, number, date, yeas, nays, result, chamber)
			VALUES (?, 45, 1, ?, ?, 100, 50, 'Carried', 'commons')`,
			row.id, row.num, row.date)
		if err != nil {
			t.Fatalf("insert division %s: %v", row.id, err)
		}
	}
	// m-term does not vote on either
	st := store.New(conn)
	missed, err := st.GetMissedVotes("m-term", 50)
	if err != nil {
		t.Fatalf("GetMissedVotes: %v", err)
	}
	if len(missed) != 1 {
		t.Fatalf("want 1 missed vote (only d-after is in term), got %d", len(missed))
	}
	if missed[0].DivisionID != "d-after" {
		t.Errorf("want DivisionID=d-after, got %s", missed[0].DivisionID)
	}
}

func TestUpsertProfiles(t *testing.T) {
	conn := tempDB(t)
	members := []store.MemberRecord{
		{ID: "m-profile-1", Name: "Profile MP One", Party: "Liberal", Active: true, LastScraped: "2026-01-01"},
		{ID: "m-profile-2", Name: "Profile MP Two", Party: "NDP", Active: true, LastScraped: "2026-01-01"},
	}
	// UpsertProfiles batches multiple UpsertMember calls with a delay between each.
	store.UpsertProfiles(conn, members, 0)
	st := store.New(conn)
	for _, m := range members {
		got, err := st.GetMember(m.ID)
		if err != nil {
			t.Fatalf("GetMember %s: %v", m.ID, err)
		}
		if got.Name != m.Name {
			t.Errorf("member %s name=%q want %q", m.ID, got.Name, m.Name)
		}
	}
}

func TestUpsertSittingDateAndSittingDates(t *testing.T) {
	conn := tempDB(t)
	for _, d := range []string{"2026-01-01", "2026-01-02", "2026-01-08"} {
		if err := store.UpsertSittingDate(conn, 45, 1, d); err != nil {
			t.Fatalf("UpsertSittingDate: %v", err)
		}
	}
	// idempotent second insert
	if err := store.UpsertSittingDate(conn, 45, 1, "2026-01-01"); err != nil {
		t.Fatalf("UpsertSittingDate duplicate: %v", err)
	}
	dates, err := store.SittingDates(conn, 45, 1)
	if err != nil {
		t.Fatalf("SittingDates: %v", err)
	}
	if len(dates) != 3 {
		t.Fatalf("want 3 sitting dates, got %d: %v", len(dates), dates)
	}
	if dates[0] != "2026-01-01" {
		t.Errorf("dates[0]=%q want 2026-01-01", dates[0])
	}
}

func TestDivisionExists(t *testing.T) {
	conn := tempDB(t)
	exists, err := store.DivisionExists(conn, "d-none")
	if err != nil || exists {
		t.Fatalf("DivisionExists before insert: %v %v", exists, err)
	}
	if err := store.UpsertDivision(conn, store.DivisionRecord{
		ID: "d-exists", Parliament: 45, Session: 1, Number: 1,
		Yeas: 100, Nays: 50, Result: "Agreed to", Chamber: "commons",
		LastScraped: "2026-01-01",
	}); err != nil {
		t.Fatalf("UpsertDivision: %v", err)
	}
	exists, err = store.DivisionExists(conn, "d-exists")
	if err != nil || !exists {
		t.Fatalf("DivisionExists after insert: %v %v", exists, err)
	}
}

func TestVerifyEmailCode_InvalidAndExpired(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	// Wrong code
	if _, err := st.VerifyEmailCode("nobody@example.com", "000000"); err == nil {
		t.Fatal("expected error for invalid code")
	}

	// Empty inputs
	if _, err := st.VerifyEmailCode("", "123456"); err == nil {
		t.Fatal("expected error for empty email")
	}
	if _, err := st.VerifyEmailCode("a@example.com", ""); err == nil {
		t.Fatal("expected error for empty code")
	}

	// Valid code flow: create verification, then verify with correct code.
	_, code, err := st.CreateEmailVerification("code2@example.com", time.Hour)
	if err != nil {
		t.Fatalf("CreateEmailVerification: %v", err)
	}
	u, err := st.VerifyEmailCode("code2@example.com", code)
	if err != nil {
		t.Fatalf("VerifyEmailCode valid: %v", err)
	}
	if !u.EmailVerified {
		t.Fatal("expected user verified after code verification")
	}

	// Code reuse should fail.
	if _, err := st.VerifyEmailCode("code2@example.com", code); err == nil {
		t.Fatal("expected error on code reuse")
	}
}

func TestOrdinal(t *testing.T) {
	// ordinal is unexported but exercised via GetParliamentStatus label.
	// We test indirectly: parliament 1 → "1st", 2 → "2nd", 3 → "3rd",
	// 11 → "11th" (special case), 21 → "21st".
	conn := tempDB(t)
	st := store.New(conn)
	cases := []struct{ parl int; want string }{
		{1, "1st"},
		{2, "2nd"},
		{3, "3rd"},
		{4, "4th"},
		{11, "11th"},
		{12, "12th"},
		{13, "13th"},
		{21, "21st"},
	}
	for _, tc := range cases {
		ps, err := st.GetParliamentStatus(tc.parl, 1)
		if err != nil {
			t.Fatalf("GetParliamentStatus(%d): %v", tc.parl, err)
		}
		if !strings.Contains(ps.Label, tc.want) {
			t.Errorf("parliament=%d label=%q, want ordinal %q", tc.parl, ps.Label, tc.want)
		}
	}
}

func TestReplaceLegislatureCalendarDates(t *testing.T) {
	conn := tempDB(t)
	dates := []string{"2026-02-01", "2026-02-02", "2026-02-03"}
	if err := store.ReplaceLegislatureCalendarDates(conn, "provincial-ON", dates, "2026-01-01T00:00:00"); err != nil {
		t.Fatalf("ReplaceLegislatureCalendarDates: %v", err)
	}

	got, err := store.LegislatureCalendarDates(conn, "provincial-ON")
	if err != nil {
		t.Fatalf("LegislatureCalendarDates: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 dates, got %d: %v", len(got), got)
	}

	// Replace with a smaller set — old dates should be removed.
	if err := store.ReplaceLegislatureCalendarDates(conn, "provincial-ON", []string{"2026-03-01"}, "2026-02-01T00:00:00"); err != nil {
		t.Fatalf("ReplaceLegislatureCalendarDates replace: %v", err)
	}
	got, err = store.LegislatureCalendarDates(conn, "provincial-ON")
	if err != nil {
		t.Fatalf("LegislatureCalendarDates after replace: %v", err)
	}
	if len(got) != 1 || got[0] != "2026-03-01" {
		t.Fatalf("want [2026-03-01], got %v", got)
	}
}

func TestReplaceLegislatureCalendarDates_EdgeCases(t *testing.T) {
	conn := tempDB(t)

	// Empty jurisdiction must return an error.
	if err := store.ReplaceLegislatureCalendarDates(conn, "", []string{"2026-01-01"}, ""); err == nil {
		t.Fatal("expected error for empty jurisdiction")
	}

	// Empty dates list clears existing rows for the jurisdiction.
	if err := store.ReplaceLegislatureCalendarDates(conn, "provincial-BC", []string{"2026-03-01", "2026-03-02"}, ""); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := store.ReplaceLegislatureCalendarDates(conn, "provincial-BC", nil, ""); err != nil {
		t.Fatalf("replace with nil dates: %v", err)
	}
	got, err := store.LegislatureCalendarDates(conn, "provincial-BC")
	if err != nil {
		t.Fatalf("LegislatureCalendarDates: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 dates after empty replace, got %v", got)
	}
}

func TestVerifyEmailToken_ErrorPaths(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	// Empty token should return an error immediately.
	if _, err := st.VerifyEmailToken(""); err == nil {
		t.Fatal("expected error for empty token")
	}

	// Non-existent token should return an error.
	if _, err := st.VerifyEmailToken("no-such-token-xyz"); err == nil {
		t.Fatal("expected error for unknown token")
	}

	// Expired token should return an error.
	token, _, err := st.CreateEmailVerification("expire@example.com", time.Hour)
	if err != nil {
		t.Fatalf("CreateEmailVerification: %v", err)
	}
	// Back-date the expiry so the token is already expired.
	if _, err := conn.Exec(`UPDATE email_verification_tokens SET expires_at = '2000-01-01T00:00:00Z' WHERE email = 'expire@example.com'`); err != nil {
		t.Fatalf("backdating token: %v", err)
	}
	if _, err := st.VerifyEmailToken(token); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestListDivisions_DefaultPagination(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	// Insert two divisions.
	for _, id := range []int{1, 2} {
		if _, err := conn.Exec(`INSERT INTO divisions (id, parliament, session, number, yeas, nays, paired) VALUES (?,44,1,?,10,5,0)`, id, id); err != nil {
			t.Fatalf("insert division %d: %v", id, err)
		}
	}

	// page=0 and perPage=0 should apply defaults (page→1, perPage→50).
	divs, total, err := st.ListDivisions(0, 0)
	if err != nil {
		t.Fatalf("ListDivisions: %v", err)
	}
	if total != 2 {
		t.Fatalf("want total=2, got %d", total)
	}
	if len(divs) != 2 {
		t.Fatalf("want 2 divisions, got %d", len(divs))
	}
}

func TestGetCombinedJurisdictionStatus_EdgeCases(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	// No jurisdictions → unavailable.
	status, err := st.GetCombinedJurisdictionStatus()
	if err != nil {
		t.Fatalf("no-arg: %v", err)
	}
	if status != "status_unavailable" {
		t.Fatalf("want status_unavailable, got %q", status)
	}

	// Whitespace-only jurisdiction strings are skipped → unavailable.
	status, err = st.GetCombinedJurisdictionStatus("   ", "")
	if err != nil {
		t.Fatalf("whitespace only: %v", err)
	}
	if status != "status_unavailable" {
		t.Fatalf("want status_unavailable for whitespace, got %q", status)
	}

	// One jurisdiction has no data, one is on_break → result is on_break.
	// Use a date 10 days ago to be beyond the 3-day in-session grace window.
	tenDaysAgo := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02")
	if err := store.ReplaceLegislatureCalendarDates(conn, "fed", []string{tenDaysAgo}, ""); err != nil {
		t.Fatalf("setup fed: %v", err)
	}
	status, err = st.GetCombinedJurisdictionStatus("fed", "no-data-jur")
	if err != nil {
		t.Fatalf("mixed jurisdictions: %v", err)
	}
	if status != "on_break" {
		t.Fatalf("want on_break, got %q", status)
	}
}

func TestReactToBill_InvalidReaction(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	if err := st.ReactToBill("react@example.com", "bill-1", "thumbsup", ""); err == nil {
		t.Fatal("expected error for invalid reaction type")
	}
}

func TestSaveUserCategoryPreferences_SkipsEmptyCategories(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	u, err := st.UpsertUser("catpref@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	// Save with a mix of valid and empty/whitespace categories.
	if err := st.SaveUserCategoryPreferences(u.ID, []string{"health", "  ", "", "environment"}); err != nil {
		t.Fatalf("SaveUserCategoryPreferences: %v", err)
	}

	cats, err := st.GetUserCategoryPreferences(u.ID)
	if err != nil {
		t.Fatalf("GetUserCategoryPreferences: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("want 2 categories (empty ones skipped), got %d: %v", len(cats), cats)
	}
}

func TestGetUserBySession_ExpiredSession(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	u, err := st.UpsertUser("expiredses@example.com")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	sid, err := st.CreateSession(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Back-date the session so it's already expired.
	if _, err := conn.Exec(`UPDATE user_sessions SET expires_at = '2000-01-01T00:00:00Z'`); err != nil {
		t.Fatalf("backdating session: %v", err)
	}

	if _, err := st.GetUserBySession(sid); err == nil {
		t.Fatal("expected error for expired session")
	}
}

func TestUpdateUserLocation_EmptyUserID(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	if _, err := st.UpdateUserLocation("", "fed-riding", "prov-riding"); err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestGetJurisdictionStatus_NoData(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	// No rows for this jurisdiction → must return "status_unavailable".
	status, err := st.GetJurisdictionStatus("nonexistent-jur")
	if err != nil {
		t.Fatalf("GetJurisdictionStatus: %v", err)
	}
	if status != "status_unavailable" {
		t.Fatalf("want status_unavailable, got %q", status)
	}
}

func TestGetRecentDivisions_DefaultLimit(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	// Insert one division so the function returns real results.
	if _, err := conn.Exec(`INSERT INTO divisions (id, parliament, session, number, yeas, nays, paired) VALUES (1,44,1,1,10,5,0)`); err != nil {
		t.Fatalf("insert division: %v", err)
	}

	// limit=0 should trigger the default limit=10 branch.
	divs, err := st.GetRecentDivisions(0)
	if err != nil {
		t.Fatalf("GetRecentDivisions: %v", err)
	}
	if len(divs) != 1 {
		t.Fatalf("want 1 division, got %d", len(divs))
	}
}

func TestListBills_ProvinceLevelFilters(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	if _, err := conn.Exec(`INSERT INTO bills (id, parliament, session, number, title, category, current_stage, chamber, last_activity_date)
		VALUES
		('fed-1', 45, 1, 'C-10', 'Federal Bill', 'General', '1st_reading', 'commons', '2026-01-01'),
		('on-1', 45, 1, 'ON-5', 'Ontario Bill', 'General', '1st_reading', 'legislature', '2026-01-02')`); err != nil {
		t.Fatalf("insert bills: %v", err)
	}

	// Level "federal" should return only commons/senate bills.
	bills, _, err := st.ListBills(store.BillFilter{Level: "federal", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills federal: %v", err)
	}
	if len(bills) != 1 || bills[0].ID != "fed-1" {
		t.Errorf("federal filter: want [fed-1], got %v", bills)
	}

	// Level "provincial" should return non-commons/senate bills.
	bills, _, err = st.ListBills(store.BillFilter{Level: "provincial", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills provincial: %v", err)
	}
	if len(bills) != 1 || bills[0].ID != "on-1" {
		t.Errorf("provincial filter: want [on-1], got %v", bills)
	}

	// Search filter on title.
	bills, _, err = st.ListBills(store.BillFilter{Search: "Ontario", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("ListBills search: %v", err)
	}
	if len(bills) != 1 || bills[0].ID != "on-1" {
		t.Errorf("search filter: want [on-1], got %v", bills)
	}
}

func TestAuthenticateOAuth_NotMarkingVerified(t *testing.T) {
	conn := tempDB(t)
	st := store.New(conn)

	u, err := st.AuthenticateOAuth("github", "gh-999", "nomark@example.com", false)
	if err != nil {
		t.Fatalf("AuthenticateOAuth: %v", err)
	}
	if u.EmailVerified {
		t.Fatalf("expected email_verified=false when markEmailVerified=false")
	}
}

func TestTutorialProgress(t *testing.T) {
conn := tempDB(t)
st := store.New(conn)

u, err := st.UpsertUser("tutorial@example.com")
if err != nil {
t.Fatalf("UpsertUser: %v", err)
}

// Initially empty, not dismissed.
tp, err := st.GetTutorialProgress(u.ID)
if err != nil {
t.Fatalf("GetTutorialProgress initial: %v", err)
}
if tp.Dismissed {
t.Error("expected not dismissed initially")
}
if len(tp.Done) != 0 {
t.Errorf("expected 0 done activities initially, got %d", len(tp.Done))
}

// Complete view_bill.
if err := st.CompleteTutorialActivity(u.ID, store.TutorialViewBill); err != nil {
t.Fatalf("CompleteTutorialActivity view_bill: %v", err)
}
// Idempotent — second call must not error.
if err := st.CompleteTutorialActivity(u.ID, store.TutorialViewBill); err != nil {
t.Fatalf("CompleteTutorialActivity view_bill idempotent: %v", err)
}

tp, err = st.GetTutorialProgress(u.ID)
if err != nil {
t.Fatalf("GetTutorialProgress after view_bill: %v", err)
}
if !tp.Done[store.TutorialViewBill] {
t.Error("expected view_bill to be done")
}
if tp.Done[store.TutorialViewRep] {
t.Error("expected view_rep not yet done")
}
if tp.Dismissed {
t.Error("expected not dismissed after completing one activity")
}

// Complete all steps.
for _, def := range store.AllTutorialStepDefs {
if err := st.CompleteTutorialActivity(u.ID, def.Key); err != nil {
t.Fatalf("CompleteTutorialActivity %q: %v", def.Key, err)
}
}
tp, err = st.GetTutorialProgress(u.ID)
if err != nil {
t.Fatalf("GetTutorialProgress after all steps: %v", err)
}
for _, def := range store.AllTutorialStepDefs {
if !tp.Done[def.Key] {
t.Errorf("expected %q done after completing all steps", def.Key)
}
}

// Dismiss.
if err := st.DismissTutorial(u.ID); err != nil {
t.Fatalf("DismissTutorial: %v", err)
}
tp, err = st.GetTutorialProgress(u.ID)
if err != nil {
t.Fatalf("GetTutorialProgress after dismiss: %v", err)
}
if !tp.Dismissed {
t.Error("expected dismissed after DismissTutorial")
}
}
