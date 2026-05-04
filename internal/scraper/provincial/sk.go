package provincial

import (
	"fmt"
	"image/color"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
)

// ── Saskatchewan constants ────────────────────────────────────────────────────

const (
	// Saskatchewan
	// NOTE: The archive URL currently returns HTTP 500. The new SK minutes-votes page
	// (/legislative-business/minutes-votes/) loads document links via JavaScript and
	// has no static HTML equivalents. CrawlSaskatchewanMinutesLinks will fail; the
	// error is now logged as a warning and the crawl continues with 0 divisions.
	SaskatchewanArchiveURL  = "https://www.legassembly.sk.ca/legislative-business/archive/?Start=&End=&Type=Assembly"
	SaskatchewanLegislature = 30
	SaskatchewanSession     = 2
)

// ── Saskatchewan regexps ──────────────────────────────────────────────────────

var saskatchewanProgressPDFRe = regexp.MustCompile(`(?i)progress(?:-of)?-bills.*\.pdf$`)
var saskatchewanProgressEntryRe = regexp.MustCompile(`(?i)\b(\d{1,3}[A-Z]?)\s+(?:EN\s+)?\*\s+(.{1,260}?)\s+[A-Z][A-Za-z''.\-]+,\s+[A-Z][A-Za-z''.\-]+(?:\s+[A-Z][A-Za-z''.\-]+)?\s+[A-Z][a-z]{2}\s+\d{2},\s+\d{4}`)
var saskatchewanBillLinkRe = regexp.MustCompile(`(?i)(legislative-business/bills|/bills/)`)
var skDateFromURLRe = regexp.MustCompile(`/(\d{8})Minutes-HTML\.htm`)
var skCountRe = regexp.MustCompile(`(?:YEAS|NAYS)[^\d]*(\d+)`)

// ── Saskatchewan bills ────────────────────────────────────────────────────────

func crawlSaskatchewanBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	indexURL = normalizeSaskatchewanBillsURL(indexURL)
	if indexURL == "" {
		indexURL = "https://www.legassembly.sk.ca/legislative-business/bills/"
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}
	bills, err := crawlSaskatchewanBillsFromProgressPDF(indexURL, legislature, session, client)
	if err == nil && len(bills) > 0 {
		return bills, nil
	}
	return crawlProvincialBillsFromIndexWithMatcher(indexURL, "sk", legislature, session, "saskatchewan", client, saskatchewanBillLinkRe)
}

func normalizeSaskatchewanBillsURL(indexURL string) string {
	trimmed := strings.TrimSpace(indexURL)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if lower == "https://www.legassembly.sk.ca/legislative-business" || lower == "https://www.legassembly.sk.ca/legislative-business/" {
		return "https://www.legassembly.sk.ca/legislative-business/bills/"
	}
	return trimmed
}

// NormalizeSaskatchewanBillsURLForTest is test-only access to URL normalization.
func NormalizeSaskatchewanBillsURLForTest(indexURL string) string {
	return normalizeSaskatchewanBillsURL(indexURL)
}

func crawlSaskatchewanBillsFromProgressPDF(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	doc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, err
	}
	pdfURL := ""
	doc.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !saskatchewanProgressPDFRe.MatchString(href) {
			return true
		}
		pdfURL = resolveRelativeURL(indexURL, href)
		return false
	})
	if pdfURL == "" {
		return nil, nil
	}
	text, err := downloadAndExtractPDFText(pdfURL, "sk", client)
	if err != nil {
		return nil, err
	}
	return parseSaskatchewanBillsFromProgressText(text, pdfURL, legislature, session), nil
}

func parseSaskatchewanBillsFromProgressText(text, sourceURL string, legislature, session int) []ProvincialBillStub {
	normalized := strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	matches := saskatchewanProgressEntryRe.FindAllStringSubmatch(normalized, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]ProvincialBillStub, 0, len(matches))
	for _, match := range matches {
		billNumber := strings.TrimSpace(match[1])
		title := strings.TrimSpace(match[2])
		title = strings.ReplaceAll(title, `\`, " ")
		title = strings.Join(strings.Fields(title), " ")
		id := ProvincialBillID("sk", legislature, session, billNumber)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, ProvincialBillStub{
			ID:           id,
			ProvinceCode: "sk",
			Parliament:   legislature,
			Session:      session,
			Number:       billNumber,
			Title:        title,
			Chamber:      "saskatchewan",
			DetailURL:    sourceURL,
			SourceURL:    sourceURL,
			LastScraped:  utils.NowISO(),
		})
	}
	return out
}

// CrawlSaskatchewanBills crawls Saskatchewan bills pages.
func CrawlSaskatchewanBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlSaskatchewanBills(indexURL, legislature, session, client)
}

// ── Saskatchewan votes ────────────────────────────────────────────────────────

// crawlSaskatchewanMinutesLinks fetches the Saskatchewan legislature archive page
// and returns the list of Assembly Minutes HTML document URLs.
func crawlSaskatchewanMinutesLinks(archiveURL string, client *http.Client) ([]string, error) {
	if archiveURL == "" {
		archiveURL = SaskatchewanArchiveURL
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Infof("[sk-votes] fetching archive: %s", archiveURL)

	doc, err := fetchDoc(archiveURL, client)
	if err != nil {
		return nil, fmt.Errorf("sk archive: %w", err)
	}

	var links []string
	seen := make(map[string]bool)
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		if strings.Contains(href, "legdocs/Assembly/Minutes/") && strings.HasSuffix(href, "Minutes-HTML.htm") {
			if !seen[href] {
				seen[href] = true
				links = append(links, href)
			}
		}
	})

	clog.Debugf("[sk-votes] found %d Assembly Minutes HTML links", len(links))
	return links, nil
}

// CrawlSaskatchewanMinutesLinks is the exported wrapper.
func CrawlSaskatchewanMinutesLinks(archiveURL string, client *http.Client) ([]string, error) {
	return crawlSaskatchewanMinutesLinks(archiveURL, client)
}

// crawlSaskatchewanMinutes scrapes a single Saskatchewan Assembly Minutes HTML document.
// legislature and session are used to build division IDs.
func crawlSaskatchewanMinutes(minutesURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Debugf("[sk-votes] scraping Minutes: %s", minutesURL)

	m := skDateFromURLRe.FindStringSubmatch(minutesURL)
	if len(m) != 2 {
		return nil, fmt.Errorf("sk Minutes: cannot extract date from URL %s", minutesURL)
	}
	raw := m[1] // "20260414"
	date := fmt.Sprintf("%s-%s-%s", raw[:4], raw[4:6], raw[6:8])

	doc, err := fetchDoc(minutesURL, client)
	if err != nil {
		return nil, fmt.Errorf("sk Minutes %s: %w", date, err)
	}

	return parseSaskatchewanMinutesDoc(doc, legislature, session, date), nil
}

// CrawlSaskatchewanMinutes is the exported wrapper.
func CrawlSaskatchewanMinutes(minutesURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlSaskatchewanMinutes(minutesURL, legislature, session, client)
}

func isWarmTanLike(c color.NRGBA) bool {
	r, g, b := int(c.R), int(c.G), int(c.B)
	return r >= 170 && r <= 235 &&
		g >= 150 && g <= 220 &&
		b >= 120 && b <= 200 &&
		r >= g && g >= b &&
		r-b >= 10 && r-b <= 90
}

func isNeutralGreyLike(c color.NRGBA) bool {
	r, g, b := int(c.R), int(c.G), int(c.B)
	maxRGB := r
	if g > maxRGB {
		maxRGB = g
	}
	if b > maxRGB {
		maxRGB = b
	}
	minRGB := r
	if g < minRGB {
		minRGB = g
	}
	if b < minRGB {
		minRGB = b
	}
	return r >= 130 && g >= 130 && b >= 130 && maxRGB-minRGB <= 20
}

// IsSaskatchewanSittingLike returns true for SK sitting-day highlight colors.
func IsSaskatchewanSittingLike(c color.NRGBA) bool {
	// SK highlights appear as grey in some exports and warm tan in others.
	return isNeutralGreyLike(c) || isWarmTanLike(c)
}

// parseSaskatchewanMinutesDoc is the pure HTML-parsing logic for Saskatchewan Minutes.
func parseSaskatchewanMinutesDoc(doc *goquery.Document, legislature, session int, date string) []ProvincialDivisionResult {
	var results []ProvincialDivisionResult
	divNum := 0

	doc.Find("table").Each(func(_ int, t *goquery.Selection) {
		if !strings.Contains(t.Text(), "YEAS") {
			return
		}

		divNum++
		divID := ProvincialDivisionID("sk", legislature, session, divNum, date)

		var votes []ProvincialMemberVote
		yeas, nays := 0, 0

		t.Find("td").Each(func(_ int, cell *goquery.Selection) {
			cellText := cell.Text()

			var voteType string
			if strings.Contains(cellText, "YEAS") {
				voteType = "Yea"
			} else if strings.Contains(cellText, "NAYS") {
				voteType = "Nay"
			}
			if voteType == "" {
				return
			}

			// Extract count from "YEAS / POUR – N"
			if cm := skCountRe.FindStringSubmatch(cellText); len(cm) == 2 {
				n, _ := strconv.Atoi(cm[1])
				if voteType == "Yea" {
					yeas = n
				} else {
					nays = n
				}
			}

			// Extract member names from <p> elements.
			cell.Find("p").Each(func(_ int, p *goquery.Selection) {
				// Prefer text from the first span with lang=EN-GB; fall back to the paragraph text.
				name := ""
				p.Find("span").Each(func(_ int, s *goquery.Selection) {
					if name != "" {
						return
					}
					lang, _ := s.Attr("lang")
					if !strings.EqualFold(lang, "en-gb") {
						return
					}
					// Only the outermost EN-GB span carries the name.
					if s.ParentsFiltered("span[lang]").Length() > 0 {
						return
					}
					name = strings.TrimSpace(s.Text())
				})
				if name == "" {
					name = strings.TrimSpace(p.Text())
				}
				// Normalise whitespace and drop non-breaking spaces.
				name = strings.Join(strings.Fields(strings.ReplaceAll(name, "\u00a0", " ")), " ")
				// Skip the header row and blank entries.
				upper := strings.ToUpper(name)
				if name == "" || strings.Contains(upper, "YEAS") || strings.Contains(upper, "NAYS") ||
					strings.Contains(upper, "POUR") || strings.Contains(upper, "CONTRE") {
					return
				}
				votes = append(votes, ProvincialMemberVote{
					DivisionID: divID,
					MemberName: name,
					Vote:       voteType,
				})
			})
		})

		// Description: English text from the nearest preceding paragraph that
		// mentions a bill number or motion.
		desc := ""
		t.PrevAll().Filter("p").Each(func(_ int, p *goquery.Selection) {
			if desc != "" {
				return
			}
			text := strings.TrimSpace(p.Text())
			text = strings.Join(strings.Fields(strings.ReplaceAll(text, "\u00a0", " ")), " ")
			if text != "" && !strings.Contains(strings.ToLower(text), "recorded division") {
				desc = text
			}
		})

		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID:          divID,
				Parliament:  legislature,
				Session:     session,
				Number:      divNum,
				Date:        date,
				Description: desc,
				Yeas:        yeas,
				Nays:        nays,
				Result:      result,
				Chamber:     "saskatchewan",
				LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
	})

	clog.Debugf("[sk-votes] %s: parsed %d divisions", date, len(results))
	return results
}
