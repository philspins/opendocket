package provincial

import (
	"fmt"
	"html"
	"image"
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
)

var englishMonthNames = map[string]int{
	"january": 1, "february": 2, "march": 3, "april": 4,
	"may": 5, "june": 6, "july": 7, "august": 8,
	"september": 9, "october": 10, "november": 11, "december": 12,
}

var calendarWordRe = regexp.MustCompile(`<word[^>]*xMin="([0-9.]+)"[^>]*yMin="([0-9.]+)"[^>]*xMax="([0-9.]+)"[^>]*yMax="([0-9.]+)"[^>]*>([^<]*)</word>`)
var calendarPageBBoxRe = regexp.MustCompile(`<page[^>]*width="([0-9.]+)"[^>]*height="([0-9.]+)"`)
var calendarPageXMLRe = regexp.MustCompile(`<page[^>]*height="(\d+)"[^>]*width="(\d+)"`)
var calendarTextXMLRe = regexp.MustCompile(`(?s)<text[^>]*top="(\d+)"[^>]*left="(\d+)"[^>]*width="(\d+)"[^>]*height="(\d+)"[^>]*>(.*?)</text>`)
var calendarStripHTMLTagRe = regexp.MustCompile(`<[^>]+>`)

type ocrWord struct {
	Text         string
	Left         int
	Top          int
	Width        int
	Height       int
	Confidence   float64
	ParsedNumber int
}

type monthPos struct {
	month  int
	cx, cy float64
}

func FetchCalendarPDFBytes(client *http.Client, pdfURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, pdfURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenDemocracyCrawler/1.0; +https://github.com/philspins/opendocket)")
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

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func extractTextWithPDFToText(pdfBytes []byte) (string, error) {
	if !hasCommand("pdftotext") {
		return "", fmt.Errorf("pdftotext not installed")
	}
	tmpIn, err := os.CreateTemp("", "calendar-text-*.pdf")
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

func renderAndOCRCalendarPage(pdfBytes []byte) (image.Image, []ocrWord, bool) {
	tmpPDF, err := os.CreateTemp("", "calendar-highlight-*.pdf")
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
				headings = append(headings, monthPos{month: m, cx: float64(w.Left + w.Width/2), cy: float64(w.Top + w.Height/2)})
				break
			}
		}
	}
	return headings
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
	pm := calendarPageBBoxRe.FindStringSubmatch(s)
	if len(pm) != 3 {
		return nil, false
	}
	pageW, err1 := strconv.ParseFloat(pm[1], 64)
	pageH, err2 := strconv.ParseFloat(pm[2], 64)
	if err1 != nil || err2 != nil || pageW <= 0 || pageH <= 0 {
		return nil, false
	}

	matches := calendarWordRe.FindAllStringSubmatch(s, -1)
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

	pm := calendarPageXMLRe.FindStringSubmatch(xmlStr)
	if len(pm) != 3 {
		return nil, false
	}
	pageH, errH := strconv.ParseFloat(pm[1], 64)
	pageW, errW := strconv.ParseFloat(pm[2], 64)
	if errH != nil || errW != nil || pageW <= 0 || pageH <= 0 {
		return nil, false
	}

	matches := calendarTextXMLRe.FindAllStringSubmatch(xmlStr, -1)
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
		text := html.UnescapeString(calendarStripHTMLTagRe.ReplaceAllString(m[5], " "))
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
