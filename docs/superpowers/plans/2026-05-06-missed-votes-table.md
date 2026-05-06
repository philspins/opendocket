# Missed Votes Table Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a paginated Missed Votes table to the MP detail page, and update MissedPct to use the member's term dates as scope (instead of current parliament/session).

**Architecture:** Add `MissedVoteRow` to the store models, implement `GetMissedVotes` using a `NOT EXISTS` subquery filtered by term dates, update `GetMemberStats` to use term dates for the missed-vote denominator, thread the new data through the handler and template.

**Tech Stack:** Go, SQLite, [templ](https://templ.guide) (`.templ` files compile to `*_templ.go` via `make templ`)

---

## File Map

| File | Change |
|---|---|
| `internal/store/models.go` | Add `MissedVoteRow` struct |
| `internal/store/store.go` | Add `GetMissedVotes`; update `GetMemberStats` term-date scope |
| `internal/store/store_test.go` | Add `TestGetMissedVotes*`; add `TestGetMemberStats_MissedPct_UsesTermDates` |
| `internal/templates/member_profile.templ` | Add `missedVotes` param; add Missed Votes section |
| `internal/templates/member_profile_templ.go` | Auto-generated — regenerate via `make templ` |
| `internal/server/server.go` | Call `GetMissedVotes`; pass result to template |
| `internal/server/server_test.go` | Update `TestHandleMemberProfile_RendersPage` with term_start |

---

## Task 1: Add `MissedVoteRow` model

**Files:**
- Modify: `internal/store/models.go`

- [ ] **Step 1: Add the struct**

  Open `internal/store/models.go` and append after the `CategoryScore` struct (end of file):

  ```go
  // MissedVoteRow represents a division that occurred during a member's term
  // where the member has no recorded vote.
  type MissedVoteRow struct {
  	DivisionID    string
  	Date          string
  	BillID        string
  	BillNumber    string
  	BillTitle     string
  	Description   string
  	PartyMajority string // "Yea", "Nay", or "Split"
  	Result        string
  }
  ```

- [ ] **Step 2: Verify it compiles**

  ```bash
  go build ./...
  ```
  Expected: no output (clean build).

- [ ] **Step 3: Commit**

  ```bash
  git add internal/store/models.go
  git commit -m "feat: add MissedVoteRow model"
  ```

---

## Task 2: Implement `GetMissedVotes` (TDD)

**Files:**
- Modify: `internal/store/store_test.go`
- Modify: `internal/store/store.go`

- [ ] **Step 1: Write the failing tests**

  Add to `internal/store/store_test.go` (after `TestGetMemberVotes`):

  ```go
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
  		       ('m-lib2', 'Lib A', 'Liberal', 'Other Riding', 'ON', 'commons', 1),
  		       ('m-lib3', 'Lib B', 'Liberal', 'Another Riding', 'ON', 'commons', 1)`)
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
  ```

- [ ] **Step 2: Run the tests to confirm they fail**

  ```bash
  go test ./internal/store/... -run "TestGetMissedVotes" -v
  ```
  Expected: `FAIL` — `st.GetMissedVotes undefined`

- [ ] **Step 3: Implement `GetMissedVotes` in `store.go`**

  Add after `GetMemberVotes` (around line 635 in `internal/store/store.go`):

  ```go
  // GetMissedVotes returns divisions that occurred during the member's term where
  // the member has no recorded vote, ordered most-recent first.
  // Returns nil (not an error) when the member has no term_start set.
  func (s *Store) GetMissedVotes(id string, limit int) ([]MissedVoteRow, error) {
  	if limit <= 0 {
  		limit = 50
  	}

  	var termStart, termEnd string
  	if err := s.db.QueryRow(
  		"SELECT COALESCE(term_start,''), COALESCE(term_end,'') FROM members WHERE id = ?", id,
  	).Scan(&termStart, &termEnd); err != nil {
  		return nil, nil
  	}
  	if termStart == "" {
  		return nil, nil
  	}

  	var party string
  	_ = s.db.QueryRow("SELECT COALESCE(party,'') FROM members WHERE id = ?", id).Scan(&party)

  	rows, err := s.db.Query(`
  		SELECT d.id, COALESCE(d.date,''), COALESCE(d.bill_id,''),
  		       COALESCE(b.number,''),
  		       COALESCE(NULLIF(b.short_title,''), NULLIF(b.title,''), ''),
  		       COALESCE(d.description,''), COALESCE(d.result,'')
  		FROM divisions d
  		LEFT JOIN bills b ON b.id = d.bill_id
  		WHERE d.date >= ?
  		  AND (? = '' OR d.date <= ?)
  		  AND NOT EXISTS (
  		    SELECT 1 FROM member_votes mv
  		    WHERE mv.member_id = ? AND mv.division_id = d.id
  		  )
  		ORDER BY d.date DESC
  		LIMIT ?`,
  		termStart, termEnd, termEnd, id, limit)
  	if err != nil {
  		return nil, err
  	}
  	defer rows.Close()

  	type rawRow struct {
  		divisionID  string
  		date        string
  		billID      string
  		billNumber  string
  		billTitle   string
  		description string
  		result      string
  	}
  	var rawRows []rawRow
  	for rows.Next() {
  		var r rawRow
  		if err := rows.Scan(&r.divisionID, &r.date, &r.billID, &r.billNumber,
  			&r.billTitle, &r.description, &r.result); err != nil {
  			return nil, err
  		}
  		rawRows = append(rawRows, r)
  	}
  	if err := rows.Err(); err != nil {
  		return nil, err
  	}
  	rows.Close()

  	if len(rawRows) == 0 {
  		return nil, nil
  	}

  	divIDs := make([]string, len(rawRows))
  	for i, r := range rawRows {
  		divIDs[i] = r.divisionID
  	}
  	partyMajorityMap, _ := s.batchPartyMajority(divIDs, party, "")
  	if partyMajorityMap == nil {
  		partyMajorityMap = make(map[string]string)
  	}

  	result := make([]MissedVoteRow, len(rawRows))
  	for i, r := range rawRows {
  		majority := partyMajorityMap[r.divisionID]
  		if majority == "" {
  			majority = "Split"
  		}
  		result[i] = MissedVoteRow{
  			DivisionID:    r.divisionID,
  			Date:          r.date,
  			BillID:        r.billID,
  			BillNumber:    r.billNumber,
  			BillTitle:     r.billTitle,
  			Description:   r.description,
  			PartyMajority: majority,
  			Result:        r.result,
  		}
  	}
  	return result, nil
  }
  ```

- [ ] **Step 4: Run the tests to confirm they pass**

  ```bash
  go test ./internal/store/... -run "TestGetMissedVotes" -v
  ```
  Expected: all four tests `PASS`.

- [ ] **Step 5: Verify full build**

  ```bash
  go build ./...
  ```
  Expected: no output.

- [ ] **Step 6: Commit**

  ```bash
  git add internal/store/store.go internal/store/store_test.go
  git commit -m "feat: add GetMissedVotes store function"
  ```

---

## Task 3: Update `GetMemberStats` to use term-date scope (TDD)

**Files:**
- Modify: `internal/store/store_test.go`
- Modify: `internal/store/store.go`

- [ ] **Step 1: Write the failing test**

  Add to `internal/store/store_test.go` (after `TestGetMemberStats_SolePartyMemberCountsAsPartyLine`):

  ```go
  func TestGetMemberStats_MissedPct_UsesTermDates(t *testing.T) {
  	conn := tempDB(t)

  	// term_start is 2024-02-01, so d-before (2024-01-15) must NOT count.
  	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, term_start)
  		VALUES ('m-term', 'Term MP', 'Liberal', 'Ottawa Centre', 'ON', 'commons', 1, '2024-02-01'),
  		       ('m-lib2', 'Lib Colleague', 'Liberal', 'Ottawa West', 'ON', 'commons', 1)`)
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
  ```

- [ ] **Step 2: Run the test to confirm it fails**

  ```bash
  go test ./internal/store/... -run "TestGetMemberStats_MissedPct_UsesTermDates" -v
  ```
  Expected: `FAIL` — the current parliament/session logic yields `MissedPct=33` (3 divisions in parliament 45/session 1, 1 voted) rather than `50`.

- [ ] **Step 3: Replace the parliament/session scope block in `GetMemberStats`**

  In `internal/store/store.go`, find and replace this block (around lines 711–750):

  ```go
  	// Derive the current parliament/session from the member's most recent votes
  	// so that provincial members (who use different parliament numbers) get the
  	// right denominator rather than being compared against federal divisions.
  	var currentParliament, currentSession int
  	_ = s.db.QueryRow(`
  		SELECT d.parliament, d.session
  		FROM member_votes mv
  		JOIN divisions d ON d.id = mv.division_id
  		WHERE mv.member_id = ?
  		ORDER BY d.parliament DESC, d.session DESC
  		LIMIT 1`, id).Scan(&currentParliament, &currentSession)

  	var totalDivisions int
  	if currentParliament > 0 {
  		_ = s.db.QueryRow(`
  			SELECT COUNT(*) FROM divisions
  			WHERE parliament = ? AND session = ?`,
  			currentParliament, currentSession).Scan(&totalDivisions)
  	}

  	var voted int
  	if currentParliament > 0 {
  		_ = s.db.QueryRow(`
  			SELECT COUNT(*) FROM member_votes mv
  			JOIN divisions d ON d.id = mv.division_id
  			WHERE mv.member_id = ? AND d.parliament = ? AND d.session = ?`,
  			id, currentParliament, currentSession).Scan(&voted)
  	}
  ```

  Replace with:

  ```go
  	var termStart, termEnd string
  	_ = s.db.QueryRow(
  		"SELECT COALESCE(term_start,''), COALESCE(term_end,'') FROM members WHERE id = ?", id,
  	).Scan(&termStart, &termEnd)

  	var totalDivisions int
  	if termStart != "" {
  		_ = s.db.QueryRow(`
  			SELECT COUNT(*) FROM divisions
  			WHERE date >= ? AND (? = '' OR date <= ?)`,
  			termStart, termEnd, termEnd).Scan(&totalDivisions)
  	}

  	var voted int
  	if termStart != "" {
  		_ = s.db.QueryRow(`
  			SELECT COUNT(*) FROM member_votes mv
  			JOIN divisions d ON d.id = mv.division_id
  			WHERE mv.member_id = ? AND d.date >= ? AND (? = '' OR d.date <= ?)`,
  			id, termStart, termEnd, termEnd).Scan(&voted)
  	}
  ```

- [ ] **Step 4: Run the failing test again to confirm it now passes**

  ```bash
  go test ./internal/store/... -run "TestGetMemberStats_MissedPct_UsesTermDates" -v
  ```
  Expected: `PASS`

- [ ] **Step 5: Run all store tests to check for regressions**

  ```bash
  go test ./internal/store/... -v
  ```
  Expected: all tests pass. (The existing `TestGetMemberStats_Basic` tests don't set `term_start`, so `MissedPct` will be `0` for them — the test does not assert `MissedPct`, so no regression.)

- [ ] **Step 6: Commit**

  ```bash
  git add internal/store/store.go internal/store/store_test.go
  git commit -m "feat: scope MissedPct to member term dates"
  ```

---

## Task 4: Update handler

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`

- [ ] **Step 1: Update `handleMemberProfile` in `server.go`**

  Find this block (around line 452 in `internal/server/server.go`):

  ```go
  	votes, _ := s.store.GetMemberVotes(id, 500)
  	stats, _ := s.store.GetMemberStats(id)
  	catScores, _ := s.store.GetMemberCategoryScores(id)
  	_ = templates.MemberProfile(ps, member, votes, stats, catScores).Render(r.Context(), w)
  ```

  Replace with:

  ```go
  	votes, _ := s.store.GetMemberVotes(id, 500)
  	stats, _ := s.store.GetMemberStats(id)
  	catScores, _ := s.store.GetMemberCategoryScores(id)
  	missedVotes, _ := s.store.GetMissedVotes(id, 500)
  	_ = templates.MemberProfile(ps, member, votes, stats, catScores, missedVotes).Render(r.Context(), w)
  ```

- [ ] **Step 2: Update `TestHandleMemberProfile_RendersPage` in `server_test.go`**

  Find the member INSERT in `TestHandleMemberProfile_RendersPage` (around line 1825):

  ```go
  	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, government_level)
  		VALUES ('m-profile', 'Profile MP', 'NDP', 'Vancouver East', 'British Columbia', 'commons', 1, 'federal')`)
  ```

  Replace with:

  ```go
  	_, err := conn.Exec(`INSERT INTO members (id, name, party, riding, province, chamber, active, government_level, term_start)
  		VALUES ('m-profile', 'Profile MP', 'NDP', 'Vancouver East', 'British Columbia', 'commons', 1, 'federal', '2024-01-01')`)
  ```

- [ ] **Step 3: Confirm expected compile error**

  ```bash
  go build ./...
  ```

  Expected: compile error — `templates.MemberProfile` does not yet accept a 6th argument. This is intentional and will be resolved in Task 5 when the template signature is updated.

- [ ] **Step 4: Commit (will be buildable after Task 5)**

  Skip commit until Task 5 completes — the code won't compile until the template signature is updated.

---

## Task 5: Update template and regenerate

**Files:**
- Modify: `internal/templates/member_profile.templ`
- Regenerate: `internal/templates/member_profile_templ.go` (via `make templ`)

- [ ] **Step 1: Update the `MemberProfile` signature in `member_profile.templ`**

  Find the first line of the component:

  ```go
  templ MemberProfile(ps store.ParliamentStatus, member store.MemberRow, votes []store.VoteRow, stats store.MemberStats, catScores []store.CategoryScore) {
  ```

  Replace with:

  ```go
  templ MemberProfile(ps store.ParliamentStatus, member store.MemberRow, votes []store.VoteRow, stats store.MemberStats, catScores []store.CategoryScore, missedVotes []store.MissedVoteRow) {
  ```

- [ ] **Step 2: Add the Missed Votes section**

  Find the closing `</section>` tag of the Recent Votes section (around line 141 of `member_profile.templ`):

  ```
  			@VotesPagination("member-votes", "[data-vote-row]", MemberVotesPerPage)
  		</section>
  ```

  Replace with:

  ```
  			@VotesPagination("member-votes", "[data-vote-row]", MemberVotesPerPage)
  		</section>

  			<!-- Missed Votes -->
  			<section id="missed-votes-section">
  				<h2 class="text-lg font-semibold text-gray-800 mb-3">Missed Votes</h2>
  				<div class="table-shell vote-table-shell">
  					<table class="vote-table member-vote-table min-w-full text-sm">
  						<thead>
  							<tr>
  								<th class="px-4 py-2.5 col-date">Date</th>
  								<th class="px-4 py-2.5">Bill</th>
  								<th class="px-4 py-2.5">Description</th>
  								<th class="px-4 py-2.5">Party Vote</th>
  								<th class="px-4 py-2.5">Result</th>
  							</tr>
  						</thead>
  						<tbody>
  							for i, mv := range missedVotes {
  								<tr
  									class={ templ.KV("hidden", i >= MemberVotesPerPage) }
  									data-missed-vote-row=""
  								>
  									<td class="px-4 py-2.5 col-date">{ FormatDate(mv.Date) }</td>
  									<td class="px-4 py-2.5 col-bill">
  										if mv.BillNumber != "" {
  											<a href={ templ.SafeURL("/bills/" + mv.BillID) } class="hover:underline">{ mv.BillNumber }</a>
  										}
  									</td>
  									<td class="px-4 py-2.5 col-description">{ mv.Description }</td>
  									<td class={ "px-4 py-2.5 col-vote", VoteBadgeClass(mv.PartyMajority) }>{ mv.PartyMajority }</td>
  									<td class="px-4 py-2.5 col-meta">{ mv.Result }</td>
  								</tr>
  							}
  							if len(missedVotes) == 0 {
  								<tr>
  									<td colspan="5" class="px-4 py-8 text-center text-gray-500">No missed votes recorded.</td>
  								</tr>
  							}
  						</tbody>
  					</table>
  				</div>
  				@VotesPagination("missed-votes", "[data-missed-vote-row]", MemberVotesPerPage)
  			</section>
  ```

- [ ] **Step 3: Regenerate the templ output**

  ```bash
  make templ
  ```
  Expected: regenerates `internal/templates/member_profile_templ.go` with no errors.

- [ ] **Step 4: Verify full build**

  ```bash
  go build ./...
  ```
  Expected: no output.

- [ ] **Step 5: Run all tests**

  ```bash
  go test ./...
  ```
  Expected: all tests pass.

- [ ] **Step 6: Commit**

  ```bash
  git add internal/templates/member_profile.templ internal/templates/member_profile_templ.go \
          internal/server/server.go internal/server/server_test.go
  git commit -m "feat: add Missed Votes table to MP detail page (#89)"
  ```
