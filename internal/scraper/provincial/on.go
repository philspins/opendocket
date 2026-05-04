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
var ontarioExceptionSameMoRe = regexp.MustCompile(`(?i)\b([A-Za-z]+)\s+(\d{1,2})\s+to\s+(\d{1,2})\b`)
var ontarioExceptionCrossMoRe = regexp.MustCompile(`(?i)\b([A-Za-z]+)\s+(\d{1,2})\s+to\s+([A-Za-z]+)\s+(\d{1,2})\b`)
var ontarioExceptionSingleRe = regexp.MustCompile(`(?i)\b([A-Za-z]+)\s+(\d{1,2})\b`)
var anyCalendarYearRe = regexp.MustCompile(`(?i)Parliamentary calendar\s+\d{4}`)
var ontarioCalendarDateRe = regexp.MustCompile(`\b(?:January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},?\s+\d{4}\b`)
var stripHTMLTagRe = regexp.MustCompile(`<[^>]+>`)

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
	clog.Debugf("[ontario-votes] fetching session index: %s", indexURL)

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

	clog.Infof("[ontario-votes] found %d sitting dates with V&P", len(dates))
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
	clog.Debugf("[ontario-votes] scraping V&P: %s", vpURL)

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

// OntarioCalendarDates parses the Ontario parliamentary calendar page for in-session dates.
func OntarioCalendarDates(body string, year int) ([]string, bool) {
	text := normalizeCalendarText(body)
	yearText, ok := ontarioCalendarTextForYear(text, year)
	if !ok {
		return nil, false
	}
	exceptionsIdx := strings.Index(strings.ToLower(yearText), "with the following exceptions")
	if exceptionsIdx < 0 {
		return nil, false
	}
	prefix := yearText[:exceptionsIdx]
	mainDates := ontarioCalendarDateRe.FindAllString(prefix, -1)
	if len(mainDates) < 2 {
		return nil, false
	}
	mainStart, err := time.Parse("January 2, 2006", mainDates[len(mainDates)-2])
	if err != nil {
		return nil, false
	}
	mainEnd, err := time.Parse("January 2, 2006", mainDates[len(mainDates)-1])
	if err != nil {
		return nil, false
	}
	mainStart = dayStartUTC(mainStart)
	mainEnd = dayStartUTC(mainEnd)
	exceptionsText := yearText[exceptionsIdx:]

	excluded := map[string]struct{}{}
	for _, m := range ontarioExceptionCrossMoRe.FindAllStringSubmatch(exceptionsText, -1) {
		if len(m) != 5 {
			continue
		}
		startDate, err1 := parseMonthDayWithYear(m[1], m[2], year)
		endDate, err2 := parseMonthDayWithYear(m[3], m[4], year)
		if err1 != nil || err2 != nil {
			continue
		}
		for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
			excluded[d.Format("2006-01-02")] = struct{}{}
		}
	}
	for _, m := range ontarioExceptionSameMoRe.FindAllStringSubmatch(exceptionsText, -1) {
		if len(m) != 4 {
			continue
		}
		startDate, err1 := parseMonthDayWithYear(m[1], m[2], year)
		endDate, err2 := parseMonthDayWithYear(m[1], m[3], year)
		if err1 != nil || err2 != nil {
			continue
		}
		for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
			excluded[d.Format("2006-01-02")] = struct{}{}
		}
	}
	for _, m := range ontarioExceptionSingleRe.FindAllStringSubmatch(exceptionsText, -1) {
		if len(m) != 3 {
			continue
		}
		d, err := parseMonthDayWithYear(m[1], m[2], year)
		if err != nil {
			continue
		}
		excluded[d.Format("2006-01-02")] = struct{}{}
	}

	var out []string
	for d := mainStart; !d.After(mainEnd); d = d.AddDate(0, 0, 1) {
		wd := d.Weekday()
		if wd < time.Monday || wd > time.Thursday {
			continue
		}
		iso := d.Format("2006-01-02")
		if _, skip := excluded[iso]; skip {
			continue
		}
		out = append(out, iso)
	}
	return out, true
}

func normalizeCalendarText(body string) string {
	text := stripHTMLTagRe.ReplaceAllString(body, " ")
	text = strings.ReplaceAll(text, "\u00a0", " ")
	return strings.Join(strings.Fields(text), " ")
}

func parseMonthDayWithYear(month, day string, year int) (time.Time, error) {
	dayNum, err := strconv.Atoi(strings.TrimSpace(day))
	if err != nil {
		return time.Time{}, err
	}
	dateStr := strings.TrimSpace(month) + " " + strconv.Itoa(dayNum) + ", " + strconv.Itoa(year)
	t, err := time.Parse("January 2, 2006", dateStr)
	if err != nil {
		return time.Time{}, err
	}
	return dayStartUTC(t), nil
}

func dayStartUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func ontarioCalendarTextForYear(text string, year int) (string, bool) {
	marker := "Parliamentary calendar " + strconv.Itoa(year)
	start := strings.Index(strings.ToLower(text), strings.ToLower(marker))
	if start < 0 {
		return "", false
	}
	rest := text[start:]
	loc := anyCalendarYearRe.FindStringIndex(rest[len(marker):])
	if loc == nil {
		return rest, true
	}
	sectionEnd := len(marker) + loc[0]
	if sectionEnd <= 0 || sectionEnd > len(rest) {
		return rest, true
	}
	return rest[:sectionEnd], true
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

	clog.Debugf("[ontario-votes] %s: parsed %d divisions", date, len(results))
	return results
}
