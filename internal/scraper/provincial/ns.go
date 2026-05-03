package provincial

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
)

// ── Nova Scotia regexps ───────────────────────────────────────────────────────

var novaScotiaBillLinkRe = regexp.MustCompile(`(?i)(bills-statutes|bill|legislative-business)`)

// nsVotesPDFLinkRe matches NS journals and Hansard PDF links under the default
// files path.
var nsVotesPDFLinkRe = regexp.MustCompile(`(?i)/sites/default/files/pdfs/proceedings/(?:journals|hansard)/[^"'\s]+\.pdf(?:\?[^"'\s]*)?`)

// nsHansardDayPageRe matches hrefs to individual Hansard day pages, e.g.
// "/legislative-business/hansard-debates/assembly-65-session-1/house_26apr09"
var nsHansardDayPageRe = regexp.MustCompile(`(?i)/hansard-debates/assembly-\d+-session-\d+/house_\w+`)

// nsHansardSlugRe extracts the date components from a Hansard day-page slug:
// "house_26apr09" → year="26", month="apr", day="09"
var nsHansardSlugRe = regexp.MustCompile(`(?i)house_(\d{2})([a-z]{3})(\d{1,2})`)

var nsMonthAbbr = map[string]string{
	"jan": "01", "feb": "02", "mar": "03", "apr": "04",
	"may": "05", "jun": "06", "jul": "07", "aug": "08",
	"sep": "09", "oct": "10", "nov": "11", "dec": "12",
}

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

// crawlNovaScotiaVotesFromHTML fetches the NS Hansard session index page,
// discovers individual day-page links, and parses each for vote tables.
// It returns the results and the number of day pages found (0 means the
// session page has no individual Hansard pages, likely an older session).
func crawlNovaScotiaVotesFromHTML(sessionURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, int, error) {
	clog.Infof("[ns-votes] fetching hansard session index for html: %s", sessionURL)
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
			clog.Debugf("[ns-votes] skip day page %s: %v", pageURL, derr)
			continue
		}
		divs := parseNSHansardHTMLPage(dayDoc, pageURL, legislature, session, divNum)
		results = append(results, divs...)
		divNum += len(divs)
		if len(divs) == 0 {
			divNum++
		}
	}
	clog.Infof("[ns-votes] parsed %d divisions from %d html day pages", len(results), len(dayURLs))
	return results, len(dayURLs), nil
}

// parseNSHansardHTMLPage finds all YEAS/NAYS vote tables on a single Hansard
// day page and returns one ProvincialDivisionResult per table.
func parseNSHansardHTMLPage(doc *goquery.Document, pageURL string, legislature, session, startDivNum int) []ProvincialDivisionResult {
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

func crawlNovaScotiaVotesFromPDF(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	sessionURL := novaScotiaHansardSessionURL(indexURL, legislature, session)
	clog.Infof("[ns-votes] fetching hansard session index for pdf: %s", sessionURL)
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
			clog.Infof("[ns-votes] no vote PDFs discovered for legislature=%d session=%d", legislature, session)
			return nil, nil
		}
	}

	var results []ProvincialDivisionResult
	nextDivNum := 1
	for _, pdfURL := range pdfLinks {
		text, terr := downloadAndExtractPDFText(pdfURL, "ns", client)
		if terr != nil {
			clog.Debugf("[ns-votes] skip pdf %s: %v", pdfURL, terr)
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
	clog.Infof("[ns-votes] parsed %d divisions from %d PDFs", len(results), len(pdfLinks))
	return results, nil
}

// ── Nova Scotia votes — main entry point ──────────────────────────────────────

// crawlNovaScotiaVotes tries the HTML Hansard day-page approach first (current
// sessions publish individual HTML pages with structured vote tables), then
// falls back to the PDF approach for older sessions that only have journal PDFs.
func crawlNovaScotiaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if indexURL == "" {
		indexURL = novaScotiaHansardSessionURL("", legislature, session)
	}
	if client == nil {
		client = utils.NewHTTPClientWithTimeout(45 * time.Second)
	}

	sessionURL := novaScotiaHansardSessionURL(indexURL, legislature, session)
	results, dayPages, err := crawlNovaScotiaVotesFromHTML(sessionURL, legislature, session, client)
	if err != nil {
		clog.Infof("[ns-votes] html approach failed: %v; falling back to pdf", err)
	}
	if dayPages > 0 {
		return results, nil
	}

	clog.Infof("[ns-votes] no html day pages found; falling back to pdf approach")
	return crawlNovaScotiaVotesFromPDF(indexURL, legislature, session, client)
}

// CrawlNovaScotiaVotes crawls Nova Scotia votes/proceedings pages.
func CrawlNovaScotiaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlNovaScotiaVotes(indexURL, legislature, session, client)
}
