// Votes scraper: ourcommons.ca votes index, division detail, sitting calendar.
package scraper

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
)

var chamberMeetingDateClassRe = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)

// seeListUnderRe matches journal text like "(SEE LIST UNDER DIVISION NO. 15)".
var seeListUnderRe = regexp.MustCompile(`(?i)see[\s\x{00a0}]+list[\s\x{00a0}]+under[\s\x{00a0}]+division[\s\x{00a0}]+no\.?[\s\x{00a0}]*(\d+)`)

// ── constants ─────────────────────────────────────────────────────────────────

const (
	VotesIndexURL      = "https://www.ourcommons.ca/Members/en/votes"
	SittingCalendarURL = "https://www.ourcommons.ca/en/sitting-calendar"

	// CurrentParliament and CurrentSession: update when a new parliament opens.
	CurrentParliament = 45
	CurrentSession    = 1
)

// ── types ─────────────────────────────────────────────────────────────────────

// DivisionStub holds a row from the votes index page.
type DivisionStub struct {
	ID          string
	Parliament  int
	Session     int
	Number      int
	Date        string
	BillNumber  string
	Description string
	Yeas        int
	Nays        int
	Paired      int
	Result      string
	Chamber     string
	DetailURL   string
	LastScraped string
}

// MemberVote records how a single MP voted in a division.
type MemberVote struct {
	DivisionID string
	MemberID   string
	MemberName string
	Vote       string // "Yea" | "Nay" | "Paired" | "Abstain"
}

// ── Votes index ───────────────────────────────────────────────────────────────

// CrawlVotesIndex scrapes the ourcommons.ca recorded-votes index table.
func CrawlVotesIndex(
	url string,
	parliament, session int,
	client *http.Client,
) ([]DivisionStub, error) {
	if url == "" {
		url = VotesIndexURL
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Infof("[votes] fetching index: %s", url)

	doc, err := fetchDoc(url, client)
	if err != nil {
		return nil, fmt.Errorf("votes index: %w", err)
	}

	table := doc.Find("table.table, table#votes-table, table").First()
	if table.Length() == 0 {
		return nil, fmt.Errorf("votes index: no table found on %s", url)
	}

	nonDigitRe := regexp.MustCompile(`\D`)

	var divs []DivisionStub
	table.Find("tbody tr").Each(func(_ int, row *goquery.Selection) {
		cols := row.Find("td")
		// Actual ourcommons.ca column order (6 columns):
		// 0: vote number  1: bill type (optional)  2: description
		// 3: "Yeas / Nays / Paired"  4: result (with icon)  5: date
		if cols.Length() < 5 {
			return
		}

		numText := strings.TrimSpace(nonDigitRe.ReplaceAllString(cols.Eq(0).Text(), ""))
		if numText == "" {
			return
		}
		num, _ := strconv.Atoi(numText)

		description := strings.TrimSpace(cols.Eq(2).Text())

		// Col 3 contains "Yeas / Nays / Paired" — split on "/"
		yeas, nays, paired := 0, 0, 0
		voteParts := strings.Split(cols.Eq(3).Text(), "/")
		if len(voteParts) >= 1 {
			yeas, _ = strconv.Atoi(strings.TrimSpace(nonDigitRe.ReplaceAllString(voteParts[0], "")))
		}
		if len(voteParts) >= 2 {
			nays, _ = strconv.Atoi(strings.TrimSpace(nonDigitRe.ReplaceAllString(voteParts[1], "")))
		}
		if len(voteParts) >= 3 {
			paired, _ = strconv.Atoi(strings.TrimSpace(nonDigitRe.ReplaceAllString(voteParts[2], "")))
		}

		result := strings.TrimSpace(cols.Eq(4).Text())

		// Col 5: date formatted as "Wednesday, March 25, 2026"
		date := ""
		if cols.Length() > 5 {
			date = utils.FindDateInText(strings.TrimSpace(cols.Eq(5).Text()))
		}

		// Extract bill number: first check col 1 (which may contain just a bill
		// number like "C-47" on some sites), then fall back to the description.
		billNumber := utils.ExtractBillNumber(strings.TrimSpace(cols.Eq(1).Text()))
		if billNumber == "" {
			billNumber = utils.ExtractBillNumber(description)
		}

		// Detail link
		var detailURL string
		row.Find("a[href*='votes']").Each(func(_ int, a *goquery.Selection) {
			if detailURL == "" {
				if href, ok := a.Attr("href"); ok {
					if strings.HasPrefix(href, "http") {
						detailURL = href
					} else {
						detailURL = "https://www.ourcommons.ca" + href
					}
				}
			}
		})

		divs = append(divs, DivisionStub{
			ID:          utils.DivisionID(parliament, session, num),
			Parliament:  parliament,
			Session:     session,
			Number:      num,
			Date:        date,
			BillNumber:  billNumber,
			Description: description,
			Yeas:        yeas,
			Nays:        nays,
			Paired:      paired,
			Result:      result,
			Chamber:     "commons",
			DetailURL:   detailURL,
			LastScraped: utils.NowISO(),
		})
	})

	clog.Infof("[votes] found %d divisions", len(divs))
	return divs, nil
}

// ── Division detail ───────────────────────────────────────────────────────────

// voteSelectors maps canonical vote types to the CSS selectors used on the
// legacy ourcommons.ca division detail page layout (kept as fallback).
var voteSelectors = map[string][]string{
	"Yea": {
		".vote-yea .member-name a",
		"[class*='Yea'] .member-name a",
		"section.agreed-to li a",
		"ul.yea li a",
	},
	"Nay": {
		".vote-nay .member-name a",
		"[class*='Nay'] .member-name a",
		"section.negatived li a",
		"ul.nay li a",
	},
	"Paired": {
		".vote-paired .member-name a",
		"[class*='Paired'] .member-name a",
		"ul.paired li a",
	},
}

// crawlDivisionDetailFromCSV fetches the CSV export at {url}/csv and returns
// member votes. The CSV columns are:
//
//	0: Person ID  (numeric member ID)
//	1: Member of Parliament
//	2: Political Affiliation
//	3: Member Voted  ("Yea", "Nay", or "")
//	4: Paired       (non-empty when the member is paired)
//
// Returns nil,nil when the CSV response contains only a header row (e.g. an
// older unanimous vote where ourcommons.ca serves an empty body).
func crawlDivisionDetailFromCSV(divisionID, pageURL string, client *http.Client) ([]MemberVote, error) {
	csvURL := strings.TrimRight(pageURL, "/") + "/csv"
	resp, err := client.Get(csvURL)
	if err != nil {
		return nil, fmt.Errorf("csv GET %q: %w", csvURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("csv GET %q: status %d", csvURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("csv read %q: %w", csvURL, err)
	}

	r := csv.NewReader(bytes.NewReader(body))
	// Skip header row.
	if _, err := r.Read(); err != nil {
		return nil, fmt.Errorf("csv header %q: %w", csvURL, err)
	}

	var votes []MemberVote
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv parse %q: %w", csvURL, err)
		}
		if len(record) < 4 {
			continue
		}
		personID := strings.TrimSpace(record[0])
		if personID == "" {
			continue
		}
		memberVoted := strings.TrimSpace(record[3])
		paired := ""
		if len(record) >= 5 {
			paired = strings.TrimSpace(record[4])
		}

		var vote string
		switch strings.ToLower(memberVoted) {
		case "yea":
			vote = "Yea"
		case "nay":
			vote = "Nay"
		default:
			if paired != "" {
				vote = "Paired"
			} else {
				continue // not present / abstain
			}
		}
		votes = append(votes, MemberVote{
			DivisionID: divisionID,
			MemberID:   personID,
			Vote:       vote,
		})
	}
	return votes, nil
}

// normaliseVoteText maps the free-form vote text from the current
// ourcommons.ca table layout to a canonical vote value.
var normaliseVoteText = map[string]string{
	"yea":    "Yea",
	"nay":    "Nay",
	"paired": "Paired",
}

// CrawlDivisionDetail scrapes how each MP voted on a single division.
func CrawlDivisionDetail(divisionID, url string, client *http.Client) ([]MemberVote, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Debugf("[votes] scraping division detail: %s", url)

	// ── CSV export (primary) ──────────────────────────────────────────────────
	// ourcommons.ca serves a machine-readable CSV at {url}/csv. The table on
	// the HTML page is populated via JavaScript (DataTables AJAX), so the tbody
	// is empty when fetched with a plain HTTP client.
	csvVotes, csvErr := crawlDivisionDetailFromCSV(divisionID, url, client)
	if csvErr == nil && len(csvVotes) > 0 {
		clog.Debugf("[votes] division %s: %d member votes (csv)", divisionID, len(csvVotes))
		return csvVotes, nil
	}
	if csvErr != nil {
		clog.Debugf("[votes] csv fetch error for %s: %v; falling back to HTML", divisionID, csvErr)
	}

	doc, err := fetchDoc(url, client)
	if err != nil {
		return nil, fmt.Errorf("division detail %q: %w", url, err)
	}

	var votes []MemberVote

	// ── Current layout (45th Parliament onwards) ─────────────────────────────
	// The page renders a single table with class "ce-mip-table-mobile".
	// Each tbody row has four columns:
	//   col 1: member link  (<a href="/members/en/{id}">Name</a>)
	//   col 2: party
	//   col 3: vote value   (Yea / Nay / — / empty)
	//   col 4: paired flag
	doc.Find("table.ce-mip-table-mobile tbody tr").Each(func(_ int, row *goquery.Selection) {
		cols := row.Find("td")
		if cols.Length() < 3 {
			return
		}
		href, _ := cols.Eq(0).Find("a").Attr("href")
		memberID := utils.ExtractMemberID(href)
		if memberID == "" {
			return
		}
		rawVote := strings.ToLower(strings.TrimSpace(cols.Eq(2).Text()))
		canonical, ok := normaliseVoteText[rawVote]
		if !ok {
			return // abstained / empty — skip
		}
		votes = append(votes, MemberVote{
			DivisionID: divisionID,
			MemberID:   memberID,
			Vote:       canonical,
		})
	})

	// ── Legacy / fallback layout ──────────────────────────────────────────────
	// If the table selector matched nothing, try the old selector-map approach
	// so that previously cached test fixtures and older page snapshots still work.
	if len(votes) == 0 {
		for voteType, selectors := range voteSelectors {
			for _, sel := range selectors {
				members := doc.Find(sel)
				if members.Length() == 0 {
					continue
				}
				members.Each(func(_ int, a *goquery.Selection) {
					href, _ := a.Attr("href")
					memberID := utils.ExtractMemberID(href)
					if memberID != "" {
						votes = append(votes, MemberVote{
							DivisionID: divisionID,
							MemberID:   memberID,
							Vote:       voteType,
						})
					}
				})
				break // found a working selector; skip the rest
			}
		}
	}

	// ── Journals fallback ──────────────────────────────────────────────────────
	// Some divisions render an empty member table and point to a Journals entry
	// in Motion Text (DocumentViewer link). Parse that section for vote names.
	if len(votes) == 0 {
		if journalURL := findDivisionJournalURL(doc, url); journalURL != "" {
			membersSearchURL := membersSearchURLForDivision(divisionID)
			journalVotes, err := crawlDivisionVotesFromJournal(divisionID, journalURL, membersSearchURL, client)
			if err != nil {
				clog.Debugf("[votes] journals fallback error for %s: %v", divisionID, err)
			} else {
				votes = journalVotes
			}
		}
	}

	clog.Debugf("[votes] division %s: %d member votes", divisionID, len(votes))
	return votes, nil
}

func findDivisionJournalURL(doc *goquery.Document, pageURL string) string {
	journalURL := ""
	anchoredJournalURL := ""
	doc.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, ok := a.Attr("href")
		if !ok {
			return true
		}
		href = strings.TrimSpace(href)
		if href == "" {
			return true
		}

		resolved := resolveRelativeURL(pageURL, href)
		resolvedLower := strings.ToLower(resolved)
		if !strings.Contains(resolvedLower, "/documentviewer/") {
			return true
		}

		text := strings.ToLower(strings.TrimSpace(a.Text()))
		isJournalText := strings.Contains(text, "journals") || strings.Contains(text, "journal")
		hasDocAnchor := strings.Contains(strings.ToUpper(resolved), "#DOC--")

		// Ignore generic documentviewer links from page chrome (e.g. "House
		// Publications") and only keep likely journals targets.
		if !isJournalText && !hasDocAnchor {
			return true
		}

		if hasDocAnchor {
			anchoredJournalURL = resolved
			return false
		}
		if journalURL == "" {
			journalURL = resolved
		}
		return true
	})
	if anchoredJournalURL != "" {
		return anchoredJournalURL
	}
	return journalURL
}

// ── Surname index for journals fallback ──────────────────────────────────────

// surnameEntry is one member record in the surname lookup index.
type surnameEntry struct {
	MemberID string
	Riding   string // decoded riding name, used for disambiguation
}

// surnameIndex maps a normalised surname string to one or more members.
// Collisions (e.g. two MPs named "Belanger") are resolved by riding.
type surnameIndex map[string][]surnameEntry

// accentStripper removes combining diacritical marks so that "Bélanger" and
// "Belanger" compare equal after normalisation.
var accentStripper = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// normSurname folds a surname to a canonical comparison key:
//   - Unicode NFC/NFD decomposition to strip diacritics
//   - lowercase
//   - apostrophes removed (d'Entremont → dentremont)
//   - hyphens replaced with spaces (Ste-Marie → ste marie)
//   - leading/trailing whitespace trimmed
func normSurname(s string) string {
	s, _, _ = transform.String(accentStripper, s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "'", "")      // ASCII apostrophe
	s = strings.ReplaceAll(s, "\u2019", "") // Unicode right single quotation mark
	s = strings.ReplaceAll(s, "-", " ")
	return strings.TrimSpace(s)
}

// buildSurnameIndex fetches the ourcommons.ca member search page for the given
// URL (e.g. https://www.ourcommons.ca/Members/en/search?parliament=45&session=1)
// and builds a surname→[{memberID, riding}] lookup map.
//
// The last name is derived from the member's href slug by discarding everything
// up to and including the first hyphen (the first name), e.g.:
//
//	/Members/en/ziad-aboultaif(89156)   → surname slug "aboultaif"
//	/Members/en/fares-al-soud(123033)   → surname slug "al-soud" → "al soud"
//	/Members/en/michelle-rempel-garner  → surname slug "rempel-garner" → "rempel garner"
//	/Members/en/chris-dentremont        → surname slug "dentremont"
//
// Returns an empty (non-nil) index on fetch failure so callers can continue
// without member IDs.
func buildSurnameIndex(membersSearchURL string, client *http.Client) surnameIndex {
	idx := surnameIndex{}
	if membersSearchURL == "" {
		return idx
	}
	doc, err := fetchDoc(membersSearchURL, client)
	if err != nil {
		clog.Infof("[votes] surname index: fetch error %v", err)
		return idx
	}

	memberIDRe := regexp.MustCompile(`\((\d+)\)$`)

	doc.Find("[id^='mp-tile-person-id-']").Each(func(_ int, tile *goquery.Selection) {
		link := tile.Find("a.ce-mip-mp-tile-link").First()
		href, _ := link.Attr("href")
		if href == "" {
			return
		}
		// Extract numeric member ID from href like /Members/en/ziad-aboultaif(89156)
		m := memberIDRe.FindStringSubmatch(href)
		if m == nil {
			return
		}
		memberID := m[1]

		// Extract slug: strip the /Members/en/ prefix and the (ID) suffix
		slug := href
		if i := strings.LastIndex(slug, "/"); i >= 0 {
			slug = slug[i+1:]
		}
		slug = memberIDRe.ReplaceAllString(slug, "")

		// Last name = everything after the first hyphen
		dashIdx := strings.Index(slug, "-")
		if dashIdx < 0 || dashIdx == len(slug)-1 {
			return
		}
		surnamePart := slug[dashIdx+1:]

		riding := strings.TrimSpace(tile.Find(".ce-mip-mp-constituency").First().Text())

		key := normSurname(surnamePart)
		idx[key] = append(idx[key], surnameEntry{MemberID: memberID, Riding: riding})
	})

	clog.Infof("[votes] surname index: %d unique surname keys from %s", len(idx), membersSearchURL)
	return idx
}

// lookupMemberID resolves a journal DivisionItem text (e.g. "Belanger" or
// "Belanger (Desnethé—Missinippi—Churchill River)") to a member ID using the
// provided surnameIndex. Returns "" if the member cannot be resolved.
func lookupMemberID(idx surnameIndex, journalText string) string {
	if len(idx) == 0 {
		return ""
	}
	// Split riding disambiguation from the surname
	surname := journalText
	ridingHint := ""
	if open := strings.Index(journalText, "("); open >= 0 {
		surname = strings.TrimSpace(journalText[:open])
		if close := strings.LastIndex(journalText, ")"); close > open {
			ridingHint = strings.TrimSpace(journalText[open+1 : close])
		}
	}

	key := normSurname(surname)
	candidates := idx[key]
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0].MemberID
	}
	// Multiple candidates: use riding hint for disambiguation.
	if ridingHint != "" {
		normHint := normSurname(ridingHint)
		for _, c := range candidates {
			if normSurname(c.Riding) == normHint {
				return c.MemberID
			}
		}
	}
	// Can't disambiguate; return empty.
	return ""
}

// membersSearchURLForDivision returns the ourcommons.ca member search URL
// scoped to the parliament and session encoded in the divisionID ("45-1-100").
func membersSearchURLForDivision(divisionID string) string {
	parts := strings.SplitN(divisionID, "-", 3)
	if len(parts) < 2 {
		return MembersListURL
	}
	return fmt.Sprintf("%s?parliament=%s&session=%s", MembersListURL, parts[0], parts[1])
}

// sectionTextAfterAnchor returns the concatenated text of all DOM siblings that
// follow the anchor element until the next named anchor is encountered. This
// gives us the text content of a specific journal division section without
// reading into the next section.
func sectionTextAfterAnchor(target *goquery.Selection) string {
	if target == nil || target.Length() == 0 {
		return ""
	}
	var sb strings.Builder
	for s := target.Next(); s.Length() > 0; s = s.Next() {
		if _, hasName := s.Attr("name"); hasName {
			break
		}
		sb.WriteString(s.Text())
	}
	return sb.String()
}

// findTableForDivisionNumber locates the DivisionType vote table for divNum on
// a journal page. It walks backwards through the siblings preceding each
// DivisionType table to find text containing "Division No. X", stopping at the
// previous DivisionType table boundary to avoid false matches.
func findTableForDivisionNumber(doc *goquery.Document, divNum string) *goquery.Selection {
	divNumRe := regexp.MustCompile(`(?i)division[\s\x{00a0}]+no\.?[\s\x{00a0}]*` + regexp.QuoteMeta(divNum) + `[^\d]`)
	var result *goquery.Selection

	doc.Find("table").EachWithBreak(func(_ int, t *goquery.Selection) bool {
		if t.Find("td.DivisionType").Length() == 0 {
			return true
		}
		// Check if the division number cell is inside this table (real
		// ourcommons.ca format: td.DivisionNumber is the first row of the table).
		if t.Find("td.DivisionNumber").FilterFunction(func(_ int, s *goquery.Selection) bool {
			return divNumRe.MatchString(s.Text())
		}).Length() > 0 {
			result = t
			return false
		}
		sibling := t.Prev()
		for sibling.Length() > 0 {
			if sibling.Is("table") && sibling.Find("td.DivisionType").Length() > 0 {
				break // don't look past the previous vote table
			}
			if divNumRe.MatchString(sibling.Text()) {
				result = t
				return false
			}
			sibling = sibling.Prev()
		}
		return true
	})
	return result
}

func crawlDivisionVotesFromJournal(divisionID, journalURL, membersSearchURL string, client *http.Client) ([]MemberVote, error) {
	// Build surname→memberID index from the member search page for this
	// parliament/session so we can resolve journal surnames to member IDs.
	idx := buildSurnameIndex(membersSearchURL, client)

	journalDoc, err := fetchDoc(journalURL, client)
	if err != nil {
		return nil, fmt.Errorf("journals page %q: %w", journalURL, err)
	}

	var table *goquery.Selection
	var target *goquery.Selection
	if parsed, err := neturl.Parse(journalURL); err == nil {
		anchor := strings.TrimSpace(parsed.Fragment)
		if anchor != "" {
			if t := journalDoc.Find(fmt.Sprintf("a[name='%s']", anchor)).First(); t.Length() > 0 {
				target = t
				table = t.NextAllFiltered("table").First()
			}
		}
	}

	// If the section has no DivisionType vote data, check whether it contains
	// a "(SEE LIST UNDER DIVISION NO. X)" reference and redirect to that table.
	if table == nil || table.Find("td.DivisionType").Length() == 0 {
		sectionText := sectionTextAfterAnchor(target)
		if sectionText == "" {
			sectionText = journalDoc.Text()
		}
		if m := seeListUnderRe.FindStringSubmatch(sectionText); m != nil {
			refDivNum := m[1]
			clog.Debugf("[votes] division %s: journal says SEE LIST UNDER DIVISION NO. %s", divisionID, refDivNum)
			if refTable := findTableForDivisionNumber(journalDoc, refDivNum); refTable != nil {
				table = refTable
			}
		}
	}

	if table == nil || table.Length() == 0 {
		table = journalDoc.Find("table").FilterFunction(func(_ int, s *goquery.Selection) bool {
			text := strings.ToUpper(s.Text())
			return strings.Contains(text, "YEAS") && strings.Contains(text, "NAYS")
		}).First()
	}
	if table == nil || table.Length() == 0 {
		return nil, nil
	}

	votes := make([]MemberVote, 0)
	table.Find("td.DivisionType").Each(func(_ int, td *goquery.Selection) {
		header := strings.ToUpper(strings.TrimSpace(td.Find("p.DivisionType").First().Text()))
		voteType := ""
		switch {
		case strings.Contains(header, "YEAS") || strings.Contains(header, "AYES"):
			voteType = "Yea"
		case strings.Contains(header, "NAYS"):
			voteType = "Nay"
		case strings.Contains(header, "PAIRED"):
			voteType = "Paired"
		default:
			return
		}

		td.Find("span.DivisionItem").Each(func(_ int, span *goquery.Selection) {
			// The span text contains only a surname (possibly with riding
			// disambiguation in parentheses) and a trailing <br>. Strip the <br>
			// text node artefact.
			raw := strings.TrimSpace(span.Text())
			if raw == "" {
				return
			}
			memberID := lookupMemberID(idx, raw)
			votes = append(votes, MemberVote{
				DivisionID: divisionID,
				MemberID:   memberID,
				MemberName: raw,
				Vote:       voteType,
			})
		})
	})

	return votes, nil
}

// ── Sitting calendar ──────────────────────────────────────────────────────────

// CrawlSittingCalendar scrapes the ourcommons.ca sitting calendar.
// Returns a sorted, deduplicated list of ISO-8601 sitting dates.
func CrawlSittingCalendar(url string, client *http.Client) ([]string, error) {
	if url == "" {
		url = SittingCalendarURL
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Infof("[votes] fetching sitting calendar: %s", url)

	doc, err := fetchDoc(url, client)
	if err != nil {
		return nil, fmt.Errorf("sitting calendar: %w", err)
	}

	seen := make(map[string]bool)

	// Current ourcommons.ca markup uses class tokens like:
	// class="2026-04-23 chamber-meeting"
	doc.Find("td.chamber-meeting, td[class*='chamber-meeting']").Each(func(_ int, s *goquery.Selection) {
		classAttr, _ := s.Attr("class")
		for _, token := range chamberMeetingDateClassRe.FindAllString(classAttr, -1) {
			if d := utils.ParseDate(token); d != "" {
				seen[d] = true
			}
		}
	})

	doc.Find("[data-date], td.sitting, td[class*='sitting'], [class*='sitting-day']").Each(
		func(_ int, s *goquery.Selection) {
			raw, _ := s.Attr("data-date")
			if raw == "" {
				raw, _ = s.Attr("datetime")
			}
			if raw == "" {
				raw = strings.TrimSpace(s.Text())
			}
			if d := utils.ParseDate(raw); d != "" {
				seen[d] = true
			}
		})

	dates := make([]string, 0, len(seen))
	for d := range seen {
		dates = append(dates, d)
	}

	// Sort (dates are ISO-8601 so lexicographic = chronological)
	for i := 1; i < len(dates); i++ {
		for j := i; j > 0 && dates[j] < dates[j-1]; j-- {
			dates[j], dates[j-1] = dates[j-1], dates[j]
		}
	}

	clog.Infof("[votes] found %d sitting dates", len(dates))
	return dates, nil
}

// ParliamentIsSitting returns true if today (or the provided date) falls in
// the list of known sitting dates.
func ParliamentIsSitting(sittingDates []string, today string) bool {
	if today == "" {
		today = utils.TodayISO()
	}
	for _, d := range sittingDates {
		if d == today {
			return true
		}
	}
	return false
}
