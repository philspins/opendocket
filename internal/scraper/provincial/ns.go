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

// ── Nova Scotia votes ─────────────────────────────────────────────────────────

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
func crawlNovaScotiaVotesFromPDF(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	sessionURL := novaScotiaHansardSessionURL(indexURL, legislature, session)
	clog.Infof("[ns-votes] fetching hansard session index: %s", sessionURL)
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
	clog.Infof("[ns-votes] parsed %d divisions from %d PDFs", len(results), len(pdfLinks))
	return results, nil
}

// CrawlNovaScotiaVotes crawls NS journals/proceedings pages.
//
// The live NS journals page no longer publishes current-session PDFs. Current
// divisions are exposed from per-session Hansard pages whose PDFs contain
// recorded YEAS/NAYS blocks that the generic PDF parser can consume. The old
// journals listing remains as a fallback for older sessions.
func crawlNovaScotiaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if indexURL == "" {
		indexURL = novaScotiaHansardSessionURL("", legislature, session)
	}
	if client == nil {
		// NS Hansard session pages are still large Drupal responses; use an
		// extended timeout.
		client = utils.NewHTTPClientWithTimeout(45 * time.Second)
	}
	return crawlNovaScotiaVotesFromPDF(indexURL, legislature, session, client)
}

// CrawlNovaScotiaVotes crawls Nova Scotia votes/proceedings pages.
func CrawlNovaScotiaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlNovaScotiaVotes(indexURL, legislature, session, client)
}
