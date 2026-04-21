package provincial

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/open-democracy/internal/utils"
)

// ── Manitoba regexps ──────────────────────────────────────────────────────────

var manitobaCurrentSessionLinkRe = regexp.MustCompile(`(?i)\.\./\d+-\d+/index\.php$`)
var manitobaBillLinkRe = regexp.MustCompile(`(?i)(businessofthehouse|bill|legislature)`)

// ── Manitoba bills ────────────────────────────────────────────────────────────

func crawlManitobaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if indexURL == "" {
		indexURL = "https://web2.gov.mb.ca/bills/sess/index.php"
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}
	bills, err := crawlManitobaBillsCurrentSession(indexURL, legislature, session, client)
	if err == nil && len(bills) > 0 {
		return bills, nil
	}
	return crawlProvincialBillsFromIndexWithMatcher(indexURL, "mb", legislature, session, "manitoba", client, manitobaBillLinkRe)
}

func crawlManitobaBillsCurrentSession(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	indexDoc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, err
	}
	currentURL := indexURL
	indexDoc.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !manitobaCurrentSessionLinkRe.MatchString(href) {
			return true
		}
		if goquery.NodeName(a.Parent()) != "li" || strings.ToLower(strings.TrimSpace(a.Parent().AttrOr("id", ""))) != "active" {
			return true
		}
		currentURL = resolveRelativeURL(indexURL, href)
		return false
	})
	if currentURL != indexURL {
		indexDoc, err = fetchDoc(currentURL, client)
		if err != nil {
			return nil, err
		}
	}
	return parseManitobaBillRows(indexDoc, currentURL, legislature, session), nil
}

func parseManitobaBillRows(doc *goquery.Document, sourceURL string, legislature, session int) []ProvincialBillStub {
	seen := make(map[string]bool)
	out := make([]ProvincialBillStub, 0)
	doc.Find("table.index tr").Each(func(_ int, tr *goquery.Selection) {
		cells := tr.Find("td")
		if cells.Length() < 3 {
			return
		}
		billNumber := strings.TrimSpace(strings.Join(strings.Fields(cells.First().Text()), " "))
		if !provincialStandaloneNumberRe.MatchString(billNumber) {
			billNumber = extractProvincialBillNumberWithContext(billNumber, "", strings.TrimSpace(strings.Join(strings.Fields(tr.Text()), " ")))
		}
		if billNumber == "" {
			return
		}
		id := ProvincialBillID("mb", legislature, session, billNumber)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		titleCell := cells.Eq(2)
		title := strings.TrimSpace(strings.Join(strings.Fields(titleCell.Text()), " "))
		detailURL := sourceURL
		if link := titleCell.Find("a[href]").First(); link.Length() > 0 {
			detailURL = resolveRelativeURL(sourceURL, link.AttrOr("href", ""))
		}
		out = append(out, ProvincialBillStub{
			ID:           id,
			ProvinceCode: "mb",
			Parliament:   legislature,
			Session:      session,
			Number:       billNumber,
			Title:        title,
			Chamber:      "manitoba",
			DetailURL:    detailURL,
			SourceURL:    sourceURL,
			LastScraped:  utils.NowISO(),
		})
	})
	return out
}

// CrawlManitobaBills crawls Manitoba bills pages.
func CrawlManitobaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlManitobaBills(indexURL, legislature, session, client)
}

// ── Manitoba votes ────────────────────────────────────────────────────────────

// mbVotesPDFLinkRe matches per-day Votes and Proceedings PDF links on MB session pages.
var mbVotesPDFLinkRe = regexp.MustCompile(`(?i)\d+(?:rd|th|st|nd)/votes_\d+\.pdf`)
var mbAyeNaySectionRe = regexp.MustCompile(`(?is)\bAYE\b\s+(.{1,1000}?)\.{3,}\s*(\d{1,3})\s+\bNAY\b\s+(.{0,600}?)\.{3,}\s*(\d{1,3})`)
var mbMotionDescriptionRe = regexp.MustCompile(`(?is)(THAT\s+Bill(?:\s*\(No\.\s*\d+\)|\s+No\.\s*\d+).{0,320}?|Resolution\s+No\.\s*\d+\s*:.{0,320}?)(?:And\s+the\s+Question\s+being\s+put|It\s+was\s+(?:agreed|negatived)\s+to,\s+on\s+the\s+following\s+division|$)`)
var manitobaVotesLinkRe = regexp.MustCompile(
	`(?i)(recorded_votes|votes|journals?|hansard|\d+(?:rd|th|st|nd)/\d+(?:rd|th|st|nd)_\d+\.html|/\d+(?:rd|th|st|nd)/votes_\d+\.pdf)`)

// mbSessionPageLinkRe matches session-index page links on the MB V&P index page.
// Links have the form "43rd/43rd_3rd.html" (ordinal suffix on both the legislature
// and session components), so the session number must also end with an ordinal.
var mbSessionPageLinkRe = regexp.MustCompile(`(?i)\d+(?:rd|th|st|nd)/\d+(?:rd|th|st|nd)_\d+(?:rd|th|st|nd)\.html`)

func manitobaSessionPageMatches(href string, legislature, session int) bool {
	if href == "" {
		return false
	}
	want := strings.ToLower(fmt.Sprintf("%s/%s_%s.html", parliamentOrdinal(legislature), parliamentOrdinal(legislature), parliamentOrdinal(session)))
	return strings.Contains(strings.ToLower(href), want)
}

// crawlManitobaVotesFromPDF performs a two-level crawl:
//
//	votes_proceedings.html → 43rd/43rd_3rd.html → 3rd/votes_NNN.pdf → parsePDFDivisionsYeasNays
func crawlManitobaVotesFromPDF(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	log.Printf("[mb-votes] fetching index: %s", indexURL)
	indexDoc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, fmt.Errorf("mb votes index: %w", err)
	}

	// Level 1: find session-index pages.
	var sessionLinks []string
	var matchingSessionLinks []string
	seenSession := make(map[string]bool)
	indexDoc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !mbSessionPageLinkRe.MatchString(href) {
			return
		}
		full := resolveRelativeURL(indexURL, href)
		if seenSession[full] {
			return
		}
		seenSession[full] = true
		sessionLinks = append(sessionLinks, full)
		if manitobaSessionPageMatches(href, legislature, session) {
			matchingSessionLinks = append(matchingSessionLinks, full)
		}
	})
	if len(matchingSessionLinks) > 0 {
		sessionLinks = matchingSessionLinks
	}
	if len(sessionLinks) == 0 {
		log.Printf("[mb-votes] no session pages discovered; falling back to generic parser")
		return crawlGenericProvincialVotesWithMatcher(indexURL, "mb", "manitoba", legislature, session, client, manitobaVotesLinkRe)
	}
	sort.Strings(sessionLinks)
	if len(sessionLinks) > 6 {
		sessionLinks = sessionLinks[len(sessionLinks)-6:]
	}

	// Level 2: for each session page, collect PDF links.
	var pdfLinks []string
	seenPDF := make(map[string]bool)
	for _, sessURL := range sessionLinks {
		sessDoc, serr := fetchDoc(sessURL, client)
		if serr != nil {
			log.Printf("[mb-votes] skip session %s: %v", sessURL, serr)
			continue
		}
		sessDoc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
			href := normalizeHref(a.AttrOr("href", ""))
			if href == "" || !mbVotesPDFLinkRe.MatchString(href) {
				return
			}
			full := resolveRelativeURL(sessURL, href)
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
		log.Printf("[mb-votes] no VP PDFs discovered; falling back to generic parser")
		return crawlGenericProvincialVotesWithMatcher(indexURL, "mb", "manitoba", legislature, session, client, manitobaVotesLinkRe)
	}

	var results []ProvincialDivisionResult
	nextDivNum := 1
	for _, pdfURL := range pdfLinks {
		text, terr := downloadAndExtractPDFText(pdfURL, "mb", client)
		if terr != nil {
			log.Printf("[mb-votes] skip pdf %s: %v", pdfURL, terr)
			continue
		}
		date := extractDateFromURL(pdfURL)
		if date == "" {
			date = utils.FindDateInText(text)
		}
		if date == "" {
			date = utils.TodayISO()
		}
		divs := parseManitobaAyeNayDivisions(text, pdfURL, legislature, session, nextDivNum, date)
		if len(divs) == 0 {
			divs = parsePDFDivisionsYeasNays(text, pdfURL, "mb", "manitoba", legislature, session, nextDivNum, date, extractPlainVoteNames)
		}
		results = append(results, divs...)
		nextDivNum += len(divs)
		if len(divs) == 0 {
			nextDivNum++
		}
	}
	log.Printf("[mb-votes] parsed %d divisions from %d PDFs", len(results), len(pdfLinks))
	return results, nil
}

func parseManitobaAyeNayDivisions(text, detailURL string, legislature, session, startDivisionNumber int, date string) []ProvincialDivisionResult {
	matches := mbAyeNaySectionRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	results := make([]ProvincialDivisionResult, 0, len(matches))
	for _, match := range matches {
		yeaBlock := text[match[2]:match[3]]
		yeas, _ := strconv.Atoi(text[match[4]:match[5]])
		nayBlock := text[match[6]:match[7]]
		nays, _ := strconv.Atoi(text[match[8]:match[9]])
		if yeas == 0 && nays == 0 {
			continue
		}
		divNum := startDivisionNumber + len(results)
		divID := ProvincialDivisionID("mb", legislature, session, divNum, date)
		desc := extractManitobaDivisionDescription(text, match[0])
		if desc == "" {
			desc = "Recorded division"
		}
		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}
		votes := make([]ProvincialMemberVote, 0, yeas+nays)
		for _, name := range extractPlainVoteNames(yeaBlock) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Yea"})
		}
		for _, name := range extractPlainVoteNames(nayBlock) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Nay"})
		}
		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID: divID, Parliament: legislature, Session: session,
				Number: divNum, Date: date, Description: desc,
				Yeas: yeas, Nays: nays, Result: result,
				Chamber: "manitoba", DetailURL: detailURL, LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
	}
	return results
}

func extractManitobaDivisionDescription(text string, markerStart int) string {
	start := markerStart - 1200
	if start < 0 {
		start = 0
	}
	context := strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(text[start:markerStart], "\u00a0", " ")), " "))
	if matches := mbMotionDescriptionRe.FindAllStringSubmatch(context, -1); len(matches) > 0 {
		return strings.TrimSpace(matches[len(matches)-1][1])
	}
	return newBrunswickDescriptionFromContext(text, markerStart)
}

// ParseManitobaAyeNayDivisionsForTest is test-only access to the Manitoba AYE/NAY parser.
func ParseManitobaAyeNayDivisionsForTest(text, detailURL string, legislature, session, startDivisionNumber int, date string) []ProvincialDivisionResult {
	return parseManitobaAyeNayDivisions(text, detailURL, legislature, session, startDivisionNumber, date)
}

// CrawlManitobaVotes crawls Manitoba recorded votes/journal pages.
// Performs a two-level crawl: votes_proceedings.html → session-index page
// (e.g. 43rd/43rd_3rd.html) → per-day PDF (e.g. 3rd/votes_041.pdf).
// Each PDF is parsed for YEAS/NAYS recorded divisions using a format
// adapted from the New Brunswick journal parser.
func crawlManitobaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if indexURL == "" {
		indexURL = "https://www.gov.mb.ca/legislature/business/votes_proceedings.html"
	}
	return crawlManitobaVotesFromPDF(indexURL, legislature, session, client)
}

// CrawlManitobaVotes crawls Manitoba votes/proceedings pages.
func CrawlManitobaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlManitobaVotes(indexURL, legislature, session, client)
}
