package provincial

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
)

// ── Nova Scotia regexps ───────────────────────────────────────────────────────

var novaScotiaBillLinkRe = regexp.MustCompile(`(?i)(bills-statutes|bill|legislative-business)`)

// nsHansardSessionIndexRe matches hrefs to NS Hansard session index pages, e.g.
// /legislative-business/hansard-debates/assembly-66-session-1
var nsHansardSessionIndexRe = regexp.MustCompile(`(?i)/hansard-debates/assembly-(\d+)-session-(\d+)`)

// nsVotesPDFLinkRe matches NS journals and Hansard PDF links under the default
// files path.
var nsVotesPDFLinkRe = regexp.MustCompile(`(?i)/sites/default/files/pdfs/proceedings/(?:journals|hansard)/[^"'\s]+\.pdf(?:\?[^"'\s]*)?`)

// nsHansardDayPageRe matches hrefs to individual Hansard day pages.
// Handles the modern format (assembly 64+):
//
//	/assembly-65-session-1/house_26apr09
//
// and the assembly 61–63 transitional format:
//
//	/assembly-61-session-1/61_1_house_09nov04.htm
//	/61e-assemblee-1e-session/61_1_house_10mar25
var nsHansardDayPageRe = regexp.MustCompile(`(?i)/hansard-debates/[^/]+/(?:\d+_\d+_)?house_\w+`)

// nsHansardSlugRe extracts the date components from a Hansard day-page slug:
// "house_26apr09" -> year="26", month="apr", day="09"
var nsHansardSlugRe = regexp.MustCompile(`(?i)house_(\d{2})([a-z]{3})(\d{1,2})`)

// nsDoubleSpaceRe matches two or more consecutive spaces, used to split the
// two-column paragraph vote format used by assembly 61–63.
var nsDoubleSpaceRe = regexp.MustCompile(`  +`)

var nsMonthAbbr = map[string]string{
	"jan": "01", "feb": "02", "mar": "03", "apr": "04",
	"may": "05", "jun": "06", "jul": "07", "aug": "08",
	"sep": "09", "oct": "10", "nov": "11", "dec": "12",
}
var novaScotiaVotesLinkRe = regexp.MustCompile(`(?i)(journals?|proceedings|votes|hansard-debates)`)

// ── Nova Scotia bills ─────────────────────────────────────────────────────────

func crawlNovaScotiaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if indexURL == "" {
		indexURL = "https://nslegislature.ca/legislative-business/bills-statutes/bills"
	}
	return crawlProvincialBillsFromIndexWithMatcher(indexURL, "ns", legislature, session, "nova_scotia", client, novaScotiaBillLinkRe)
}

// CrawlNovaScotiaBills crawls Nova Scotia bills pages.
func CrawlNovaScotiaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlNovaScotiaBills(indexURL, legislature, session, client)
}

// ── Nova Scotia votes — HTML path ─────────────────────────────────────────────

// nsDateFromHansardURL parses the sitting date from a Hansard day-page URL.
// The URL slug "house_26apr09" encodes year=2026, month=April, day=09.
// Year is interpreted as 20xx; "26" → 2026.
func nsDateFromHansardURL(rawURL string) string {
	m := nsHansardSlugRe.FindStringSubmatch(rawURL)
	if len(m) < 4 {
		return ""
	}
	month := nsMonthAbbr[strings.ToLower(m[2])]
	if month == "" {
		return ""
	}
	year := "20" + m[1]
	day := fmt.Sprintf("%02s", m[3])
	return fmt.Sprintf("%s-%s-%s", year, month, day)
}

type nsSessionRef struct{ leg, sess int }

// discoverNovaScotiaAllSessions fetches the main NS Hansard index and returns
// all assembly/session pairs found in links, sorted oldest-first.
func discoverNovaScotiaAllSessions(client *http.Client) []nsSessionRef {
	const indexURL = "https://nslegislature.ca/legislative-business/hansard-debates"
	doc, err := fetchDoc(indexURL, client)
	if err != nil {
		clog.Infof("[ns-votes] all-sittings: could not load session index: %v", err)
		return nil
	}
	seen := map[nsSessionRef]bool{}
	var sessions []nsSessionRef
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := normalizeHref(a.AttrOr("href", ""))
		m := nsHansardSessionIndexRe.FindStringSubmatch(href)
		if len(m) != 3 {
			return
		}
		leg, _ := strconv.Atoi(m[1])
		sess, _ := strconv.Atoi(m[2])
		if leg == 0 || sess == 0 {
			return
		}
		s := nsSessionRef{leg, sess}
		if !seen[s] {
			seen[s] = true
			sessions = append(sessions, s)
		}
	})
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].leg != sessions[j].leg {
			return sessions[i].leg < sessions[j].leg
		}
		return sessions[i].sess < sessions[j].sess
	})
	return sessions
}

// crawlNovaScotiaVotesFromHTML fetches the NS Hansard session index page,
// discovers individual day-page links, and parses each for vote tables.
// It returns the results and the number of day pages found (0 means the
// session page has no individual Hansard pages, likely an older session).
func crawlNovaScotiaVotesFromHTML(sessionURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, int, error) {
	clog.Debugf("[ns-votes] fetching hansard session index for html: %s", sessionURL)
	indexDoc, err := fetchDoc(sessionURL, client)
	if err != nil {
		return nil, 0, fmt.Errorf("ns html session index: %w", err)
	}

	seen := map[string]bool{}
	var dayURLs []string
	indexDoc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		if !nsHansardDayPageRe.MatchString(href) {
			return
		}
		full := resolveRelativeURL(sessionURL, href)
		if !seen[full] {
			seen[full] = true
			dayURLs = append(dayURLs, full)
		}
	})
	sort.Strings(dayURLs)

	if len(dayURLs) == 0 {
		return nil, 0, nil
	}

	var results []ProvincialDivisionResult
	divNum := 1
	for _, pageURL := range dayURLs {
		dayDoc, derr := fetchDoc(pageURL, client)
		if derr != nil {
			clog.Infof("[ns-votes] skip day page %s: %v", pageURL, derr)
			continue
		}
		divs := parseNSHansardHTMLPage(dayDoc, pageURL, legislature, session, divNum)
		results = append(results, divs...)
		divNum += len(divs)
		if len(divs) == 0 {
			// Advance the counter even when a day page contains no vote tables so
			// that division numbers remain consistent if later pages add votes.
			divNum++
		}
	}
	clog.Debugf("[ns-votes] parsed %d divisions from %d html day pages", len(results), len(dayURLs))
	return results, len(dayURLs), nil
}

// parseNSHansardHTMLPage finds all YEAS/NAYS vote divisions on a single
// Hansard day page and returns one ProvincialDivisionResult per division.
// It tries the modern table format (assembly 64+) first, then falls back to
// the paragraph format used by assemblies 61–63.
func parseNSHansardHTMLPage(doc *goquery.Document, pageURL string, legislature, session, startDivNum int) []ProvincialDivisionResult {
	if results := parseNSHansardTableVotes(doc, pageURL, legislature, session, startDivNum); len(results) > 0 {
		return results
	}
	return parseNSHansardParagraphVotes(doc, pageURL, legislature, session, startDivNum)
}

// parseNSHansardTableVotes handles the modern <table class="vote"> format
// introduced in assembly 64 (2023+).
func parseNSHansardTableVotes(doc *goquery.Document, pageURL string, legislature, session, startDivNum int) []ProvincialDivisionResult {
	date := nsDateFromHansardURL(pageURL)
	if date == "" {
		date = utils.TodayISO()
	}

	var results []ProvincialDivisionResult
	divNum := startDivNum

	doc.Find("table").Each(func(_ int, table *goquery.Selection) {
		// Require exactly two <th> headers reading YEAS and NAYS.
		headers := table.Find("thead tr th, tr th")
		if headers.Length() < 2 {
			return
		}
		if strings.ToUpper(strings.TrimSpace(headers.Eq(0).Text())) != "YEAS" ||
			strings.ToUpper(strings.TrimSpace(headers.Eq(1).Text())) != "NAYS" {
			return
		}

		divID := ProvincialDivisionID("ns", legislature, session, divNum, date)
		var votes []ProvincialMemberVote
		yeas, nays := 0, 0

		table.Find("tbody tr").Each(func(_ int, row *goquery.Selection) {
			cells := row.Find("td")
			if cells.Length() < 2 {
				return
			}
			yeaName := strings.TrimSpace(cells.Eq(0).Text())
			nayName := strings.TrimSpace(cells.Eq(1).Text())
			if yeaName != "" {
				votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: yeaName, Vote: "Yea"})
				yeas++
			}
			if nayName != "" {
				votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: nayName, Vote: "Nay"})
				nays++
			}
		})

		if yeas == 0 && nays == 0 {
			return
		}

		desc := nsDescriptionForVoteTable(table)
		if desc == "" {
			desc = "Recorded division"
		}

		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID: divID, Parliament: legislature, Session: session,
				Number: divNum, Date: date, Description: desc,
				Yeas: yeas, Nays: nays, Result: result,
				Chamber: "nova_scotia", DetailURL: pageURL,
				LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
		divNum++
	})
	return results
}

// parseNSHansardParagraphVotes handles the paragraph-based vote format used by
// assemblies 61–63 (2009–2021), where each vote section is bracketed by a bold
// "YEAS  NAYS" header paragraph and a "THE CLERK: For, N, Against, M." line.
// Each intervening paragraph encodes one row in a two-column layout, with the
// yea and nay names separated by two or more spaces.
func parseNSHansardParagraphVotes(doc *goquery.Document, pageURL string, legislature, session, startDivNum int) []ProvincialDivisionResult {
	date := nsDateFromHansardURL(pageURL)
	if date == "" {
		date = utils.TodayISO()
	}

	var results []ProvincialDivisionResult
	divNum := startDivNum

	doc.Find("b").Each(func(_ int, b *goquery.Selection) {
		upper := strings.ToUpper(strings.TrimSpace(b.Text()))
		if !strings.Contains(upper, "YEAS") || !strings.Contains(upper, "NAYS") {
			return
		}
		headerP := b.Parent()
		if !headerP.Is("p") {
			return
		}

		var yeaNames, nayNames []string
		for sib := headerP.Next(); sib.Length() > 0; sib = sib.Next() {
			if !sib.Is("p") {
				continue
			}
			text := sib.Text()
			upperText := strings.ToUpper(strings.TrimSpace(text))
			if strings.HasPrefix(upperText, "THE CLERK") {
				break
			}
			trimmed := strings.TrimSpace(text)
			if trimmed == "" || strings.HasPrefix(trimmed, "[") {
				continue
			}
			yea, nay := parseNSParagraphVoteRow(trimmed)
			if yea != "" {
				yeaNames = append(yeaNames, yea)
			}
			if nay != "" {
				nayNames = append(nayNames, nay)
			}
		}

		if len(yeaNames) == 0 && len(nayNames) == 0 {
			return
		}

		divID := ProvincialDivisionID("ns", legislature, session, divNum, date)
		var votes []ProvincialMemberVote
		for _, name := range yeaNames {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Yea"})
		}
		for _, name := range nayNames {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Nay"})
		}

		voteResult := "Carried"
		if len(nayNames) > len(yeaNames) {
			voteResult = "Negatived"
		}

		desc := nsDescriptionForVoteTable(headerP)
		if desc == "" {
			desc = "Recorded division"
		}

		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID: divID, Parliament: legislature, Session: session,
				Number: divNum, Date: date, Description: desc,
				Yeas: len(yeaNames), Nays: len(nayNames), Result: voteResult,
				Chamber: "nova_scotia", DetailURL: pageURL,
				LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
		divNum++
	})
	return results
}

// parseNSParagraphVoteRow splits one paragraph vote row into yea and nay names.
// The paragraph uses two or more consecutive spaces as a column separator; each
// name component within a column is separated by a single space.  "Hon." title
// tokens have a double space after them in the source, so they appear as a
// separate token in the split result.
//
// Examples:
//
//	"  Hon.  Alice Smith  Bob Jones"  → yea="Hon. Alice Smith", nay="Bob Jones"
//	"  Rafah  DiCostanzo  Tim Houston" → yea="Rafah DiCostanzo", nay="Tim Houston"
//	"  Bill  Horne  Karla  MacFarlane" → yea="Bill Horne",       nay="Karla MacFarlane"
//	"  Hon.  Iain Rankin"              → yea="Hon. Iain Rankin",  nay=""
func parseNSParagraphVoteRow(text string) (yea, nay string) {
	parts := nsDoubleSpaceRe.Split(text, -1)
	var tokens []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			tokens = append(tokens, p)
		}
	}
	switch len(tokens) {
	case 0:
		return "", ""
	case 1:
		return tokens[0], ""
	case 2:
		if strings.EqualFold(tokens[0], "Hon.") {
			return "Hon. " + tokens[1], ""
		}
		return tokens[0] + " " + tokens[1], ""
	case 3:
		if strings.EqualFold(tokens[0], "Hon.") {
			return "Hon. " + tokens[1], tokens[2]
		}
		return tokens[0] + " " + tokens[1], tokens[2]
	case 4:
		if strings.EqualFold(tokens[0], "Hon.") {
			return "Hon. " + tokens[1] + " " + tokens[2], tokens[3]
		}
		return tokens[0] + " " + tokens[1], tokens[2] + " " + tokens[3]
	default:
		mid := len(tokens) / 2
		return strings.Join(tokens[:mid], " "), strings.Join(tokens[mid:], " ")
	}
}

// nsDescriptionForVoteTable walks backwards through the siblings of the vote
// table to find the nearest <b> element, which contains the bill or motion name.
func nsDescriptionForVoteTable(table *goquery.Selection) string {
	for s := table.Prev(); s.Length() > 0; s = s.Prev() {
		if b := s.Find("b").First(); b.Length() > 0 {
			text := strings.TrimSpace(b.Text())
			if text != "" {
				return text
			}
		}
	}
	return ""
}

// ── Nova Scotia votes — PDF fallback ─────────────────────────────────────────

func novaScotiaHansardSessionURL(indexURL string, legislature, session int) string {
	trimmed := strings.TrimSpace(indexURL)
	if strings.Contains(trimmed, "/assembly-") && strings.Contains(trimmed, "/hansard-debates/") {
		return trimmed
	}
	if legislature <= 1 || session <= 0 {
		return trimmed
	}
	return fmt.Sprintf("https://nslegislature.ca/legislative-business/hansard-debates/assembly-%d-session-%d", legislature, session)
}

func includeNovaScotiaVotePDF(fullURL string, legislature, session int) bool {
	lower := strings.ToLower(fullURL)
	if strings.Contains(lower, "/proceedings/hansard/") {
		return true
	}
	if !strings.Contains(lower, "/proceedings/journals/") {
		return false
	}
	if legislature > 1 && session > 0 {
		wantDir := fmt.Sprintf("/%d-%d/", legislature, session)
		combinedDir := fmt.Sprintf("/%d-1and2/", legislature)
		if !strings.Contains(lower, wantDir) && !(session <= 2 && strings.Contains(lower, combinedDir)) {
			return false
		}
	}
	for _, token := range []string{
		"index", "appendix", "appendices", "cabinet", "cab%20list", "memberlist", "member%20list", "reports", "tabled", "bills",
	} {
		if strings.Contains(lower, token) {
			return false
		}
	}
	return true
}

func discoverNovaScotiaVotePDFLinks(doc *goquery.Document, baseURL string, legislature, session int) []string {
	var pdfLinks []string
	seen := make(map[string]bool)
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !nsVotesPDFLinkRe.MatchString(href) {
			return
		}
		full := resolveRelativeURL(baseURL, href)
		if seen[full] {
			return
		}
		if !includeNovaScotiaVotePDF(full, legislature, session) {
			return
		}
		seen[full] = true
		pdfLinks = append(pdfLinks, full)
	})
	sort.Strings(pdfLinks)
	return pdfLinks
}

// crawlNovaScotiaVotesFromPDF fetches the NS Hansard session page for the
// requested legislature/session and parses each discovered PDF. Older journal
// listings remain as a fallback when no Hansard PDFs are exposed.
func crawlNovaScotiaVotesFromPDF(indexURL string, legislature, session int, client *http.Client, allSittings bool) ([]ProvincialDivisionResult, error) {
	sessionURL := novaScotiaHansardSessionURL(indexURL, legislature, session)
	clog.Debugf("[ns-votes] fetching hansard session index for pdf: %s", sessionURL)
	indexDoc, err := fetchDoc(sessionURL, client)
	if err != nil {
		if indexURL != "" && indexURL != sessionURL {
			clog.Infof("[ns-votes] hansard session index unavailable, falling back to %s: %v", indexURL, err)
			indexDoc, err = fetchDoc(indexURL, client)
			if err != nil {
				return nil, fmt.Errorf("ns votes index: %w", err)
			}
			sessionURL = indexURL
		} else {
			return nil, fmt.Errorf("ns votes index: %w", err)
		}
	}

	pdfLinks := discoverNovaScotiaVotePDFLinks(indexDoc, sessionURL, legislature, session)
	if len(pdfLinks) == 0 {
		if indexURL != "" && indexURL != sessionURL {
			clog.Infof("[ns-votes] no hansard PDFs discovered at %s; falling back to %s", sessionURL, indexURL)
			fallbackDoc, ferr := fetchDoc(indexURL, client)
			if ferr != nil {
				return nil, fmt.Errorf("ns votes fallback index: %w", ferr)
			}
			pdfLinks = discoverNovaScotiaVotePDFLinks(fallbackDoc, indexURL, legislature, session)
			sessionURL = indexURL
		}
		if len(pdfLinks) == 0 {
			if allSittings {
				clog.Debugf("[ns-votes] no vote PDFs for assembly %d session %d", legislature, session)
			} else {
				clog.Infof("[ns-votes] no vote PDFs discovered for legislature=%d session=%d", legislature, session)
			}
			return nil, nil
		}
	}

	var results []ProvincialDivisionResult
	nextDivNum := 1
	for _, pdfURL := range pdfLinks {
		text, terr := downloadAndExtractPDFText(pdfURL, "ns", client)
		if terr != nil {
			clog.Infof("[ns-votes] skip pdf %s: %v", pdfURL, terr)
			continue
		}
		date := extractDateFromURL(pdfURL)
		if date == "" {
			date = utils.FindDateInText(text)
		}
		if date == "" {
			date = utils.TodayISO()
		}
		divs := parsePDFDivisionsYeasNays(text, pdfURL, "ns", "nova_scotia", legislature, session, nextDivNum, date, extractPlainVoteNames)
		results = append(results, divs...)
		nextDivNum += len(divs)
		if len(divs) == 0 {
			nextDivNum++
		}
	}
	clog.Debugf("[ns-votes] parsed %d divisions from %d PDFs", len(results), len(pdfLinks))
	return results, nil
}

// ── Nova Scotia votes — main entry point ──────────────────────────────────────

// crawlNovaScotiaVotes tries the HTML Hansard day-page approach first (current
// sessions publish individual HTML pages with structured vote tables), then
// falls back to the PDF approach for older sessions that only have journal PDFs.
// When allSittings is true it discovers all available assembly/session pairs and
// crawls each one instead of only the auto-detected current session.
func crawlNovaScotiaVotes(indexURL string, legislature, session int, client *http.Client, allSittings bool) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClientWithTimeout(45 * time.Second)
	}

	crawlOneSession := func(url string, leg, sess int) ([]ProvincialDivisionResult, error) {
		if url == "" {
			url = novaScotiaHansardSessionURL("", leg, sess)
		}
		sessionURL := novaScotiaHansardSessionURL(url, leg, sess)
		results, dayPages, err := crawlNovaScotiaVotesFromHTML(sessionURL, leg, sess, client)
		if err != nil {
			clog.Infof("[ns-votes] html approach failed: %v; falling back to pdf", err)
		}
		if dayPages > 0 {
			return results, nil
		}
		if allSittings {
			clog.Debugf("[ns-votes] no html day pages for assembly %d session %d; trying pdf", leg, sess)
		} else {
			clog.Infof("[ns-votes] no html day pages found; falling back to pdf approach")
		}
		return crawlNovaScotiaVotesFromPDF(url, leg, sess, client, allSittings)
	}

	if !allSittings {
		return crawlOneSession(indexURL, legislature, session)
	}

	sessions := discoverNovaScotiaAllSessions(client)
	var allResults []ProvincialDivisionResult
	for _, s := range sessions {
		clog.Debugf("[ns-votes] all-sittings: crawling assembly %d session %d", s.leg, s.sess)
		results, err := crawlOneSession("", s.leg, s.sess)
		if err != nil {
			clog.Infof("[ns-votes] all-sittings: assembly %d session %d: %v", s.leg, s.sess, err)
			continue
		}
		allResults = append(allResults, results...)
	}
	return allResults, nil
}

// CrawlNovaScotiaVotes crawls Nova Scotia votes/proceedings pages.
func CrawlNovaScotiaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlNovaScotiaVotes(indexURL, legislature, session, client, false)
}
