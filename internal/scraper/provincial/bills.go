package provincial

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/open-democracy/internal/urlutil"
	"github.com/philspins/open-democracy/internal/utils"
)

// ProvincialBillStub is a lightweight bill record scraped from a provincial
// legislature listing page.
type ProvincialBillStub struct {
	ID               string
	ProvinceCode     string
	Parliament       int
	Session          int
	Number           string
	Title            string
	Chamber          string
	DetailURL        string
	SourceURL        string
	LastActivityDate string
	LastScraped      string
}

var provincialBillNumberRe = regexp.MustCompile(`(?i)\bbill(?:\s*\(\s*no\.?|\s+no\.?)?\s+([a-z]?(?:[\s-]?\d+)[a-z]?)\s*\)?\b`)
var provincialBillURLNumberRe = regexp.MustCompile(`(?i)(?:/bill-|/bill/|/bills?/)(\d{1,4}[a-z]?)(?:[/?#-]|$)`)
var provincialNestedBillURLNumberRe = regexp.MustCompile(`(?i)/\d{1,3}/\d{1,2}/(\d{1,4}[a-z]?)(?:/|$)`)
var provincialLeadingBillNumberRe = regexp.MustCompile(`(?m)^\s*(\d{1,4}[a-z]?)\s*(?:[.)]|\|)`)
var provincialStandaloneNumberRe = regexp.MustCompile(`(?i)^\s*(\d{1,4}[a-z]?)\s*$`)
var genericBillLinkRe = regexp.MustCompile(`(?i)(bill|legislation|legislative-business|housebusiness|bills-and-legislation|legis)`)

// ExtractProvincialBillNumber extracts a bill number from provincial text.
// Examples: "Bill 12" -> "12", "bill a-23" -> "A-23".
func ExtractProvincialBillNumber(text string) string {
	if m := provincialBillNumberRe.FindStringSubmatch(text); len(m) == 2 {
		billNumber := strings.ToUpper(strings.TrimSpace(m[1]))
		return strings.Join(strings.Fields(billNumber), "-")
	}
	// Fall back to federal bill-number format (C-47 / S-209) when present.
	return utils.ExtractBillNumber(text)
}

// ProvincialBillID builds a deterministic provincial bill ID.
// Format: "{province}-{legislature}-{session}-{bill_number}".
func ProvincialBillID(province string, legislature, session int, billNumber string) string {
	clean := strings.ToLower(strings.TrimSpace(billNumber))
	clean = strings.ReplaceAll(clean, " ", "")
	clean = strings.ReplaceAll(clean, "/", "-")
	clean = strings.ReplaceAll(clean, "–", "-")
	clean = strings.ReplaceAll(clean, "—", "-")
	if clean == "" {
		return ""
	}
	return fmt.Sprintf("%s-%d-%d-%s", province, legislature, session, clean)
}

// CrawlProvincialBillsFromIndex scrapes a provincial legislative-business page
// and returns bill stubs discovered from links containing bill numbers.
func CrawlProvincialBillsFromIndex(indexURL, provinceCode string, legislature, session int, chamber string, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlProvincialBillsFromIndexWithMatcher(indexURL, provinceCode, legislature, session, chamber, client, genericBillLinkRe)
}

func crawlProvincialBillsFromIndexWithMatcher(indexURL, provinceCode string, legislature, session int, chamber string, client *http.Client, linkMatcher *regexp.Regexp) ([]ProvincialBillStub, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	doc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, fmt.Errorf("provincial bills index: %w", err)
	}
	return parseProvincialBillsIndexDoc(doc, indexURL, provinceCode, legislature, session, chamber, linkMatcher), nil
}

func parseProvincialBillsIndexDoc(doc *goquery.Document, indexURL, provinceCode string, legislature, session int, chamber string, linkMatcher *regexp.Regexp) []ProvincialBillStub {
	if linkMatcher == nil {
		linkMatcher = genericBillLinkRe
	}
	seen := make(map[string]bool)
	out := make([]ProvincialBillStub, 0)

	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		text := strings.TrimSpace(strings.Join(strings.Fields(a.Text()), " "))
		contextText := billContextText(a)
		if href == "" {
			return
		}
		if !linkMatcher.MatchString(text + " " + href) {
			return
		}

		billNumber := extractProvincialBillNumberWithContext(text, href, contextText)
		if billNumber == "" {
			return
		}

		id := ProvincialBillID(provinceCode, legislature, session, billNumber)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true

		detailURL := resolveRelativeURL(indexURL, href)
		title := billTitleFromContext(text, contextText, billNumber)
		if title == "" {
			title = "Bill " + billNumber
		}

		// Try to infer a date from the closest row/card text.
		lastActivity := utils.FindDateInText(contextText)

		out = append(out, ProvincialBillStub{
			ID:               id,
			ProvinceCode:     provinceCode,
			Parliament:       legislature,
			Session:          session,
			Number:           billNumber,
			Title:            title,
			Chamber:          chamber,
			DetailURL:        detailURL,
			SourceURL:        indexURL,
			LastActivityDate: lastActivity,
			LastScraped:      utils.NowISO(),
		})
	})

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func billContextText(a *goquery.Selection) string {
	return strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(
		a.Closest("tr, li, article, section, div, table").Text(), "\u00a0", " ")), " "))
}

func extractProvincialBillNumberWithContext(text, href, contextText string) string {
	for _, candidate := range []string{text + " " + href, contextText, href} {
		if billNumber := ExtractProvincialBillNumber(candidate); billNumber != "" {
			return billNumber
		}
	}
	if m := provincialNestedBillURLNumberRe.FindStringSubmatch(href); len(m) == 2 {
		return strings.ToUpper(strings.TrimSpace(m[1]))
	}
	if m := provincialBillURLNumberRe.FindStringSubmatch(href); len(m) == 2 {
		return strings.ToUpper(strings.TrimSpace(m[1]))
	}
	if m := provincialLeadingBillNumberRe.FindStringSubmatch(contextText); len(m) == 2 {
		return strings.ToUpper(strings.TrimSpace(m[1]))
	}
	return ""
}

func billTitleFromContext(anchorText, contextText, billNumber string) string {
	title := strings.TrimSpace(anchorText)
	if title != "" && !provincialStandaloneNumberRe.MatchString(title) {
		return title
	}
	contextText = strings.TrimSpace(contextText)
	if contextText == "" {
		return ""
	}
	if billNumber != "" {
		prefixPatterns := []string{
			"Bill " + billNumber,
			"Bill No. " + billNumber,
			billNumber + ".",
			billNumber + " |",
		}
		for _, prefix := range prefixPatterns {
			if strings.HasPrefix(contextText, prefix) {
				trimmed := strings.TrimSpace(strings.TrimPrefix(contextText, prefix))
				if trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return contextText
}

// parseStructuredProvincialBillRows parses bills from structured HTML table rows.
// Used by NL and BC bill crawlers.
func parseStructuredProvincialBillRows(doc *goquery.Document, sourceURL, provinceCode string, legislature, session int, chamber string) []ProvincialBillStub {
	seen := make(map[string]bool)
	out := make([]ProvincialBillStub, 0)
	doc.Find("table tr").Each(func(_ int, tr *goquery.Selection) {
		cells := tr.Find("th, td")
		if cells.Length() < 2 {
			return
		}
		numberText := strings.TrimSpace(strings.Join(strings.Fields(cells.First().Text()), " "))
		billNumber := extractProvincialBillNumberWithContext(numberText, tr.Find("a[href]").First().AttrOr("href", ""), strings.TrimSpace(strings.Join(strings.Fields(tr.Text()), " ")))
		if billNumber == "" {
			return
		}
		id := ProvincialBillID(provinceCode, legislature, session, billNumber)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		titleCell := cells.Eq(1)
		title := strings.TrimSpace(strings.Join(strings.Fields(titleCell.Text()), " "))
		if title == "" {
			title = "Bill " + billNumber
		}
		detailURL := sourceURL
		if link := titleCell.Find("a[href]").First(); link.Length() > 0 {
			detailURL = resolveRelativeURL(sourceURL, link.AttrOr("href", ""))
		}
		lastActivity := utils.FindDateInText(strings.TrimSpace(strings.Join(strings.Fields(tr.Text()), " ")))
		out = append(out, ProvincialBillStub{
			ID:               id,
			ProvinceCode:     provinceCode,
			Parliament:       legislature,
			Session:          session,
			Number:           billNumber,
			Title:            title,
			Chamber:          chamber,
			DetailURL:        detailURL,
			SourceURL:        sourceURL,
			LastActivityDate: lastActivity,
			LastScraped:      utils.NowISO(),
		})
	})
	return out
}

// ordinalSuffix converts an integer to its ordinal string (1→"1st", 2→"2nd", etc.).
// Used by BC and MB bill crawlers.
func ordinalSuffix(n int) string {
	if n%100 >= 11 && n%100 <= 13 {
		return strings.Join([]string{fmt.Sprintf("%d", n), "th"}, "")
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	default:
		return fmt.Sprintf("%dth", n)
	}
}

func resolveRelativeURL(baseURL, href string) string {
	return urlutil.ResolveRelativeURL(baseURL, href)
}
