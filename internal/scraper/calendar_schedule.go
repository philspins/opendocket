package scraper

import (
	"database/sql"
	"fmt"
	"html"
	"image"
	"image/color"
	_ "image/png"
	"io"
	"log"
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

	"github.com/philspins/open-democracy/internal/db"
	"github.com/philspins/open-democracy/internal/utils"
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
		log.Printf("[calendar] detected %d dates for %s", len(dates), jurisdiction)
		if len(dates) == 0 {
			continue
		}
		if err := db.ReplaceLegislatureCalendarDates(conn, jurisdiction, dates, scrapedAt); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func crawlLegislatureCalendarDates(client *http.Client, jurisdiction, url string) ([]string, error) {
	year := time.Now().UTC().Year()

	// Jurisdictions that need custom fetching (multiple pages or dynamically constructed URLs).
	switch jurisdiction {
	case "provincial-NS":
		dates, ok := novaScotiaCalendarDates(client, year)
		if !ok {
			return nil, fmt.Errorf("nova scotia calendar returned no dates")
		}
		return dates, nil
	case "provincial-NB":
		dates, ok := newBrunswickCalendarDates(client, year)
		if !ok {
			return nil, fmt.Errorf("new brunswick calendar returned no dates")
		}
		return dates, nil
	case "provincial-NL":
		nlPDFURL := fmt.Sprintf("https://www.assembly.nl.ca/pdfs/ParliamentaryCalendar%d.pdf", year)
		pdfBytes, err := fetchPEICalendarPDFBytes(client, nlPDFURL)
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenDemocracyCrawler/1.0; +https://github.com/philspins/open-democracy)")
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
		if dates, ok := peiCalendarDatesFromPDFBytes(body, year); ok {
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
		dates, ok := quebecCalendarDatesFromPDF(body, year)
		if !ok {
			return nil, fmt.Errorf("QC calendar PDF parsing returned no dates")
		}
		return dates, nil
	case "provincial-MB":
		dates, ok := parseMBHighlightedSittingDatesFromPDF(body, year)
		if !ok {
			return nil, fmt.Errorf("MB calendar PDF parsing returned no dates")
		}
		return dates, nil
	case "provincial-SK":
		dates, ok := parsePDFHighlightedCalendarDates(body, year, isSaskatchewanSittingLike, englishMonthNames, 0.9, 0.02)
		if !ok {
			return nil, fmt.Errorf("SK calendar PDF parsing returned no dates")
		}
		return dates, nil
	}

	text := string(body)

	if jurisdiction == "provincial-ON" {
		if dates, ok := ontarioCalendarDates(text, year); ok {
			return dates, nil
		}
	}
	if jurisdiction == "federal-senate" {
		if dates, ok := senateCalendarDates(client, text, year); ok {
			return dates, nil
		}
	}
	if jurisdiction == "provincial-PE" {
		if dates, ok := peiCalendarDates(client, text, year); ok {
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

	ontarioExceptionSameMoRe  = regexp.MustCompile(`(?i)\b([A-Za-z]+)\s+(\d{1,2})\s+to\s+(\d{1,2})\b`)
	ontarioExceptionCrossMoRe = regexp.MustCompile(`(?i)\b([A-Za-z]+)\s+(\d{1,2})\s+to\s+([A-Za-z]+)\s+(\d{1,2})\b`)
	ontarioExceptionSingleRe  = regexp.MustCompile(`(?i)\b([A-Za-z]+)\s+(\d{1,2})\b`)
	anyCalendarYearRe         = regexp.MustCompile(`(?i)Parliamentary calendar\s+\d{4}`)
	peiCalendarPDFURLRe       = regexp.MustCompile(`(?i)https?://[^\s"']*parliamentary[^\s"']*calendar[^\s"']*\.pdf`)
	peiMarchBreakRangeRe      = regexp.MustCompile(`(?i)March\s+(\d{1,2})\s*[-–]\s*(\d{1,2}),\s*(\d{4})`)
	peiSessionOpeningDateRe   = regexp.MustCompile(`(?i)opening\s+of\s+the\s+\d+(?:st|nd|rd|th)\s+Session[^\n]*?([A-Za-z]+\s+\d{1,2},\s+\d{4})`)

	// Nova Scotia: day-number href links indicate session days.
	nsAgendaDateRe = regexp.MustCompile(`/calendar/agenda/(\d{4}-\d{2}-\d{2})`)

	// New Brunswick: full-date span within calendar cells.
	nbFullDateRe = regexp.MustCompile(`class="full-date"><span>([^<]+)</span>`)

	// Quebec: French date-range patterns on page 2 of the PDF.
	qcDateRangeLongRe  = regexp.MustCompile(`(\d{1,2})\s+(\S+)\s+au\s+(\d{1,2})\s+(\S+)\s+(\d{4})`)
	qcDateRangeShortRe = regexp.MustCompile(`(\d{1,2})\s+au\s+(\d{1,2})\s+(\S+)\s+(\d{4})`)

	dayDigitsOnlyRe = regexp.MustCompile(`^\d+$`)
	senatePDFHrefRe = regexp.MustCompile(`(?i)href=["']([^"']+\.pdf)["']`)
)

func senateCalendarDates(client *http.Client, pageHTML string, year int) ([]string, bool) {
	pdfURL := extractSenateCalendarPDFURL(pageHTML, year)
	if pdfURL == "" {
		return nil, false
	}
	pdfBytes, err := fetchPEICalendarPDFBytes(client, pdfURL)
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

func normalizeCalendarText(body string) string {
	text := stripHTMLTagRe.ReplaceAllString(body, " ")
	text = strings.ReplaceAll(text, "\u00a0", " ")
	return strings.Join(strings.Fields(text), " ")
}

func ontarioCalendarDates(body string, year int) ([]string, bool) {
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
	mainDates := englishDateRe.FindAllString(prefix, -1)
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

func peiCalendarDates(client *http.Client, pageHTML string, year int) ([]string, bool) {
	pdfURL := extractPEICalendarPDFURL(pageHTML, year)
	if pdfURL == "" {
		return nil, false
	}
	pdfBytes, err := fetchPEICalendarPDFBytes(client, pdfURL)
	if err != nil || len(pdfBytes) == 0 {
		return nil, false
	}
	return peiCalendarDatesFromPDFBytes(pdfBytes, year)
}

func peiCalendarDatesFromPDFBytes(pdfBytes []byte, year int) ([]string, bool) {
	return parsePEIHighlightedSittingDatesFromPDF(pdfBytes, year)
}

func extractPEICalendarPDFURL(pageHTML string, year int) string {
	urls := peiCalendarPDFURLRe.FindAllString(pageHTML, -1)
	if len(urls) == 0 {
		return ""
	}
	yearToken := strconv.Itoa(year)
	for _, u := range urls {
		if strings.Contains(u, yearToken) {
			return u
		}
	}
	return urls[0]
}

func extractPEICalendarPDFText(client *http.Client, pdfURL string) (string, error) {
	pdfBytes, err := fetchPEICalendarPDFBytes(client, pdfURL)
	if err != nil {
		return "", err
	}
	return extractPEICalendarPDFTextFromBytes(pdfBytes)
}

func fetchPEICalendarPDFBytes(client *http.Client, pdfURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, pdfURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenDemocracyCrawler/1.0; +https://github.com/philspins/open-democracy)")
	req.Header.Set("Accept", "application/pdf,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pdf fetch status %d", resp.StatusCode)
	}
	pdfBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	return pdfBytes, nil
}

func extractPEICalendarPDFTextFromBytes(pdfBytes []byte) (string, error) {

	text, err := extractTextWithPDFToText(pdfBytes)
	if err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}
	if hasCommand("pdftoppm") && hasCommand("tesseract") {
		ocrText, ocrErr := extractTextWithOCR(pdfBytes)
		if ocrErr == nil && strings.TrimSpace(ocrText) != "" {
			return ocrText, nil
		}
	}
	if err != nil {
		return "", err
	}
	return text, nil
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

func extractTextWithOCR(pdfBytes []byte) (string, error) {
	tmpIn, err := os.CreateTemp("", "pei-calendar-ocr-*.pdf")
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

	imgPrefix := inPath + "-page"
	cmdRaster := exec.Command("pdftoppm", "-f", "1", "-l", "6", "-r", "220", "-gray", "-png", inPath, imgPrefix)
	if err := cmdRaster.Run(); err != nil {
		return "", err
	}

	var all strings.Builder
	for i := 1; i <= 6; i++ {
		imgPath := fmt.Sprintf("%s-%d.png", imgPrefix, i)
		if _, err := os.Stat(imgPath); err != nil {
			continue
		}
		defer os.Remove(imgPath)
		cmdOCR := exec.Command("tesseract", imgPath, "stdout", "-l", "eng")
		out, err := cmdOCR.Output()
		if err != nil {
			continue
		}
		all.Write(out)
		all.WriteString("\n")
	}
	return all.String(), nil
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

func parsePEIHighlightedSittingDatesFromPDF(pdfBytes []byte, year int) ([]string, bool) {
	if !hasCommand("pdftoppm") {
		return nil, false
	}

	img, ok := renderCalendarPageImage(pdfBytes)
	if !ok {
		return nil, false
	}

	// Prefer deterministic PDF bboxes; fall back to OCR words if available.
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

	if len(words) == 0 {
		return nil, false
	}

	// Use confidence threshold only for OCR-derived words.
	isLikelyOCR := false
	for _, w := range words {
		if w.Confidence > 0 && w.Confidence < 100 {
			isLikelyOCR = true
			break
		}
	}

	_ = bounds

	// PEI calendars place month grids in the upper portion of the page; the lower
	// section is legend/text that produces OCR false positives for day numbers.
	maxCalendarY := bounds.Min.Y + int(float64(bounds.Dy())*0.78)

	dayWords := make([]ocrWord, 0, len(words))
	xCenters := make([]float64, 0, len(words))
	yCenters := make([]float64, 0, len(words))
	for _, w := range words {
		if isLikelyOCR && w.Confidence < 30 {
			continue
		}
		appendPEIDayWordCandidates(&dayWords, &xCenters, &yCenters, w, maxCalendarY, true)
	}
	if len(dayWords) < 40 {
		// Fallback: if extraction source is sparse or shifted, retry without
		// vertical cutoff and rely on clustering + color tests to filter noise.
		dayWords = dayWords[:0]
		xCenters = xCenters[:0]
		yCenters = yCenters[:0]
		for _, w := range words {
			if isLikelyOCR && w.Confidence < 30 {
				continue
			}
			appendPEIDayWordCandidates(&dayWords, &xCenters, &yCenters, w, maxCalendarY, false)
		}
		if len(dayWords) < 40 {
			// If bbox extraction succeeded but produced too few numeric day tokens,
			// retry using OCR words when tesseract is available.
			if !isLikelyOCR && hasCommand("tesseract") {
				imgOCR, ocrWords, okOCR := renderAndOCRCalendarPage(pdfBytes)
				if okOCR {
					img = imgOCR
					bounds = img.Bounds()
					maxCalendarY = bounds.Min.Y + int(float64(bounds.Dy())*0.78)
					dayWords = dayWords[:0]
					xCenters = xCenters[:0]
					yCenters = yCenters[:0]
					for _, ow := range ocrWords {
						if ow.Confidence < 30 {
							continue
						}
						appendPEIDayWordCandidates(&dayWords, &xCenters, &yCenters, ow, maxCalendarY, true)
					}
					if len(dayWords) < 40 {
						dayWords = dayWords[:0]
						xCenters = xCenters[:0]
						yCenters = yCenters[:0]
						for _, ow := range ocrWords {
							if ow.Confidence < 30 {
								continue
							}
							appendPEIDayWordCandidates(&dayWords, &xCenters, &yCenters, ow, maxCalendarY, false)
						}
					}
				}
			}
			if len(dayWords) < 40 {
				return nil, false
			}
		}
	}

	colCenters, ok := cluster1D(xCenters, 3)
	if !ok {
		return nil, false
	}
	rowCenters, ok := cluster1D(yCenters, 4)
	if !ok {
		return nil, false
	}
	sort.Float64s(colCenters)
	sort.Float64s(rowCenters)

	greenWeeks := map[time.Time]struct{}{}
	holidayDates := map[string]struct{}{}
	for _, w := range dayWords {
		cx := float64(w.Left + w.Width/2)
		cy := float64(w.Top + w.Height/2)
		col := nearestClusterIndex(cx, colCenters)
		row := nearestClusterIndex(cy, rowCenters)
		month := row*3 + col + 1
		if month < 1 || month > 12 {
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
		green, violet := classifyPEICalendarCellColors(img, cell)
		if !green {
			continue
		}
		weekStart := mondayOfWeek(date)
		greenWeeks[weekStart] = struct{}{}
		if violet {
			holidayDates[date.Format("2006-01-02")] = struct{}{}
		}
	}

	if len(greenWeeks) == 0 {
		return nil, false
	}

	seen := map[string]struct{}{}
	var out []string
	for weekStart := range greenWeeks {
		for offset := 1; offset <= 4; offset++ { // Tue-Fri
			d := weekStart.AddDate(0, 0, offset)
			if d.Year() != year {
				continue
			}
			iso := d.Format("2006-01-02")
			if _, holiday := holidayDates[iso]; holiday {
				continue
			}
			if _, exists := seen[iso]; exists {
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

func appendPEIDayWordCandidates(dayWords *[]ocrWord, xCenters *[]float64, yCenters *[]float64, w ocrWord, maxCalendarY int, enforceY bool) {
	text := strings.TrimSpace(w.Text)
	if text == "" {
		return
	}

	appendCandidate := func(c ocrWord) {
		cy := c.Top + c.Height/2
		if enforceY && cy > maxCalendarY {
			return
		}
		*dayWords = append(*dayWords, c)
		*xCenters = append(*xCenters, float64(c.Left+c.Width/2))
		*yCenters = append(*yCenters, float64(cy))
	}

	if n, err := strconv.Atoi(text); err == nil {
		if n < 1 || n > 31 {
			if !dayDigitsOnlyRe.MatchString(text) {
				return
			}
			ns := splitCalendarDayToken(text)
			if len(ns) == 0 {
				return
			}
			segW := w.Width / len(ns)
			if segW < 1 {
				segW = 1
			}
			for i, v := range ns {
				c := w
				c.Text = strconv.Itoa(v)
				c.ParsedNumber = v
				c.Left = w.Left + i*segW
				c.Width = segW
				appendCandidate(c)
			}
			return
		}
		w.ParsedNumber = n
		appendCandidate(w)
		return
	}
}

func splitCalendarDayToken(s string) []int {
	if !dayDigitsOnlyRe.MatchString(s) || len(s) <= 1 {
		return nil
	}
	type key struct {
		idx  int
		prev int
	}
	type best struct {
		score int
		vals  []int
		ok    bool
	}
	memo := map[key]best{}
	var dfs func(i, prev int) best
	dfs = func(i, prev int) best {
		k := key{idx: i, prev: prev}
		if b, ok := memo[k]; ok {
			return b
		}
		if i == len(s) {
			return best{score: 0, vals: []int{}, ok: true}
		}
		out := best{score: -1, ok: false}
		for _, ln := range []int{1, 2} {
			if i+ln > len(s) {
				continue
			}
			n, err := strconv.Atoi(s[i : i+ln])
			if err != nil || n < 1 || n > 31 {
				continue
			}
			next := dfs(i+ln, n)
			if !next.ok {
				continue
			}
			bonus := 1
			if prev > 0 && n == prev+1 {
				bonus += 100
			}
			score := bonus + next.score
			if !out.ok || score > out.score {
				vals := make([]int, 0, 1+len(next.vals))
				vals = append(vals, n)
				vals = append(vals, next.vals...)
				out = best{score: score, vals: vals, ok: true}
			}
		}
		memo[k] = out
		return out
	}
	b := dfs(0, -1)
	if !b.ok || len(b.vals) < 2 {
		return nil
	}
	return b.vals
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

// isOliveLike detects olive/khaki highlights where R≈G >> B (used by PEI calendar).
func isOliveLike(c color.NRGBA) bool {
	return int(c.R) >= 100 && int(c.G) >= 100 &&
		int(c.R)-int(c.B) >= 40 && int(c.G)-int(c.B) >= 40 &&
		abs(int(c.R)-int(c.G)) <= 20
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// classifyPEICalendarCellColors detects PEI sitting-week cells (olive/khaki) and holidays (violet).
func classifyPEICalendarCellColors(img image.Image, rect image.Rectangle) (sitting bool, violet bool) {
	total := 0
	sittingCount := 0
	violetCount := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r16, g16, b16, _ := img.At(x, y).RGBA()
			c := color.NRGBA{R: uint8(r16 >> 8), G: uint8(g16 >> 8), B: uint8(b16 >> 8), A: 255}
			total++
			if isOliveLike(c) || isGreenLike(c) {
				sittingCount++
			}
			if isVioletLike(c) {
				violetCount++
			}
		}
	}
	if total == 0 {
		return false, false
	}
	sitting = float64(sittingCount)/float64(total) >= 0.08
	violet = violetCount >= 8 && float64(violetCount)/float64(total) >= 0.01
	return sitting, violet
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

func parsePEIDatesFromCalendarText(text string, year int) []string {
	norm := normalizeCalendarText(text)
	springStart := fourthTuesdayInFebruary(year)
	if m := peiSessionOpeningDateRe.FindStringSubmatch(norm); len(m) == 2 {
		if t, err := time.Parse("January 2, 2006", m[1]); err == nil && t.Year() == year {
			springStart = dayStartUTC(t)
		}
	}
	fallStart := firstTuesdayInNovember(year)

	planning := map[string]struct{}{}
	addWeekdaysRange(planning, mondayOfWeek(springStart).AddDate(0, 0, -7), mondayOfWeek(springStart).AddDate(0, 0, -3))
	addWeekdaysRange(planning, mondayOfWeek(fallStart).AddDate(0, 0, -7), mondayOfWeek(fallStart).AddDate(0, 0, -3))
	if m := peiMarchBreakRangeRe.FindStringSubmatch(norm); len(m) == 4 {
		startDay, _ := strconv.Atoi(m[1])
		endDay, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		if y == year {
			start := dayStartUTC(time.Date(year, time.March, startDay, 0, 0, 0, 0, time.UTC))
			end := dayStartUTC(time.Date(year, time.March, endDay, 0, 0, 0, 0, time.UTC))
			addWeekdaysRange(planning, start, end)
		}
	}

	springEnd := dayStartUTC(time.Date(year, time.June, 30, 0, 0, 0, 0, time.UTC))
	fallEnd := dayStartUTC(time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC))

	seen := map[string]struct{}{}
	var out []string
	appendSittingDays := func(start, end time.Time) {
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			wd := d.Weekday()
			if wd < time.Tuesday || wd > time.Friday {
				continue
			}
			iso := d.Format("2006-01-02")
			if _, blocked := planning[iso]; blocked {
				continue
			}
			if _, ok := seen[iso]; ok {
				continue
			}
			seen[iso] = struct{}{}
			out = append(out, iso)
		}
	}
	appendSittingDays(springStart, springEnd)
	appendSittingDays(fallStart, fallEnd)
	sort.Strings(out)
	return out
}

func addWeekdaysRange(dst map[string]struct{}, start, end time.Time) {
	for d := dayStartUTC(start); !d.After(dayStartUTC(end)); d = d.AddDate(0, 0, 1) {
		wd := d.Weekday()
		if wd < time.Monday || wd > time.Friday {
			continue
		}
		dst[d.Format("2006-01-02")] = struct{}{}
	}
}

func mondayOfWeek(t time.Time) time.Time {
	t = dayStartUTC(t)
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return t.AddDate(0, 0, -(wd - 1))
}

func fourthTuesdayInFebruary(year int) time.Time {
	d := time.Date(year, time.February, 1, 0, 0, 0, 0, time.UTC)
	for d.Weekday() != time.Tuesday {
		d = d.AddDate(0, 0, 1)
	}
	return d.AddDate(0, 0, 21)
}

func firstTuesdayInNovember(year int) time.Time {
	d := time.Date(year, time.November, 1, 0, 0, 0, 0, time.UTC)
	for d.Weekday() != time.Tuesday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// ----------------------------------------------------------------------------
// Nova Scotia — HTML calendar, session days have a /calendar/agenda/ href link
// ----------------------------------------------------------------------------

func novaScotiaCalendarDates(client *http.Client, year int) ([]string, bool) {
	seen := map[string]struct{}{}
	now := time.Now().UTC()
	for i := -1; i <= 6; i++ {
		month := now.AddDate(0, i, 0)
		var url string
		if i == 0 {
			url = "https://nslegislature.ca/get-involved/calendar"
		} else {
			url = fmt.Sprintf("https://nslegislature.ca/get-involved/calendar/month/%d-%02d", month.Year(), month.Month())
		}
		body, err := fetchCalendarPage(client, url)
		if err != nil {
			continue
		}
		for _, m := range nsAgendaDateRe.FindAllStringSubmatch(body, -1) {
			t, err := time.Parse("2006-01-02", m[1])
			if err != nil || t.Year() != year {
				continue
			}
			seen[m[1]] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, true
}

// ----------------------------------------------------------------------------
// New Brunswick — HTML calendar, non-empty non-placeholder cells are session days
// ----------------------------------------------------------------------------

func newBrunswickCalendarDates(client *http.Client, year int) ([]string, bool) {
	seen := map[string]struct{}{}
	now := time.Now().UTC()
	for i := -1; i <= 5; i++ {
		month := now.AddDate(0, i, 0)
		url := fmt.Sprintf("https://www.legnb.ca/en/calendar/%d-%d", month.Year(), int(month.Month()))
		body, err := fetchCalendarPage(client, url)
		if err != nil {
			continue
		}
		for _, d := range parseNBCalendarHTML(body, year) {
			seen[d] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, true
}

func parseNBCalendarHTML(html string, year int) []string {
	// Split by calendar-cell divs; non-empty, non-placeholder cells are session days.
	parts := strings.Split(html, `class="calendar-cell`)
	var dates []string
	for _, part := range parts[1:] {
		// Determine the class suffix (between current position and closing quote).
		quoteIdx := strings.Index(part, `"`)
		if quoteIdx < 0 {
			continue
		}
		classSuffix := part[:quoteIdx]
		if strings.Contains(classSuffix, "empty") || strings.Contains(classSuffix, "placeholder") {
			continue
		}
		m := nbFullDateRe.FindStringSubmatch(part)
		if m == nil {
			continue
		}
		t, err := time.Parse("January 2, 2006", strings.TrimSpace(m[1]))
		if err != nil || t.Year() != year {
			continue
		}
		dates = append(dates, t.Format("2006-01-02"))
	}
	return dates
}

// ----------------------------------------------------------------------------
// fetchCalendarPage — shared HTTP helper for multi-page HTML calendar fetches
// ----------------------------------------------------------------------------

func fetchCalendarPage(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenDemocracyCrawler/1.0; +https://github.com/philspins/open-democracy)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-CA,en;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

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

// isLightGreyLike detects the light grey used for MB scheduled house sittings.
func isLightGreyLike(c color.NRGBA) bool {
	r, g, b := int(c.R), int(c.G), int(c.B)
	if r > 248 && g > 248 && b > 248 {
		return false // white background
	}
	if r < 130 || g < 130 || b < 130 {
		return false // too dark or coloured
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

// isWarmTanLike detects the warm tan/sepia used for SK sitting days.
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

func isSaskatchewanSittingLike(c color.NRGBA) bool {
	// SK highlights appear as grey in some exports and warm tan in others.
	return isNeutralGreyLike(c) || isWarmTanLike(c)
}

// ----------------------------------------------------------------------------
// Quebec — text-based parser (reads session date ranges from PDF page 2)
// ----------------------------------------------------------------------------

func quebecCalendarDatesFromPDF(pdfBytes []byte, year int) ([]string, bool) {
	text, err := extractPDFTextPages(pdfBytes, 2, 2)
	if err != nil || strings.TrimSpace(text) == "" {
		var err2 error
		text, err2 = extractTextWithPDFToText(pdfBytes)
		if err2 != nil || strings.TrimSpace(text) == "" {
			return nil, false
		}
	}
	return parseQCScheduleText(text, year)
}

func extractPDFTextPages(pdfBytes []byte, firstPage, lastPage int) (string, error) {
	if !hasCommand("pdftotext") {
		return "", fmt.Errorf("pdftotext not installed")
	}
	tmpIn, err := os.CreateTemp("", "calendar-pages-*.pdf")
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
	args := []string{"-layout", "-f", strconv.Itoa(firstPage), "-l", strconv.Itoa(lastPage), inPath, "-"}
	out, err := exec.Command("pdftotext", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func parseQCScheduleText(text string, year int) ([]string, bool) {
	textLower := strings.ToLower(text)
	intensifIdx := strings.Index(textLower, "intensif")

	var regularSection, intensiveSection string
	if intensifIdx >= 0 {
		regularSection = text[:intensifIdx]
		intensiveSection = text[intensifIdx:]
	} else {
		regularSection = text
	}

	seen := map[string]struct{}{}

	// Regular work: Assembly sits Tue/Wed/Thu.
	for _, rng := range parseQCDateRanges(regularSection, year) {
		addSpecificWeekdays(seen, rng.start, rng.end,
			[]time.Weekday{time.Tuesday, time.Wednesday, time.Thursday})
	}
	// Intensive work: Assembly sits Tue/Wed/Thu/Fri.
	if intensiveSection != "" {
		for _, rng := range parseQCDateRanges(intensiveSection, year) {
			addSpecificWeekdays(seen, rng.start, rng.end,
				[]time.Weekday{time.Tuesday, time.Wednesday, time.Thursday, time.Friday})
		}
	}

	if len(seen) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, true
}

type qcDateRange struct{ start, end time.Time }

func parseQCDateRanges(text string, year int) []qcDateRange {
	var ranges []qcDateRange

	// Cross-month: "3 février au 28 mai 2026"
	for _, m := range qcDateRangeLongRe.FindAllStringSubmatch(text, -1) {
		startDay, _ := strconv.Atoi(m[1])
		startMonth := parseFrenchMonth(m[2])
		endDay, _ := strconv.Atoi(m[3])
		endMonth := parseFrenchMonth(m[4])
		rangeYear, _ := strconv.Atoi(m[5])
		if rangeYear != year || startMonth == 0 || endMonth == 0 {
			continue
		}
		start := time.Date(year, time.Month(startMonth), startDay, 0, 0, 0, 0, time.UTC)
		end := time.Date(year, time.Month(endMonth), endDay, 0, 0, 0, 0, time.UTC)
		if !start.After(end) {
			ranges = append(ranges, qcDateRange{start, end})
		}
	}

	// Same-month: "2 au 12 juin 2026"
	for _, m := range qcDateRangeShortRe.FindAllStringSubmatch(text, -1) {
		startDay, _ := strconv.Atoi(m[1])
		endDay, _ := strconv.Atoi(m[2])
		month := parseFrenchMonth(m[3])
		rangeYear, _ := strconv.Atoi(m[4])
		if rangeYear != year || month == 0 {
			continue
		}
		start := time.Date(year, time.Month(month), startDay, 0, 0, 0, 0, time.UTC)
		end := time.Date(year, time.Month(month), endDay, 0, 0, 0, 0, time.UTC)
		if !start.After(end) {
			ranges = append(ranges, qcDateRange{start, end})
		}
	}
	return ranges
}

var frenchMonthSubstrs = []struct {
	sub   string
	month int
}{
	{"janv", 1},
	{"vrier", 2}, // février / fΘvrier
	{"mars", 3},
	{"avri", 4},
	{"mai", 5},
	{"juin", 6},
	{"juil", 7},
	{"ao", 8}, // août / aoΦt
	{"sept", 9},
	{"octo", 10},
	{"novem", 11},
	{"cembre", 12}, // décembre / dΘcembre
}

func parseFrenchMonth(s string) int {
	s = strings.ToLower(s)
	for _, e := range frenchMonthSubstrs {
		if strings.Contains(s, e.sub) {
			return e.month
		}
	}
	return 0
}

func addSpecificWeekdays(seen map[string]struct{}, start, end time.Time, weekdays []time.Weekday) {
	wdSet := map[time.Weekday]bool{}
	for _, wd := range weekdays {
		wdSet[wd] = true
	}
	for d := dayStartUTC(start); !d.After(end); d = d.AddDate(0, 0, 1) {
		if wdSet[d.Weekday()] {
			seen[d.Format("2006-01-02")] = struct{}{}
		}
	}
}

// ----------------------------------------------------------------------------
// Manitoba — image parser using fixed 2x4 month-grid layout in the 2026 PDF.
// Rows: [Mar Apr], [May Jun], [Sep Oct], [Nov Dec]
// ----------------------------------------------------------------------------

func parseMBHighlightedSittingDatesFromPDF(pdfBytes []byte, year int) ([]string, bool) {
	// Preferred path: PDF text bbox + image highlight detection (no HTML required).
	if dates, ok := parseMBHighlightedSittingDatesFromPDFBBox(pdfBytes, year); ok {
		return dates, true
	}

	// Fallback path: OCR day-number clustering on the PDF image.
	if dates, ok := parseMBHighlightedSittingDatesFromPDFOCR(pdfBytes, year); ok {
		return dates, true
	}

	// Last-resort fallback: derive likely sitting weekdays from the MB sessional
	// calendar PDF text when visual highlight extraction is unavailable.
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
			// Manitoba typically sits midweek; use Tue-Thu as conservative fallback.
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
			// Use weighted distance only; some PDF bbox coordinate systems invert Y,
			// so requiring heading-above-day is unreliable across sources.
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
		month := mbMonthFromGrid(row, col)
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

func mbMonthFromGrid(row, col int) int {
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
