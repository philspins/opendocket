package provincial

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
)

// ── Newfoundland and Labrador regexps ─────────────────────────────────────────

var nlBillSessionDirLinkRe = regexp.MustCompile(`(?i)ga\d+session\d+/?$`)
var newfoundlandBillLinkRe = regexp.MustCompile(`(?i)(housebusiness|bill|legislation)`)

// ── Newfoundland and Labrador bills ───────────────────────────────────────────

func crawlNewfoundlandAndLabradorBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if indexURL == "" {
		indexURL = "https://www.assembly.nl.ca/HouseBusiness/Bills/"
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}
	indexDoc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, fmt.Errorf("nl bills index: %w", err)
	}
	wantSessionDir := fmt.Sprintf("ga%dsession%d/", legislature, session)
	sessionURLs := make([]string, 0)
	seen := make(map[string]bool)
	indexDoc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !nlBillSessionDirLinkRe.MatchString(href) {
			return
		}
		full := resolveRelativeURL(indexURL, href)
		if !strings.HasSuffix(full, "/") {
			full += "/"
		}
		if seen[full] {
			return
		}
		seen[full] = true
		if strings.Contains(strings.ToLower(full), strings.ToLower(wantSessionDir)) {
			sessionURLs = append([]string{full}, sessionURLs...)
			return
		}
		sessionURLs = append(sessionURLs, full)
	})
	if len(sessionURLs) == 0 {
		return crawlProvincialBillsFromIndexWithMatcher(indexURL, "nl", legislature, session, "newfoundland_labrador", client, newfoundlandBillLinkRe)
	}
	if len(sessionURLs) > 4 {
		sessionURLs = sessionURLs[:4]
	}
	out := make([]ProvincialBillStub, 0)
	seenID := make(map[string]bool)
	for _, sessionURL := range sessionURLs {
		sessionDoc, derr := fetchDoc(sessionURL, client)
		if derr != nil {
			continue
		}
		for _, bill := range parseStructuredProvincialBillRows(sessionDoc, sessionURL, "nl", legislature, session, "newfoundland_labrador") {
			if seenID[bill.ID] {
				continue
			}
			seenID[bill.ID] = true
			out = append(out, bill)
		}
		if len(out) > 0 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) == 0 {
		return crawlProvincialBillsFromIndexWithMatcher(indexURL, "nl", legislature, session, "newfoundland_labrador", client, newfoundlandBillLinkRe)
	}
	return out, nil
}

// CrawlNewfoundlandAndLabradorBills crawls Newfoundland and Labrador bills pages.
func CrawlNewfoundlandAndLabradorBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlNewfoundlandAndLabradorBills(indexURL, legislature, session, client)
}

// ── Newfoundland and Labrador votes ───────────────────────────────────────────

// nlSessionDirLinkRe matches NL journal session-directory links like "ga51session1/".
var nlSessionDirLinkRe = regexp.MustCompile(`(?i)ga\d+session\d+/?$`)

// nlJournalPDFLinkRe matches per-day NL journal PDF filenames like "26-04-14.pdf".
var nlJournalPDFLinkRe = regexp.MustCompile(`(?i)\d{2}-\d{2}-\d{2}\.pdf$`)

// nlShortDateRe extracts a YY-MM-DD date from an NL journal PDF URL.
var nlShortDateRe = regexp.MustCompile(`(\d{2})-(\d{2})-(\d{2})\.pdf`)

// nlCarriedRe matches outcome text in NL journal PDFs.
var nlCarriedRe = regexp.MustCompile(`(?i)(?:motion|amendment|bill|resolution)\s+(?:was\s+)?(?:agreed\s+to|carried)`)
var nlNegativedRe = regexp.MustCompile(`(?i)(?:motion|amendment|bill|resolution)\s+(?:was\s+)?(?:defeated|negatived|lost)`)

// nlMotionDescRe captures the motion text for NL journal divisions.
var nlMotionDescRe = regexp.MustCompile(`(?i)on\s+the\s+(?:motion|amendment|question)\s+(?:that\s+)?(.{0,180}?)(?:,\s+the\s+question\s+was\s+put|was\s+(?:agreed|carried|defeated|negatived))`)
var newfoundlandVotesLinkRe = regexp.MustCompile(`(?i)(/business/votes|housebusiness|ga\d+session\d+|votes\.aspx|/votes(?:/|$))`)

// expandNLShortDate converts an NL journal filename date "YY-MM-DD" to ISO "20YY-MM-DD".
func expandNLShortDate(yy, mm, dd string) string {
	return "20" + yy + "-" + mm + "-" + dd
}

// parseNLJournalDivisions extracts division outcomes from an NL journal PDF text.
// NL Journals record proceedings minutes; per-member vote names are not present
// in the accessible static PDF format. This function records outcomes only
// (Yeas/Nays fields set to 0, MemberVotes empty).
func parseNLJournalDivisions(text, detailURL string, legislature, session, startDivisionNumber int, date string) []ProvincialDivisionResult {
	// Try YEAS/AYES member-level data first.
	if genericYeasNaysVoteSectionRe.MatchString(text) {
		return parsePDFDivisionsYeasNays(text, detailURL, "nl", "newfoundland_labrador", legislature, session, startDivisionNumber, date, parseNewBrunswickVoteNames)
	}

	// Outcome-only: find "agreed to" / "defeated" patterns.
	carriedIdxs := nlCarriedRe.FindAllStringIndex(text, -1)
	negIdxs := nlNegativedRe.FindAllStringIndex(text, -1)

	type outcome struct {
		pos    int
		result string
	}
	outcomes := make([]outcome, 0, len(carriedIdxs)+len(negIdxs))
	for _, m := range carriedIdxs {
		outcomes = append(outcomes, outcome{m[0], "Carried"})
	}
	for _, m := range negIdxs {
		outcomes = append(outcomes, outcome{m[0], "Negatived"})
	}
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].pos < outcomes[j].pos })

	// Deduplicate outcomes that are within 400 chars of each other (same motion text).
	deduped := make([]outcome, 0, len(outcomes))
	for _, o := range outcomes {
		if len(deduped) > 0 && o.pos-deduped[len(deduped)-1].pos < 400 {
			continue
		}
		deduped = append(deduped, o)
	}

	results := make([]ProvincialDivisionResult, 0, len(deduped))
	for i, o := range deduped {
		divNum := startDivisionNumber + i
		divID := ProvincialDivisionID("nl", legislature, session, divNum, date)

		start := o.pos - 300
		if start < 0 {
			start = 0
		}
		snippet := text[start:o.pos]
		desc := ""
		if m := nlMotionDescRe.FindStringSubmatch(snippet); len(m) == 2 {
			desc = strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(m[1], "\u00a0", " ")), " "))
		}
		if desc == "" {
			desc = strings.TrimSpace(snippet)
			if len(desc) > 200 {
				desc = desc[len(desc)-200:]
			}
			desc = strings.TrimSpace(desc)
		}
		if desc == "" {
			desc = "Division"
		}

		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID: divID, Parliament: legislature, Session: session,
				Number: divNum, Date: date, Description: desc,
				Yeas: 0, Nays: 0, Result: o.result,
				Chamber: "newfoundland_labrador", DetailURL: detailURL, LastScraped: utils.NowISO(),
			},
		})
	}
	clog.Debugf("[nl-votes] %s: parsed %d divisions (outcome-only)", date, len(results))
	return results
}

// ParseNLJournalDivisionsForTest is test-only access to NL journal parsing.
func ParseNLJournalDivisionsForTest(text, detailURL string, legislature, session, startDivisionNumber int, date string) []ProvincialDivisionResult {
	return parseNLJournalDivisions(text, detailURL, legislature, session, startDivisionNumber, date)
}

// crawlNLVotesFromPDF performs a two-level crawl of the NL assembly journals:
//
//	/HouseBusiness/Journals/ → ga51session1/ → YY-MM-DD.pdf → parseNLJournalDivisions
func crawlNLVotesFromPDF(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Infof("[nl-votes] fetching journals index: %s", indexURL)
	indexDoc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, fmt.Errorf("nl journals index: %w", err)
	}

	// Level 1: session directories.
	var sessionDirs []string
	seenDir := make(map[string]bool)
	indexDoc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !nlSessionDirLinkRe.MatchString(href) {
			return
		}
		full := resolveRelativeURL(indexURL, href)
		if !strings.HasSuffix(full, "/") {
			full += "/"
		}
		if seenDir[full] {
			return
		}
		seenDir[full] = true
		sessionDirs = append(sessionDirs, full)
	})
	if len(sessionDirs) == 0 {
		clog.Infof("[nl-votes] no session directories found; falling back to generic parser")
		return crawlGenericProvincialVotesWithMatcher(indexURL, "nl", "newfoundland_labrador", legislature, session, client, newfoundlandVotesLinkRe)
	}
	sort.Strings(sessionDirs)
	if len(sessionDirs) > 4 {
		sessionDirs = sessionDirs[len(sessionDirs)-4:]
	}

	// Level 2: per-day PDF links in each session directory.
	var pdfLinks []string
	seenPDF := make(map[string]bool)
	for _, dirURL := range sessionDirs {
		dirDoc, derr := fetchDoc(dirURL, client)
		if derr != nil {
			clog.Debugf("[nl-votes] skip session dir %s: %v", dirURL, derr)
			continue
		}
		dirDoc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
			href := normalizeHref(a.AttrOr("href", ""))
			if href == "" || !nlJournalPDFLinkRe.MatchString(href) {
				return
			}
			full := resolveRelativeURL(dirURL, href)
			if seenPDF[full] {
				return
			}
			seenPDF[full] = true
			pdfLinks = append(pdfLinks, full)
		})
	}

	sort.Strings(pdfLinks)
	if len(pdfLinks) > 80 {
		pdfLinks = pdfLinks[len(pdfLinks)-80:]
	}
	if len(pdfLinks) == 0 {
		clog.Infof("[nl-votes] no journal PDFs discovered")
		return nil, nil
	}

	var results []ProvincialDivisionResult
	nextDivNum := 1
	for _, pdfURL := range pdfLinks {
		text, terr := downloadAndExtractPDFText(pdfURL, "nl", client)
		if terr != nil {
			clog.Debugf("[nl-votes] skip pdf %s: %v", pdfURL, terr)
			continue
		}
		date := ""
		if m := nlShortDateRe.FindStringSubmatch(pdfURL); len(m) == 4 {
			date = expandNLShortDate(m[1], m[2], m[3])
		}
		if date == "" {
			date = extractDateFromURL(pdfURL)
		}
		if date == "" {
			date = utils.FindDateInText(text)
		}
		if date == "" {
			date = utils.TodayISO()
		}
		divs := parseNLJournalDivisions(text, pdfURL, legislature, session, nextDivNum, date)
		results = append(results, divs...)
		nextDivNum += len(divs)
		if len(divs) == 0 {
			nextDivNum++
		}
	}
	clog.Infof("[nl-votes] parsed %d divisions from %d PDFs", len(results), len(pdfLinks))
	return results, nil
}

// crawlNewfoundlandAndLabradorVotes crawls NL assembly journal PDFs for division outcomes.
// NL Journal PDFs contain proceedings minutes; per-member AYES/NAYS name lists are not
// present in the accessible static PDF format. Division results (Carried/Negatived) are
// extracted from the motion outcome text; yea/nay counts are not available.
// Two-level crawl: /HouseBusiness/Journals/ → ga51session1/ → YY-MM-DD.pdf
func crawlNewfoundlandAndLabradorVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if indexURL == "" {
		indexURL = "https://www.assembly.nl.ca/HouseBusiness/Journals/"
	}
	return crawlNLVotesFromPDF(indexURL, legislature, session, client)
}

// CrawlNewfoundlandAndLabradorVotes crawls Newfoundland and Labrador votes/proceedings pages.
func CrawlNewfoundlandAndLabradorVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlNewfoundlandAndLabradorVotes(indexURL, legislature, session, client)
}
