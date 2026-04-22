package scraper

import (
	"database/sql"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"io"
	"math"
	"net/http"
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
	"provincial-MB":   "https://www.gov.mb.ca/legislature/business/housecalendar.html",
	"provincial-NB":   "https://www.legnb.ca/en/parliamentary-business/calendar",
	"provincial-NL":   "https://www.assembly.nl.ca/HouseBusiness/ParliamentaryCalendar.aspx",
	"provincial-NS":   "https://nslegislature.ca/legislative-business",
	"provincial-ON":   "https://www.ola.org/en/legislative-business/parliamentary-calendars",
	"provincial-PE":   "https://www.assembly.pe.ca/sites/www.assembly.pe.ca/files/parliamentary%20calendar.2026.pdf",
	"provincial-QC":   "https://www.assnat.qc.ca/en/parliamentary-business/house-proceedings/calendar.html",
	"provincial-SK":   "https://www.legassembly.sk.ca/legislative-business/calendar/",
	"provincial-YT":   "https://yukonassembly.ca/calendar",
	"provincial-NT":   "https://www.ntassembly.ca/",
	"provincial-NU":   "https://www.assembly.nu.ca/",
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if jurisdiction == "provincial-PE" && strings.HasSuffix(strings.ToLower(url), ".pdf") {
		if dates, ok := peiCalendarDatesFromPDFBytes(body, time.Now().UTC().Year()); ok {
			return dates, nil
		}
		return nil, fmt.Errorf("pei pdf parsing returned no dates")
	}

	text := string(body)

	if jurisdiction == "provincial-ON" {
		if dates, ok := ontarioCalendarDates(text, time.Now().UTC().Year()); ok {
			return dates, nil
		}
	}
	if jurisdiction == "provincial-PE" {
		if dates, ok := peiCalendarDates(client, text, time.Now().UTC().Year()); ok {
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
)

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
	if !hasCommand("pdftoppm") || !hasCommand("tesseract") {
		return nil, false
	}

	img, words, ok := renderAndOCRCalendarPage(pdfBytes)
	if !ok {
		return nil, false
	}

	bounds := img.Bounds()
	// PEI calendars place month grids in the top ~60% of the page; the lower
	// section is legend/text that produces OCR false positives for day numbers.
	maxCalendarY := bounds.Min.Y + int(float64(bounds.Dy())*0.62)

	dayWords := make([]ocrWord, 0, len(words))
	xCenters := make([]float64, 0, len(words))
	yCenters := make([]float64, 0, len(words))
	for _, w := range words {
		if w.Confidence < 30 {
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
		w.ParsedNumber = n
		dayWords = append(dayWords, w)
		xCenters = append(xCenters, float64(w.Left+w.Width/2))
		yCenters = append(yCenters, float64(cy))
	}
	if len(dayWords) < 40 {
		return nil, false
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
		green, violet := classifyCalendarCellColors(img, cell)
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

	tsvOut, err := exec.Command("tesseract", imgPath, "stdout", "-l", "eng", "tsv").Output()
	if err != nil {
		return nil, nil, false
	}
	words := parseTesseractTSVWords(string(tsvOut))
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
