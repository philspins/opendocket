# Missed Votes Table — MP Detail Page

**Issue:** [#89](https://github.com/philspins/opendocket/issues/89)
**Date:** 2026-05-06

## Summary

Add a paginated Missed Votes table to the MP detail page, placed below the existing Recent Votes table. A "missed" vote is any division that occurred during the member's current term where the member has no record in `member_votes`. Update `MissedPct` to use the same term-date scope so the stat and the table are consistent.

---

## Data Model

**New type in `internal/store/models.go`:**

```go
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

`PartyMajority` is display-ready: the existing `batchPartyMajority` helper returns `""` for a split; `GetMissedVotes` maps `""` → `"Split"` before returning.

---

## Store

### New: `GetMissedVotes(id string, limit int) ([]MissedVoteRow, error)`

1. Look up `term_start` and `term_end` from `members` for the given `id`. If `term_start` is empty, return nil (no term data available to scope against).
2. Query all divisions where the member has no `member_votes` record, filtered to `d.date >= term_start AND (term_end = '' OR d.date <= term_end)`, ordered by `d.date DESC`, limited to `limit`.
3. Resolve party majority via `batchPartyMajority` for the fetched division IDs; map `""` → `"Split"`.
4. Return `[]MissedVoteRow`.

SQL shape:
```sql
SELECT d.id, d.date, COALESCE(d.bill_id,''), COALESCE(b.number,''),
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
LIMIT ?
```

### Updated: `GetMemberStats`

Replace the current parliament/session-based scope for `totalDivisions` and `voted` with term-date-based counts:

- `totalDivisions`: `COUNT(*) FROM divisions WHERE date >= term_start AND (term_end = '' OR date <= term_end)`
- `voted`: `COUNT(*) FROM member_votes mv JOIN divisions d ON d.id = mv.division_id WHERE mv.member_id = ? AND d.date >= term_start AND (term_end = '' OR d.date <= term_end)`

`MissedPct` is then derived as before: `(missed * 100) / totalDivisions`.

---

## Handler

`handleMemberProfile` in `internal/server/server.go` adds one call:

```go
missedVotes, _ := s.store.GetMissedVotes(id, 500)
```

And passes it as an additional argument to `templates.MemberProfile(...)`.

---

## Template

**Updated signature** in `internal/templates/member_profile.templ`:

```go
templ MemberProfile(
    ps store.ParliamentStatus,
    member store.MemberRow,
    votes []store.VoteRow,
    stats store.MemberStats,
    catScores []store.CategoryScore,
    missedVotes []store.MissedVoteRow,
)
```

**New section** inserted immediately after the closing `</section>` of the Recent Votes table:

- Section heading: "Missed Votes"
- Table columns: Date | Bill | Description | Party Vote | Result
- Rows hidden/shown via the existing `VotesPagination` component with ID `"missed-votes"` and attribute `data-missed-vote-row=""`
- Empty state: "No missed votes recorded." when `len(missedVotes) == 0`
- `PartyMajority` rendered with `VoteBadgeClass`: `"Yea"` → `vote-yea`, `"Nay"` → `vote-nay`, `"Split"` → `vote-other` (neutral grey, handled by the default case)

---

## Testing

| Layer | What to test |
|---|---|
| `GetMissedVotes` | Member with term dates, some voted divisions and some not — verifies correct rows returned, party majority mapping including `"Split"` |
| `GetMemberStats` | `MissedPct` uses term-date scope (not parliament/session) |
| `TestHandleMemberProfile_RendersPage` | Handler passes `missedVotes` to the template; page renders without error |

---

## Out of Scope

- Changing how `batchPartyMajority` works internally
- Adding missed votes to the compare page
- Filtering/searching within the missed votes table
