package provincial

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
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

// MBMonthFromGrid maps Manitoba calendar row/column into month number.
func MBMonthFromGrid(row, col int) int {
	if col < 0 || col > 1 || row < 0 || row > 3 {
		return 0
	}
	grid := [4][2]int{
		{3, 4},
		{5, 6},
		{9, 10},
		{11, 12},
	}
	return grid[row][col]
}

func isLightGreyLike(c color.NRGBA) bool {
	r, g, b := int(c.R), int(c.G), int(c.B)
	if r > 248 && g > 248 && b > 248 {
		return false
	}
	if r < 130 || g < 130 || b < 130 {
		return false
	}
	rg := r - g
	if rg < 0 {
		rg = -rg
	}
	gb := g - b
	if gb < 0 {
		gb = -gb
	}
	rb := r - b
	if rb < 0 {
		rb = -rb
	}
	return rg <= 40 && gb <= 40 && rb <= 40
}

// ParseMBHighlightedSittingDatesFromPDF extracts Manitoba calendar dates from highlighted PDF cells.
func ParseMBHighlightedSittingDatesFromPDF(pdfBytes []byte, year int) ([]string, bool) {
	if dates, ok := parseMBHighlightedSittingDatesFromPDFBBox(pdfBytes, year); ok {
		return dates, true
	}
	if dates, ok := parseMBHighlightedSittingDatesFromPDFOCR(pdfBytes, year); ok {
		return dates, true
	}
	return parseMBHeuristicDatesFromPDFText(pdfBytes, year)
}

func parseMBHeuristicDatesFromPDFText(pdfBytes []byte, year int) ([]string, bool) {
	text, err := extractTextWithPDFToText(pdfBytes)
	if err != nil {
		return nil, false
	}
	norm := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if !strings.Contains(norm, "sessional") || !strings.Contains(norm, strconv.Itoa(year)) {
		return nil, false
	}

	months := []time.Month{
		time.March, time.April, time.May, time.June,
		time.September, time.October, time.November, time.December,
	}
	seen := map[string]struct{}{}
	var out []string
	for _, m := range months {
		start := time.Date(year, m, 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, -1)
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			if d.Weekday() < time.Tuesday || d.Weekday() > time.Thursday {
				continue
			}
			iso := d.Format("2006-01-02")
			if _, ok := seen[iso]; ok {
				continue
			}
			seen[iso] = struct{}{}
			out = append(out, iso)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func parseMBHighlightedSittingDatesFromPDFBBox(pdfBytes []byte, year int) ([]string, bool) {
	img, ok := renderCalendarPageImage(pdfBytes)
	if !ok {
		return nil, false
	}

	words, ok := extractPDFBBoxWordsAsOCRWords(pdfBytes, img.Bounds())
	if !ok {
		return nil, false
	}

	headings := extractMonthHeadings(words, englishMonthNames, img.Bounds().Max.Y)
	if len(headings) == 0 {
		return nil, false
	}

	seen := map[string]struct{}{}
	var out []string
	bounds := img.Bounds()
	for _, w := range words {
		n, err := strconv.Atoi(strings.TrimSpace(w.Text))
		if err != nil || n < 1 || n > 31 {
			continue
		}
		dx := float64(w.Left + w.Width/2)
		dy := float64(w.Top + w.Height/2)

		bestM := 0
		bestDist := math.MaxFloat64
		for _, h := range headings {
			dxw := (dx - h.cx) * 1.8
			dyW := (dy - h.cy)
			dist := math.Hypot(dxw, dyW)
			if dist < bestDist {
				bestDist = dist
				bestM = h.month
			}
		}
		if bestM == 0 {
			continue
		}

		date := time.Date(year, time.Month(bestM), n, 0, 0, 0, 0, time.UTC)
		if date.Month() != time.Month(bestM) {
			continue
		}

		cell := image.Rect(w.Left-8, w.Top-8, w.Left+w.Width+8, w.Top+w.Height+8).Intersect(bounds)
		if cell.Empty() {
			continue
		}
		total, grey := 0, 0
		for y := cell.Min.Y; y < cell.Max.Y; y++ {
			for x := cell.Min.X; x < cell.Max.X; x++ {
				r16, g16, b16, _ := img.At(x, y).RGBA()
				c := color.NRGBA{R: uint8(r16 >> 8), G: uint8(g16 >> 8), B: uint8(b16 >> 8), A: 255}
				total++
				if isLightGreyLike(c) {
					grey++
				}
			}
		}
		if total == 0 || float64(grey)/float64(total) < 0.01 {
			continue
		}

		iso := date.Format("2006-01-02")
		if _, exists := seen[iso]; exists {
			continue
		}
		seen[iso] = struct{}{}
		out = append(out, iso)
	}

	sort.Strings(out)
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func parseMBHighlightedSittingDatesFromPDFOCR(pdfBytes []byte, year int) ([]string, bool) {
	if !hasCommand("pdftoppm") || !hasCommand("tesseract") {
		return nil, false
	}

	img, words, ok := renderAndOCRCalendarPage(pdfBytes)
	if !ok {
		return nil, false
	}

	bounds := img.Bounds()
	dayWords := make([]ocrWord, 0, len(words))
	xCenters := make([]float64, 0, len(words))
	yCenters := make([]float64, 0, len(words))
	for _, w := range words {
		if w.Confidence < 25 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(w.Text))
		if err != nil || n < 1 || n > 31 {
			continue
		}
		w.ParsedNumber = n
		dayWords = append(dayWords, w)
		xCenters = append(xCenters, float64(w.Left+w.Width/2))
		yCenters = append(yCenters, float64(w.Top+w.Height/2))
	}
	if len(dayWords) < 40 {
		return nil, false
	}

	colCenters, ok := cluster1D(xCenters, 2)
	if !ok {
		return nil, false
	}
	rowCenters, ok := cluster1D(yCenters, 4)
	if !ok {
		return nil, false
	}
	sort.Float64s(colCenters)
	sort.Float64s(rowCenters)

	seen := map[string]struct{}{}
	var out []string
	for _, w := range dayWords {
		cx := float64(w.Left + w.Width/2)
		cy := float64(w.Top + w.Height/2)
		col := nearestClusterIndex(cx, colCenters)
		row := nearestClusterIndex(cy, rowCenters)
		month := MBMonthFromGrid(row, col)
		if month == 0 {
			continue
		}

		date := time.Date(year, time.Month(month), w.ParsedNumber, 0, 0, 0, 0, time.UTC)
		if date.Month() != time.Month(month) {
			continue
		}

		cell := image.Rect(w.Left-8, w.Top-8, w.Left+w.Width+8, w.Top+w.Height+8).Intersect(bounds)
		if cell.Empty() {
			continue
		}
		total, grey := 0, 0
		for y := cell.Min.Y; y < cell.Max.Y; y++ {
			for x := cell.Min.X; x < cell.Max.X; x++ {
				r16, g16, b16, _ := img.At(x, y).RGBA()
				c := color.NRGBA{R: uint8(r16 >> 8), G: uint8(g16 >> 8), B: uint8(b16 >> 8), A: 255}
				total++
				if isLightGreyLike(c) {
					grey++
				}
			}
		}
		if total == 0 || float64(grey)/float64(total) < 0.03 {
			continue
		}

		iso := date.Format("2006-01-02")
		if _, exists := seen[iso]; exists {
			continue
		}
		seen[iso] = struct{}{}
		out = append(out, iso)
	}

	sort.Strings(out)
	if len(out) == 0 {
		return nil, false
	}
	return out, true
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
func crawlManitobaVotesFromPDF(indexURL string, legislature, session int, client *http.Client, allSittings bool) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Debugf("[mb-votes] fetching index: %s", indexURL)
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
		clog.Infof("[mb-votes] no session pages discovered; falling back to generic parser")
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
			clog.Infof("[mb-votes] skip session %s: %v", sessURL, serr)
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
	if !allSittings && len(pdfLinks) > 80 {
		pdfLinks = pdfLinks[len(pdfLinks)-80:]
	}
	if len(pdfLinks) == 0 {
		clog.Infof("[mb-votes] no VP PDFs discovered; falling back to generic parser")
		return crawlGenericProvincialVotesWithMatcher(indexURL, "mb", "manitoba", legislature, session, client, manitobaVotesLinkRe)
	}

	var results []ProvincialDivisionResult
	nextDivNum := 1
	for _, pdfURL := range pdfLinks {
		text, terr := downloadAndExtractPDFText(pdfURL, "mb", client)
		if terr != nil {
			if errors.Is(terr, errNonPDFResponse) {
				clog.Debugf("[mb-votes] skip non-pdf link %s: %v", pdfURL, terr)
				continue
			}
			clog.Infof("[mb-votes] skip pdf %s: %v", pdfURL, terr)
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
	clog.Debugf("[mb-votes] parsed %d divisions from %d PDFs", len(results), len(pdfLinks))
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
	// The bill motion may be further back when a full AYE/NAY block from the
	// previous division falls between the THAT clause and this division's AYE.
	wideStart := markerStart - 3000
	if wideStart < 0 {
		wideStart = 0
	}
	if wideStart < start {
		wideCtx := strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(text[wideStart:markerStart], "\u00a0", " ")), " "))
		if matches := mbMotionDescriptionRe.FindAllStringSubmatch(wideCtx, -1); len(matches) > 0 {
			return strings.TrimSpace(matches[len(matches)-1][1])
		}
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
func crawlManitobaVotes(indexURL string, legislature, session int, client *http.Client, allSittings bool) ([]ProvincialDivisionResult, error) {
	if indexURL == "" {
		indexURL = "https://www.gov.mb.ca/legislature/business/votes_proceedings.html"
	}
	return crawlManitobaVotesFromPDF(indexURL, legislature, session, client, allSittings)
}

// CrawlManitobaVotes crawls Manitoba votes/proceedings pages.
func CrawlManitobaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlManitobaVotes(indexURL, legislature, session, client, false)
}
