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

// ── Ontario constants ─────────────────────────────────────────────────────────

const (
	// Ontario
	OntarioVPIndexURL = "https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1"
	OntarioParliament = 44
	OntarioSession    = 1
)

// ── Ontario bills ─────────────────────────────────────────────────────────────

func crawlOntarioBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if indexURL == "" {
		indexURL = "https://www.ola.org/en/legislative-business"
	}
	return crawlProvincialBillsFromIndexWithMatcher(indexURL, "on", legislature, session, "ontario", client, genericBillLinkRe)
}

// CrawlOntarioBills crawls Ontario bills pages.
func CrawlOntarioBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlOntarioBills(indexURL, legislature, session, client)
}

// ── Ontario Votes and Proceedings ─────────────────────────────────────────────

var ontarioDivCountRe = regexp.MustCompile(`\((\d+)\)`)
var ontarioHouseDocDatePathRe = regexp.MustCompile(`/parliament-\d+/session-\d+/(\d{4}-\d{2}-\d{2})/`)

// CrawlOntarioVPSittingDates fetches the Ontario legislature session index page
// and returns the list of sitting dates that have a Votes and Proceedings document.
func crawlOntarioVPSittingDates(indexURL string, parliament, session int, client *http.Client) ([]string, error) {
	if indexURL == "" {
		indexURL = fmt.Sprintf(
			"https://www.ola.org/en/legislative-business/house-documents/parliament-%d/session-%d",
			parliament, session,
		)
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}
	log.Printf("[ontario-votes] fetching session index: %s", indexURL)

	doc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, fmt.Errorf("ontario VP index: %w", err)
	}

	seen := make(map[string]bool)
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")

		// Legacy/expected link: .../YYYY-MM-DD/votes-proceedings
		if strings.HasSuffix(href, "/votes-proceedings") {
			withoutSuffix := strings.TrimSuffix(href, "/votes-proceedings")
			if i := strings.LastIndex(withoutSuffix, "/"); i >= 0 {
				date := withoutSuffix[i+1:]
				if len(date) == 10 && date[4] == '-' && date[7] == '-' {
					seen[date] = true
					return
				}
			}
		}

		// Current OLA index commonly links to /hansard for dates where V&P
		// content exists. /orders-notices dates can legitimately have no V&P page.
		if strings.HasSuffix(href, "/hansard") {
			if m := ontarioHouseDocDatePathRe.FindStringSubmatch(href); len(m) == 2 {
				seen[m[1]] = true
			}
			return
		}

		if m := ontarioHouseDocDatePathRe.FindStringSubmatch(href); len(m) == 2 && strings.Contains(href, "/votes-proceedings") {
			seen[m[1]] = true
		}
	})

	dates := make([]string, 0, len(seen))
	for d := range seen {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	log.Printf("[ontario-votes] found %d sitting dates with V&P", len(dates))
	return dates, nil
}

// CrawlOntarioVPSittingDates is the exported wrapper.
func CrawlOntarioVPSittingDates(indexURL string, parliament, session int, client *http.Client) ([]string, error) {
	return crawlOntarioVPSittingDates(indexURL, parliament, session, client)
}

// ontarioVPDayURL returns the canonical URL for the Ontario V&P page on a given date.
func ontarioVPDayURL(parliament, session int, date string) string {
	return fmt.Sprintf(
		"https://www.ola.org/en/legislative-business/house-documents/parliament-%d/session-%d/%s/votes-proceedings",
		parliament, session, date,
	)
}

// OntarioVPDayURL is the exported wrapper.
func OntarioVPDayURL(parliament, session int, date string) string {
	return ontarioVPDayURL(parliament, session, date)
}

// crawlOntarioVPDay scrapes a single Ontario Votes and Proceedings page for the
// given date. vpURL is the full URL for the page (use OntarioVPDayURL to build it).
func crawlOntarioVPDay(vpURL string, parliament, session int, date string, client *http.Client) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	log.Printf("[ontario-votes] scraping V&P: %s", vpURL)

	doc, err := fetchDoc(vpURL, client)
	if err != nil {
		return nil, fmt.Errorf("ontario VP %s: %w", date, err)
	}

	return parseOntarioVPDoc(doc, parliament, session, date), nil
}

// CrawlOntarioVPDay is the exported wrapper.
func CrawlOntarioVPDay(vpURL string, parliament, session int, date string, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlOntarioVPDay(vpURL, parliament, session, date, client)
}

func normaliseOntarioEventText(text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	text = strings.TrimSuffix(text, ":")
	return text
}

func isOntarioDivisionOutcomeText(text string) bool {
	switch strings.ToLower(normaliseOntarioEventText(text)) {
	case "carried on the following division", "lost on the following division", "negatived on the following division":
		return true
	default:
		return false
	}
}

func extractOntarioDivisionDescription(wrapper *goquery.Selection) string {
	baseTable := wrapper.Closest("table")
	if baseTable.Length() == 0 {
		return ""
	}

	desc := ""
	baseTable.PrevAllFiltered("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		text := normaliseOntarioEventText(table.Find("td[lang='en']").First().Text())
		if text == "" || isOntarioDivisionOutcomeText(text) {
			return true
		}
		desc = text
		return false
	})
	return desc
}

// parseOntarioVPDoc is the pure HTML-parsing logic for Ontario V&P pages.
// Separated from CrawlOntarioVPDay so tests can call it without a network round-trip.
func parseOntarioVPDoc(doc *goquery.Document, parliament, session int, date string) []ProvincialDivisionResult {
	var results []ProvincialDivisionResult
	divNum := 0

	// Each recorded division is rendered inside a div.datawrapper that contains
	// alternating h5.divisionHeader / table.votesList pairs (Ayes then Nays).
	doc.Find("div.datawrapper").Each(func(_ int, wrapper *goquery.Selection) {
		if wrapper.Find("h5.divisionHeader").Length() == 0 || wrapper.Find("table.votesList").Length() == 0 {
			return
		}

		divNum++
		divID := ProvincialDivisionID("on", parliament, session, divNum, date)

		var votes []ProvincialMemberVote
		yeas, nays := 0, 0
		currentVoteType := ""

		wrapper.Children().Each(func(_ int, child *goquery.Selection) {
			if child.Is("h5.divisionHeader") {
				headerText := child.Text()
				enText := strings.ToLower(strings.TrimSpace(child.Find("span[lang='en']").First().Text()))
				switch enText {
				case "ayes":
					currentVoteType = "Yea"
					if m := ontarioDivCountRe.FindStringSubmatch(headerText); len(m) == 2 {
						yeas, _ = strconv.Atoi(m[1])
					}
				case "nays":
					currentVoteType = "Nay"
					if m := ontarioDivCountRe.FindStringSubmatch(headerText); len(m) == 2 {
						nays, _ = strconv.Atoi(m[1])
					}
				}
				return
			}

			if !child.Is("table.votesList") || currentVoteType == "" {
				return
			}
			vt := currentVoteType
			child.Find("td div[lang='en']").Each(func(_ int, div *goquery.Selection) {
				if div.HasClass("docHide") {
					return
				}
				name := strings.TrimSpace(div.Text())
				if name == "" || name == "\u00a0" {
					return
				}
				votes = append(votes, ProvincialMemberVote{
					DivisionID: divID,
					MemberName: name,
					Vote:       vt,
				})
			})
		})

		desc := extractOntarioDivisionDescription(wrapper)

		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID:          divID,
				Parliament:  parliament,
				Session:     session,
				Number:      divNum,
				Date:        date,
				Description: desc,
				Yeas:        yeas,
				Nays:        nays,
				Result:      result,
				Chamber:     "ontario",
				LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
	})

	log.Printf("[ontario-votes] %s: parsed %d divisions", date, len(results))
	return results
}
