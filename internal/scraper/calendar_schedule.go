package scraper

import (
	"database/sql"
	"fmt"
	"html"
	"image"
	"image/color"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/scraper/provincial"
	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/utils"
)

var legislatureCalendarSources = map[string]string{
	"federal-commons": "https://www.ourcommons.ca/en/sitting-calendar",
	"federal-senate":  "https://sencanada.ca/en/calendar/",
	"provincial-AB":   "https://www.assembly.ab.ca/assembly-business/assembly-dashboard",
	"provincial-BC":   "https://www.leg.bc.ca/parliamentary-business/parliamentary-calendar",
	"provincial-MB":   "https://www.gov.mb.ca/legislature/business/sessional_calendar.pdf",
	"provincial-NB":   "https://www.legnb.ca/en/calendar",
	"provincial-NL":   "https://www.assembly.nl.ca/HouseBusiness/ParliamentaryCalendar.aspx",
	"provincial-NS":   "https://nslegislature.ca/get-involved/calendar",
	"provincial-ON":   "https://www.ola.org/en/legislative-business/parliamentary-calendars",
	"provincial-PE":   "https://www.assembly.pe.ca/sites/www.assembly.pe.ca/files/parliamentary%20calendar.2026.pdf",
	"provincial-QC":   "https://www.assnat.qc.ca/en/document/211091.html",
	"provincial-SK":   "https://www.legassembly.sk.ca/media/bdcjdifz/30l3s-calendar.pdf",
}

func CrawlAndPersistLegislatureCalendars(conn *sql.DB, client *http.Client) error {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	jurisdictions := make([]string, 0, len(legislatureCalendarSources))
	for k := range legislatureCalendarSources {
		jurisdictions = append(jurisdictions, k)
	}
	sort.Strings(jurisdictions)

	scrapedAt := utils.NowISO()
	var firstErr error
	for _, jurisdiction := range jurisdictions {
		url := legislatureCalendarSources[jurisdiction]
		dates, err := crawlLegislatureCalendarDates(client, jurisdiction, url)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		clog.Debugf("[calendar] detected %d dates for %s", len(dates), jurisdiction)
		if len(dates) == 0 {
			continue
		}
		if err := store.ReplaceLegislatureCalendarDates(conn, jurisdiction, dates, scrapedAt); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func crawlLegislatureCalendarDates(client *http.Client, jurisdiction, url string) ([]string, error) {
	year := time.Now().UTC().Year()

	// assnat.qc.ca omits its intermediate CA from the TLS handshake; use the
	// QC-specific client that embeds it.
	if jurisdiction == "provincial-QC" {
		timeout := 15 * time.Second
		if client != nil && client.Timeout > 0 {
			timeout = client.Timeout
		}
		client = provincial.NewQCHTTPClient(timeout)
	}

	// Jurisdictions that need custom fetching (multiple pages or dynamically constructed URLs).
	switch jurisdiction {
	case "provincial-NS":
		dates, ok := provincial.NovaScotiaCalendarDates(client, year)
		if !ok {
			return nil, fmt.Errorf("nova scotia calendar returned no dates")
		}
		return dates, nil
	case "provincial-NB":
		dates, ok := provincial.NewBrunswickCalendarDates(client, year)
		if !ok {
			return nil, fmt.Errorf("new brunswick calendar returned no dates")
		}
		return dates, nil
	case "provincial-NL":
		nlPDFURL := fmt.Sprintf("https://www.assembly.nl.ca/pdfs/ParliamentaryCalendar%d.pdf", year)
		pdfBytes, err := provincial.FetchCalendarPDFBytes(client, nlPDFURL)
		if err != nil {
			return nil, fmt.Errorf("NL PDF fetch: %w", err)
		}
		dates, ok := parsePDFHighlightedCalendarDates(pdfBytes, year, isGreenLike, englishMonthNames, 1.0, 0.08)
		if !ok {
			return nil, fmt.Errorf("NL calendar returned no dates")
		}
		return dates, nil
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenDemocracyCrawler/1.0; +https://github.com/philspins/opendocket)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-CA,en;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	if jurisdiction == "provincial-PE" && strings.HasSuffix(strings.ToLower(url), ".pdf") {
		if dates, ok := provincial.PEICalendarDatesFromPDFBytes(body, year); ok {
			return dates, nil
		}
		if !hasCommand("tesseract") {
			return nil, fmt.Errorf("pei pdf parsing requires tesseract (PEI calendar PDF is image-only)")
		}
		return nil, fmt.Errorf("pei pdf parsing returned no dates")
	}

	// PDF-based jurisdictions: body is raw PDF bytes.
	switch jurisdiction {
	case "provincial-QC":
		dates, ok := provincial.QuebecCalendarDatesFromPDF(body, year)
		if !ok {
			return nil, fmt.Errorf("QC calendar PDF parsing returned no dates")
		}
		return dates, nil
	case "provincial-MB":
		dates, ok := provincial.ParseMBHighlightedSittingDatesFromPDF(body, year)
		if !ok {
			return nil, fmt.Errorf("MB calendar PDF parsing returned no dates")
		}
		return dates, nil
	case "provincial-SK":
		dates, ok := parsePDFHighlightedCalendarDates(body, year, provincial.IsSaskatchewanSittingLike, englishMonthNames, 0.9, 0.02)
		if !ok {
			return nil, fmt.Errorf("SK calendar PDF parsing returned no dates")
		}
		return dates, nil
	}

	text := string(body)

	if jurisdiction == "provincial-ON" {
		if dates, ok := provincial.OntarioCalendarDates(text, year); ok {
			return dates, nil
		}
	}
	if jurisdiction == "federal-senate" {
		if dates, ok := senateCalendarDates(client, text, year); ok {
			return dates, nil
		}
	}
	if jurisdiction == "provincial-PE" {
		if dates, ok := provincial.PEICalendarDates(client, text, year); ok {
			return dates, nil
		}
	}
	return extractCalendarDatesFromText(text), nil
}

var (
	isoDateRe      = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	englishDateRe  = regexp.MustCompile(`\b(?:January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},?\s+\d{4}\b`)
	englishDateRe2 = regexp.MustCompile(`\b\d{1,2}\s+(?:January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{4}\b`)
	stripHTMLTagRe = regexp.MustCompile(`<[^>]+>`)

	senatePDFHrefRe = regexp.MustCompile(`(?i)href=["']([^"']+\.pdf)["']`)
)

func senateCalendarDates(client *http.Client, pageHTML string, year int) ([]string, bool) {
	pdfURL := extractSenateCalendarPDFURL(pageHTML, year)
	if pdfURL == "" {
		return nil, false
	}
	pdfBytes, err := provincial.FetchCalendarPDFBytes(client, pdfURL)
	if err != nil {
		return nil, false
	}
	return parsePDFHighlightedCalendarDates(pdfBytes, year, isSenateOpenDayLike, englishMonthNames, 1.0, 0.02)
}

func extractSenateCalendarPDFURL(pageHTML string, year int) string {
	matches := senatePDFHrefRe.FindAllStringSubmatch(pageHTML, -1)
	if len(matches) == 0 {
		return ""
	}
	base, _ := url.Parse("https://sencanada.ca/en/calendar/")
	yearToken := strconv.Itoa(year)
	best := ""
	for _, m := range matches {
		if len(m) != 2 {
			continue
		}
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		resolved := u.String()
		if !u.IsAbs() && base != nil {
			resolved = base.ResolveReference(u).String()
		}
		lower := strings.ToLower(resolved)
		if !strings.Contains(lower, "senate") || !strings.Contains(lower, "calendar") {
			continue
		}
		if strings.Contains(lower, yearToken) {
			return resolved
		}
		if best == "" {
			best = resolved
		}
	}
	return best
}

func isSenateOpenDayLike(c color.NRGBA) bool {
	r, g, b := int(c.R), int(c.G), int(c.B)
	// Red open days.
	isRed := r >= 145 && g <= 165 && b <= 165 && r-g >= 20 && r-b >= 20
	// Pink open days.
	isPink := r >= 170 && g >= 95 && g <= 210 && b >= 95 && b <= 220 && r-g >= 8 && r-b >= 8
	return isRed || isPink
}

func extractCalendarDatesFromText(body string) []string {
	seen := map[string]struct{}{}
	now := dayStartUTC(time.Now().UTC())
	minDate := now.AddDate(-1, 0, 0)
	maxDate := now.AddDate(1, 0, 0)
	addIfInRange := func(raw string) {
		d := utils.ParseDate(raw)
		if d == "" {
			return
		}
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			return
		}
		parsed = dayStartUTC(parsed)
		if parsed.Before(minDate) || parsed.After(maxDate) {
			return
		}
		seen[d] = struct{}{}
	}
	for _, m := range isoDateRe.FindAllString(body, -1) {
		addIfInRange(m)
	}
	for _, m := range englishDateRe.FindAllString(body, -1) {
		addIfInRange(m)
	}
	for _, m := range englishDateRe2.FindAllString(body, -1) {
		addIfInRange(m)
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func dayStartUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func extractTextWithPDFToText(pdfBytes []byte) (string, error) {
	if !hasCommand("pdftotext") {
		return "", fmt.Errorf("pdftotext not installed")
	}
	tmpIn, err := os.CreateTemp("", "pei-calendar-*.pdf")
	if err != nil {
		return "", err
	}
	inPath := tmpIn.Name()
	defer os.Remove(inPath)
	if _, err := tmpIn.Write(pdfBytes); err != nil {
		tmpIn.Close()
		return "", err
	}
	if err := tmpIn.Close(); err != nil {
		return "", err
	}

	cmd := exec.Command("pdftotext", "-layout", inPath, "-")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

type ocrWord struct {
	Text         string
	Left         int
	Top          int
	Width        int
	Height       int
	Confidence   float64
	ParsedNumber int
}

func renderAndOCRCalendarPage(pdfBytes []byte) (image.Image, []ocrWord, bool) {
	tmpPDF, err := os.CreateTemp("", "pei-calendar-highlight-*.pdf")
	if err != nil {
		return nil, nil, false
	}
	pdfPath := tmpPDF.Name()
	defer os.Remove(pdfPath)
	if _, err := tmpPDF.Write(pdfBytes); err != nil {
		tmpPDF.Close()
		return nil, nil, false
	}
	if err := tmpPDF.Close(); err != nil {
		return nil, nil, false
	}

	imgPrefix := pdfPath + "-page"
	cmdRaster := exec.Command("pdftoppm", "-f", "1", "-l", "1", "-r", "220", "-png", pdfPath, imgPrefix)
	if err := cmdRaster.Run(); err != nil {
		return nil, nil, false
	}
	imgPath := imgPrefix + "-1.png"
	defer os.Remove(imgPath)

	f, err := os.Open(imgPath)
	if err != nil {
		return nil, nil, false
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return nil, nil, false
	}

	tsvTmp, err := os.CreateTemp("", "calendar-ocr-*.tsv")
	if err != nil {
		return nil, nil, false
	}
	tsvTmp.Close()
	// os.CreateTemp creates file with .tsv extension; tesseract appends its own extension,
	// so strip it to get the base, then expect tesseract to write <base>.tsv.
	tsvBase := strings.TrimSuffix(tsvTmp.Name(), ".tsv")
	tsvPath := tsvBase + ".tsv"
	os.Remove(tsvTmp.Name())
	defer os.Remove(tsvPath)
	if err := exec.Command("tesseract", imgPath, tsvBase, "-l", "eng", "tsv").Run(); err != nil {
		return nil, nil, false
	}
	tsvBytes, err := os.ReadFile(tsvPath)
	if err != nil {
		return nil, nil, false
	}
	words := parseTesseractTSVWords(string(tsvBytes))
	if len(words) == 0 {
		return nil, nil, false
	}
	return img, words, true
}

func parseTesseractTSVWords(tsv string) []ocrWord {
	lines := strings.Split(tsv, "\n")
	if len(lines) <= 1 {
		return nil
	}
	out := make([]ocrWord, 0, len(lines)-1)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 12 {
			continue
		}
		text := strings.TrimSpace(cols[11])
		if text == "" {
			continue
		}
		left, err1 := strconv.Atoi(cols[6])
		top, err2 := strconv.Atoi(cols[7])
		width, err3 := strconv.Atoi(cols[8])
		height, err4 := strconv.Atoi(cols[9])
		conf, err5 := strconv.ParseFloat(cols[10], 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			continue
		}
		out = append(out, ocrWord{Text: text, Left: left, Top: top, Width: width, Height: height, Confidence: conf})
	}
	return out
}

func cluster1D(values []float64, k int) ([]float64, bool) {
	if len(values) < k || k <= 0 {
		return nil, false
	}
	copyVals := append([]float64(nil), values...)
	sort.Float64s(copyVals)
	centers := make([]float64, k)
	for i := 0; i < k; i++ {
		idx := int(math.Round(float64(i*(len(copyVals)-1)) / float64(k-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(copyVals) {
			idx = len(copyVals) - 1
		}
		centers[i] = copyVals[idx]
	}

	assignments := make([]int, len(values))
	for iter := 0; iter < 20; iter++ {
		changed := false
		for i, v := range values {
			best := nearestClusterIndex(v, centers)
			if assignments[i] != best {
				assignments[i] = best
				changed = true
			}
		}
		sums := make([]float64, k)
		counts := make([]int, k)
		for i, v := range values {
			c := assignments[i]
			sums[c] += v
			counts[c]++
		}
		for i := 0; i < k; i++ {
			if counts[i] == 0 {
				return nil, false
			}
			centers[i] = sums[i] / float64(counts[i])
		}
		if !changed {
			break
		}
	}
	return centers, true
}

func nearestClusterIndex(value float64, centers []float64) int {
	best := 0
	bestDist := math.Abs(value - centers[0])
	for i := 1; i < len(centers); i++ {
		d := math.Abs(value - centers[i])
		if d < bestDist {
			best = i
			bestDist = d
		}
	}
	return best
}

func classifyCalendarCellColors(img image.Image, rect image.Rectangle) (green bool, violet bool) {
	total := 0
	greenCount := 0
	violetCount := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r16, g16, b16, _ := img.At(x, y).RGBA()
			c := color.NRGBA{R: uint8(r16 >> 8), G: uint8(g16 >> 8), B: uint8(b16 >> 8), A: 255}
			total++
			if isGreenLike(c) {
				greenCount++
			}
			if isVioletLike(c) {
				violetCount++
			}
		}
	}
	if total == 0 {
		return false, false
	}
	green = float64(greenCount)/float64(total) >= 0.08
	violet = violetCount >= 8 && float64(violetCount)/float64(total) >= 0.01
	return green, violet
}

func isGreenLike(c color.NRGBA) bool {
	return c.G >= 95 && int(c.G)-int(c.R) >= 18 && int(c.G)-int(c.B) >= 12
}

func isVioletLike(c color.NRGBA) bool {
	if c.R < 90 || c.B < 90 {
		return false
	}
	if c.G > 130 {
		return false
	}
	deltaRB := int(c.R) - int(c.B)
	if deltaRB < 0 {
		deltaRB = -deltaRB
	}
	return deltaRB <= 70
}

// ----------------------------------------------------------------------------
// Nova Scotia — HTML calendar, session days have a /calendar/agenda/ href link
// ----------------------------------------------------------------------------

// ----------------------------------------------------------------------------
// Generic PDF image-based highlight parser (NL, MB, SK)
// Renders page 1 of the PDF, OCRs it, finds month headings by text matching,
// then for each day-number cell checks whether the background matches colorFn.
// maxYFraction (0 < f <= 1) limits the y-range considered (useful for SK where
// the fall and spring sections are on the same page).
// ----------------------------------------------------------------------------

var englishMonthNames = map[string]int{
	"january": 1, "february": 2, "march": 3, "april": 4,
	"may": 5, "june": 6, "july": 7, "august": 8,
	"september": 9, "october": 10, "november": 11, "december": 12,
}

type monthPos struct {
	month  int
	cx, cy float64
}

func parsePDFHighlightedCalendarDates(pdfBytes []byte, year int, colorFn func(color.NRGBA) bool, monthNames map[string]int, maxYFraction float64, minColorFraction float64) ([]string, bool) {
	if !hasCommand("pdftoppm") {
		return nil, false
	}
	if minColorFraction <= 0 {
		minColorFraction = 0.08
	}

	img, ok := renderCalendarPageImage(pdfBytes)
	if !ok {
		return nil, false
	}

	// Prefer deterministic PDF text bboxes; fallback to OCR words if needed.
	words, ok := extractPDFBBoxWordsAsOCRWords(pdfBytes, img.Bounds())
	if !ok {
		if !hasCommand("tesseract") {
			return nil, false
		}
		imgOCR, ocrWords, okOCR := renderAndOCRCalendarPage(pdfBytes)
		if !okOCR {
			return nil, false
		}
		img = imgOCR
		words = ocrWords
	}

	bounds := img.Bounds()
	maxCalendarY := bounds.Max.Y
	if maxYFraction > 0 && maxYFraction < 1.0 {
		maxCalendarY = bounds.Min.Y + int(float64(bounds.Dy())*maxYFraction)
	}

	headings := extractMonthHeadings(words, monthNames, maxCalendarY)
	if len(headings) == 0 {
		return nil, false
	}

	seen := map[string]struct{}{}
	var out []string

	for _, w := range words {
		if w.Confidence > 0 && w.Confidence < 30 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(w.Text))
		if err != nil || n < 1 || n > 31 {
			continue
		}
		cy := w.Top + w.Height/2
		if cy > maxCalendarY {
			continue
		}
		dx := float64(w.Left + w.Width/2)
		dy := float64(cy)

		// Nearest month heading; avoid strict vertical assumptions as sources
		// differ in coordinate extraction (OCR vs PDF bboxes).
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
			continue // overflow (e.g. Feb 30)
		}

		cell := image.Rect(w.Left-8, w.Top-8, w.Left+w.Width+8, w.Top+w.Height+8).Intersect(bounds)
		if cell.Empty() {
			continue
		}
		total, matchCount := 0, 0
		for y := cell.Min.Y; y < cell.Max.Y; y++ {
			for x := cell.Min.X; x < cell.Max.X; x++ {
				r16, g16, b16, _ := img.At(x, y).RGBA()
				c := color.NRGBA{R: uint8(r16 >> 8), G: uint8(g16 >> 8), B: uint8(b16 >> 8), A: 255}
				total++
				if colorFn(c) {
					matchCount++
				}
			}
		}
		if total == 0 || float64(matchCount)/float64(total) < minColorFraction {
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

func extractMonthHeadings(words []ocrWord, monthNames map[string]int, maxY int) []monthPos {
	var headings []monthPos
	for _, w := range words {
		cy := w.Top + w.Height/2
		if maxY > 0 && cy > maxY {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(w.Text))
		if len(text) < 3 {
			continue
		}
		for name, m := range monthNames {
			if text == name || strings.HasPrefix(name, text) {
				headings = append(headings, monthPos{
					month: m,
					cx:    float64(w.Left + w.Width/2),
					cy:    float64(w.Top + w.Height/2),
				})
				break
			}
		}
	}
	return headings
}

func renderCalendarPageImage(pdfBytes []byte) (image.Image, bool) {
	if !hasCommand("pdftoppm") {
		return nil, false
	}
	tmpPDF, err := os.CreateTemp("", "calendar-page-*.pdf")
	if err != nil {
		return nil, false
	}
	pdfPath := tmpPDF.Name()
	defer os.Remove(pdfPath)
	if _, err := tmpPDF.Write(pdfBytes); err != nil {
		tmpPDF.Close()
		return nil, false
	}
	if err := tmpPDF.Close(); err != nil {
		return nil, false
	}

	imgPrefix := pdfPath + "-page"
	cmdRaster := exec.Command("pdftoppm", "-f", "1", "-l", "1", "-r", "220", "-png", pdfPath, imgPrefix)
	if err := cmdRaster.Run(); err != nil {
		return nil, false
	}
	imgPath := imgPrefix + "-1.png"
	defer os.Remove(imgPath)

	f, err := os.Open(imgPath)
	if err != nil {
		return nil, false
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return nil, false
	}
	return img, true
}

func extractPDFBBoxWordsAsOCRWords(pdfBytes []byte, imgBounds image.Rectangle) ([]ocrWord, bool) {
	if !hasCommand("pdftotext") && !hasCommand("pdftohtml") {
		return nil, false
	}
	tmpPDF, err := os.CreateTemp("", "calendar-bbox-*.pdf")
	if err != nil {
		return nil, false
	}
	pdfPath := tmpPDF.Name()
	defer os.Remove(pdfPath)
	if _, err := tmpPDF.Write(pdfBytes); err != nil {
		tmpPDF.Close()
		return nil, false
	}
	if err := tmpPDF.Close(); err != nil {
		return nil, false
	}

	if hasCommand("pdftotext") {
		cmd := exec.Command("pdftotext", "-bbox-layout", "-f", "1", "-l", "1", pdfPath, "-")
		out, err := cmd.Output()
		if err == nil || len(strings.TrimSpace(string(out))) > 0 {
			if words, ok := parsePDFToTextBBoxWords(string(out), imgBounds); ok {
				return words, true
			}
		}
	}

	if hasCommand("pdftohtml") {
		if words, ok := extractPDFWordsWithPDFToHTML(pdfPath, imgBounds); ok {
			return words, true
		}
	}

	return nil, false
}

func parsePDFToTextBBoxWords(s string, imgBounds image.Rectangle) ([]ocrWord, bool) {

	pageRe := regexp.MustCompile(`<page[^>]*width="([0-9.]+)"[^>]*height="([0-9.]+)"`)
	pm := pageRe.FindStringSubmatch(s)
	if len(pm) != 3 {
		return nil, false
	}
	pageW, err1 := strconv.ParseFloat(pm[1], 64)
	pageH, err2 := strconv.ParseFloat(pm[2], 64)
	if err1 != nil || err2 != nil || pageW <= 0 || pageH <= 0 {
		return nil, false
	}

	wordRe := regexp.MustCompile(`<word[^>]*xMin="([0-9.]+)"[^>]*yMin="([0-9.]+)"[^>]*xMax="([0-9.]+)"[^>]*yMax="([0-9.]+)"[^>]*>([^<]*)</word>`)
	matches := wordRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil, false
	}

	imgW := float64(imgBounds.Dx())
	imgH := float64(imgBounds.Dy())
	words := make([]ocrWord, 0, len(matches))
	for _, m := range matches {
		if len(m) != 6 {
			continue
		}
		xMin, errX1 := strconv.ParseFloat(m[1], 64)
		yMin, errY1 := strconv.ParseFloat(m[2], 64)
		xMax, errX2 := strconv.ParseFloat(m[3], 64)
		yMax, errY2 := strconv.ParseFloat(m[4], 64)
		if errX1 != nil || errY1 != nil || errX2 != nil || errY2 != nil {
			continue
		}
		text := strings.TrimSpace(html.UnescapeString(m[5]))
		if text == "" {
			continue
		}

		left := int((xMin / pageW) * imgW)
		top := int((yMin / pageH) * imgH)
		width := int(((xMax - xMin) / pageW) * imgW)
		height := int(((yMax - yMin) / pageH) * imgH)
		if width < 1 {
			width = 1
		}
		if height < 1 {
			height = 1
		}
		words = append(words, ocrWord{Text: text, Left: left, Top: top, Width: width, Height: height, Confidence: 100})
	}
	if len(words) == 0 {
		return nil, false
	}
	return words, true
}

func extractPDFWordsWithPDFToHTML(pdfPath string, imgBounds image.Rectangle) ([]ocrWord, bool) {
	tmpOut, err := os.CreateTemp("", "calendar-pdftohtml-*")
	if err != nil {
		return nil, false
	}
	prefix := tmpOut.Name()
	tmpOut.Close()
	os.Remove(prefix)
	xmlPath := prefix + ".xml"
	defer os.Remove(xmlPath)

	cmd := exec.Command("pdftohtml", "-xml", "-f", "1", "-l", "1", "-hidden", pdfPath, prefix)
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	xmlBytes, err := os.ReadFile(xmlPath)
	if err != nil {
		return nil, false
	}
	xmlStr := string(xmlBytes)

	pageRe := regexp.MustCompile(`<page[^>]*height="(\d+)"[^>]*width="(\d+)"`)
	pm := pageRe.FindStringSubmatch(xmlStr)
	if len(pm) != 3 {
		return nil, false
	}
	pageH, errH := strconv.ParseFloat(pm[1], 64)
	pageW, errW := strconv.ParseFloat(pm[2], 64)
	if errH != nil || errW != nil || pageW <= 0 || pageH <= 0 {
		return nil, false
	}

	textRe := regexp.MustCompile(`(?s)<text[^>]*top="(\d+)"[^>]*left="(\d+)"[^>]*width="(\d+)"[^>]*height="(\d+)"[^>]*>(.*?)</text>`)
	matches := textRe.FindAllStringSubmatch(xmlStr, -1)
	if len(matches) == 0 {
		return nil, false
	}

	imgW := float64(imgBounds.Dx())
	imgH := float64(imgBounds.Dy())
	words := make([]ocrWord, 0, len(matches)*2)
	for _, m := range matches {
		if len(m) != 6 {
			continue
		}
		top, err1 := strconv.Atoi(m[1])
		left, err2 := strconv.Atoi(m[2])
		width, err3 := strconv.Atoi(m[3])
		height, err4 := strconv.Atoi(m[4])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || width <= 0 || height <= 0 {
			continue
		}
		text := html.UnescapeString(stripHTMLTagRe.ReplaceAllString(m[5], " "))
		tokens := strings.Fields(strings.TrimSpace(text))
		if len(tokens) == 0 {
			continue
		}

		nTok := len(tokens)
		tokW := width / nTok
		if tokW < 1 {
			tokW = 1
		}
		for i, tok := range tokens {
			topPx := int((float64(top) / pageH) * imgH)
			leftPx := int((float64(left+i*tokW) / pageW) * imgW)
			wPx := int((float64(tokW) / pageW) * imgW)
			hPx := int((float64(height) / pageH) * imgH)
			if wPx < 1 {
				wPx = 1
			}
			if hPx < 1 {
				hPx = 1
			}
			words = append(words, ocrWord{Text: tok, Left: leftPx, Top: topPx, Width: wPx, Height: hPx, Confidence: 100})
		}
	}
	if len(words) == 0 {
		return nil, false
	}
	return words, true
}
