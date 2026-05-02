package store

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// ── write-side record types ────────────────────────────────────────────────────
// These carry all fields a crawler writes, including LastScraped, which the
// read-side Row types (MemberRow, BillRow, etc.) deliberately omit.

// MemberRecord is the write-side representation of a member, used by scrapers.
type MemberRecord struct {
	ID              string
	Name            string
	Party           string
	Riding          string
	Province        string
	Role            string
	PhotoURL        string
	Email           string
	Website         string
	Chamber         string
	Active          bool
	LastScraped     string
	GovernmentLevel string // "federal" | "provincial"
}

// BillRecord is the write-side representation of a bill, used by scrapers.
type BillRecord struct {
	ID               string
	Parliament       int
	Session          int
	Number           string
	Title            string
	ShortTitle       string
	BillType         string
	Chamber          string
	SponsorID        string
	CurrentStage     string
	CurrentStatus    string
	Category         string
	SummaryAI        string
	FullTextURL      string
	LegisInfoURL     string
	IntroducedDate   string
	LastActivityDate string
	LastScraped      string
}

// DivisionRecord is the write-side representation of a division, used by scrapers.
type DivisionRecord struct {
	ID          string
	Parliament  int
	Session     int
	Number      int
	Date        string
	BillID      string
	Description string
	Yeas        int
	Nays        int
	Paired      int
	Result      string
	Chamber     string
	SittingURL  string
	LastScraped string
}

// BillStageRecord is the write-side representation of a bill stage, used by scrapers.
type BillStageRecord struct {
	BillID  string
	Stage   string
	Chamber string
	Date    string
	Notes   string
}

// ── upsert helpers ────────────────────────────────────────────────────────────

// UpsertMember inserts or updates a member record.
func UpsertMember(db *sql.DB, m MemberRecord) error {
	active := 0
	if m.Active {
		active = 1
	}
	chamber := m.Chamber
	if chamber == "" {
		chamber = "commons"
	}
	govLevel := m.GovernmentLevel
	if govLevel == "" {
		govLevel = "federal"
	}
	_, err := db.Exec(`
		INSERT INTO members
			(id, name, party, riding, province, role, photo_url, email, website,
			 chamber, active, last_scraped, government_level)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name             = excluded.name,
			party            = excluded.party,
			riding           = excluded.riding,
			province         = excluded.province,
			role             = excluded.role,
			photo_url        = excluded.photo_url,
			email            = excluded.email,
			website          = excluded.website,
			chamber          = excluded.chamber,
			active           = excluded.active,
			last_scraped     = excluded.last_scraped,
			government_level = excluded.government_level`,
		m.ID, m.Name, m.Party, m.Riding, m.Province, m.Role, m.PhotoURL,
		m.Email, m.Website, chamber, active, m.LastScraped, govLevel,
	)
	return err
}

// UpsertProfiles persists a slice of members, logging individual row failures
// and continuing to process the remaining records.
func UpsertProfiles(db *sql.DB, members []MemberRecord, delay time.Duration) {
	for _, m := range members {
		if err := UpsertMember(db, m); err != nil {
			log.Printf("[members] upsert %s: %v", m.ID, err)
		}
		time.Sleep(delay)
	}
}

// UpsertBill inserts or updates a bill record.
// Existing AI summaries are preserved when the incoming value is empty.
func UpsertBill(db *sql.DB, b BillRecord) error {
	_, err := db.Exec(`
		INSERT INTO bills
			(id, parliament, session, number, title, short_title, bill_type,
			 chamber, sponsor_id, current_stage, current_status, category,
			 summary_ai, full_text_url, legisinfo_url,
			 introduced_date, last_activity_date, last_scraped)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			parliament         = excluded.parliament,
			session            = excluded.session,
			number             = excluded.number,
			title              = excluded.title,
			short_title        = excluded.short_title,
			bill_type          = excluded.bill_type,
			chamber            = COALESCE(NULLIF(excluded.chamber,''), bills.chamber),
			sponsor_id         = excluded.sponsor_id,
			current_stage      = excluded.current_stage,
			current_status     = excluded.current_status,
			category           = COALESCE(NULLIF(excluded.category,''), bills.category),
			summary_ai         = COALESCE(NULLIF(excluded.summary_ai,''), bills.summary_ai),
			full_text_url      = excluded.full_text_url,
			legisinfo_url      = excluded.legisinfo_url,
			introduced_date    = excluded.introduced_date,
			last_activity_date = excluded.last_activity_date,
			last_scraped       = excluded.last_scraped`,
		b.ID, b.Parliament, b.Session, b.Number, b.Title, b.ShortTitle,
		b.BillType, b.Chamber, nullStr(b.SponsorID), b.CurrentStage, b.CurrentStatus,
		b.Category, b.SummaryAI, b.FullTextURL, b.LegisInfoURL,
		b.IntroducedDate, b.LastActivityDate, b.LastScraped,
	)
	return err
}

// UpsertDivision inserts or updates a division record.
func UpsertDivision(db *sql.DB, d DivisionRecord) error {
	chamber := d.Chamber
	if chamber == "" {
		chamber = "commons"
	}
	_, err := db.Exec(`
		INSERT INTO divisions
			(id, parliament, session, number, date, bill_id, description,
			 yeas, nays, paired, result, chamber, sitting_url, last_scraped)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			parliament   = excluded.parliament,
			session      = excluded.session,
			number       = excluded.number,
			date         = excluded.date,
			bill_id      = excluded.bill_id,
			description  = excluded.description,
			yeas         = excluded.yeas,
			nays         = excluded.nays,
			paired       = excluded.paired,
			result       = excluded.result,
			chamber      = excluded.chamber,
			sitting_url  = excluded.sitting_url,
			last_scraped = excluded.last_scraped`,
		d.ID, d.Parliament, d.Session, d.Number, d.Date, nullStr(d.BillID), d.Description,
		d.Yeas, d.Nays, d.Paired, d.Result, chamber, d.SittingURL, d.LastScraped,
	)
	return err
}

// UpsertMemberVote inserts or updates a single member vote on a division.
func UpsertMemberVote(db *sql.DB, divisionID, memberID, vote string) error {
	_, err := db.Exec(`
		INSERT INTO member_votes (division_id, member_id, vote)
		VALUES (?,?,?)
		ON CONFLICT(division_id, member_id) DO UPDATE SET vote = excluded.vote`,
		divisionID, memberID, vote,
	)
	return err
}

// UpsertBillStage inserts a bill-stage record (idempotent by bill_id + stage + date).
func UpsertBillStage(db *sql.DB, s BillStageRecord) error {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO bill_stages (bill_id, stage, chamber, date, notes)
		VALUES (?,?,?,?,?)`,
		s.BillID, s.Stage, s.Chamber, s.Date, s.Notes,
	)
	return err
}

// UpsertSittingDate inserts a sitting calendar date (idempotent).
func UpsertSittingDate(db *sql.DB, parliament, session int, date string) error {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO sitting_calendar (parliament, session, date)
		VALUES (?,?,?)`,
		parliament, session, date,
	)
	return err
}

// ── division helpers ──────────────────────────────────────────────────────────

// DivisionExists returns true if a division with the given ID already exists.
func DivisionExists(db *sql.DB, divisionID string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(1) FROM divisions WHERE id = ?`, divisionID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DivisionHasVotes returns true if at least one member_votes row exists for
// the given division. Used by crawlers to decide whether to re-fetch detail pages.
func DivisionHasVotes(db *sql.DB, divisionID string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(1) FROM member_votes WHERE division_id = ?`, divisionID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// SittingDates returns all sitting dates for the given parliament/session.
func SittingDates(db *sql.DB, parliament, session int) ([]string, error) {
	rows, err := db.Query(
		`SELECT date FROM sitting_calendar WHERE parliament = ? AND session = ? ORDER BY date`,
		parliament, session,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

// ReplaceLegislatureCalendarDates atomically replaces all dates for a jurisdiction.
func ReplaceLegislatureCalendarDates(db *sql.DB, jurisdiction string, dates []string, lastScraped string) error {
	if strings.TrimSpace(jurisdiction) == "" {
		return fmt.Errorf("jurisdiction required")
	}
	if strings.TrimSpace(lastScraped) == "" {
		lastScraped = time.Now().UTC().Format("2006-01-02T15:04:05")
	}
	uniq := make(map[string]struct{}, len(dates))
	for _, d := range dates {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		uniq[d] = struct{}{}
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.Exec(`DELETE FROM legislature_calendar_dates WHERE jurisdiction = ?`, jurisdiction); err != nil {
		return err
	}
	if len(uniq) > 0 {
		stmt, prepErr := tx.Prepare(`
			INSERT INTO legislature_calendar_dates (jurisdiction, date, last_scraped)
			VALUES (?,?,?)`)
		if prepErr != nil {
			return prepErr
		}
		defer stmt.Close()
		for d := range uniq {
			if _, err = stmt.Exec(jurisdiction, d, lastScraped); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// LegislatureCalendarDates returns stored dates for a jurisdiction.
func LegislatureCalendarDates(db *sql.DB, jurisdiction string) ([]string, error) {
	rows, err := db.Query(
		`SELECT date FROM legislature_calendar_dates WHERE jurisdiction = ? ORDER BY date`,
		jurisdiction,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

// nullStr converts an empty string to nil so FK columns without a value don't
// trigger foreign-key constraint violations.
func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
