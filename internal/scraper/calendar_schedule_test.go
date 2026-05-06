package scraper

import (
	"image"
	"image/color"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestExtractCalendarDatesFromText(t *testing.T) {
	in := `<div data-date="2026-04-22"></div><p>April 24, 2026</p><p>24 April 2026</p>`
	got := extractCalendarDatesFromText(in)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 unique dates, got %v", got)
	}
}

func TestExtractCalendarDatesFromText_FiltersFarDates(t *testing.T) {
	now := time.Now().UTC()
	inRange := now.Format("2006-01-02")
	tooOld := now.AddDate(-5, 0, 0).Format("2006-01-02")
	tooFuture := now.AddDate(5, 0, 0).Format("2006-01-02")
	in := strings.Join([]string{
		`<div data-date="` + inRange + `"></div>`,
		`<div data-date="` + tooOld + `"></div>`,
		`<div data-date="` + tooFuture + `"></div>`,
	}, "")

	got := extractCalendarDatesFromText(in)
	if len(got) != 1 || got[0] != inRange {
		t.Fatalf("expected only in-range date %q, got %v", inRange, got)
	}
}

func TestDayStartUTC(t *testing.T) {
	n := time.Date(2026, time.April, 22, 15, 30, 0, 0, time.FixedZone("X", -5*3600))
	d := dayStartUTC(n)
	if d.Hour() != 0 || d.Minute() != 0 || d.Second() != 0 {
		t.Fatalf("expected midnight, got %v", d)
	}
}

func TestCluster1D(t *testing.T) {
	values := []float64{12, 18, 24, 110, 125, 138, 220, 235, 248}
	centers, ok := cluster1D(values, 3)
	if !ok {
		t.Fatalf("expected clustering to succeed")
	}
	if len(centers) != 3 {
		t.Fatalf("expected 3 centers, got %d", len(centers))
	}

	sort.Float64s(centers)
	if centers[0] >= 60 || centers[1] <= 80 || centers[1] >= 190 || centers[2] <= 200 {
		t.Fatalf("unexpected centers: %v", centers)
	}
}

func TestClassifyCalendarCellColors(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 60, 40))

	// Fill base as white.
	for y := 0; y < 40; y++ {
		for x := 0; x < 60; x++ {
			img.Set(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	// Green sitting area.
	for y := 5; y < 30; y++ {
		for x := 5; x < 45; x++ {
			img.Set(x, y, color.NRGBA{R: 120, G: 190, B: 120, A: 255})
		}
	}

	// Violet holiday triangle-like patch.
	for y := 5; y < 12; y++ {
		for x := 35; x < 42; x++ {
			img.Set(x, y, color.NRGBA{R: 150, G: 80, B: 155, A: 255})
		}
	}

	green, violet := classifyCalendarCellColors(img, image.Rect(5, 5, 45, 30))
	if !green {
		t.Fatalf("expected green cell classification")
	}
	if !violet {
		t.Fatalf("expected violet overlay classification")
	}
}

func TestExtractSenateCalendarPDFURL_PrefersRequestedYear(t *testing.T) {
	html := `
		<a href="/media/old/2025-senate-sitting-calendar.pdf">2025</a>
		<a href="/media/current/2026-senate-sitting-calendar.pdf">2026</a>
	`
	got := extractSenateCalendarPDFURL(html, 2026)
	want := "https://sencanada.ca/media/current/2026-senate-sitting-calendar.pdf"
	if got != want {
		t.Fatalf("extractSenateCalendarPDFURL()=%q want %q", got, want)
	}
}

func TestExtractSenateCalendarPDFURL_FallsBackToFirstMatch(t *testing.T) {
	html := `<a href="https://sencanada.ca/media/annual/senate-sitting-calendar.pdf">calendar</a>`
	got := extractSenateCalendarPDFURL(html, 2027)
	want := "https://sencanada.ca/media/annual/senate-sitting-calendar.pdf"
	if got != want {
		t.Fatalf("extractSenateCalendarPDFURL()=%q want %q", got, want)
	}
}

func TestExtractSenateCalendarPDFURL_ReturnsEmptyWhenNoMatch(t *testing.T) {
	got := extractSenateCalendarPDFURL(`<a href="/about">About</a>`, 2026)
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// ── isSenateOpenDayLike ───────────────────────────────────────────────────────

func TestIsSenateOpenDayLike(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b uint8
		want    bool
	}{
		// Red open-day cell (r >= 145, g/b low, r dominates)
		{"red open day", 200, 80, 80, true},
		// Pink open-day cell
		{"pink open day", 220, 150, 160, true},
		// Neutral grey — not a sitting day
		{"grey", 180, 180, 180, false},
		// White — not a sitting day
		{"white", 255, 255, 255, false},
		// Vivid green — not a sitting day
		{"green", 60, 200, 60, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := color.NRGBA{R: tt.r, G: tt.g, B: tt.b, A: 255}
			if got := isSenateOpenDayLike(c); got != tt.want {
				t.Errorf("isSenateOpenDayLike(%d,%d,%d) = %v, want %v", tt.r, tt.g, tt.b, got, tt.want)
			}
		})
	}
}

// ── isGreenLike ───────────────────────────────────────────────────────────────

func TestIsGreenLike(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b uint8
		want    bool
	}{
		{"sitting green", 100, 190, 130, true},
		{"vivid green", 50, 200, 50, true},
		{"white", 255, 255, 255, false},
		{"red", 220, 60, 60, false},
		{"dark grey", 80, 80, 80, false},
		// g=94 is just below the threshold; g=95 is the inclusive lower bound
		{"g just under threshold", 60, 94, 60, false},
		{"g at threshold", 60, 95, 60, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := color.NRGBA{R: tt.r, G: tt.g, B: tt.b, A: 255}
			if got := isGreenLike(c); got != tt.want {
				t.Errorf("isGreenLike(%d,%d,%d) = %v, want %v", tt.r, tt.g, tt.b, got, tt.want)
			}
		})
	}
}

// ── nearestClusterIndex ───────────────────────────────────────────────────────

func TestNearestClusterIndex(t *testing.T) {
	centers := []float64{10.0, 50.0, 90.0}
	tests := []struct {
		value float64
		want  int
	}{
		{8.0, 0},   // clearly closest to 10
		{33.0, 1},  // 17 from 50, 23 from 10
		{75.0, 2},  // 15 from 90, 25 from 50
		{50.0, 1},  // exact centre
		{49.9, 1},  // just below centre
	}
	for _, tt := range tests {
		got := nearestClusterIndex(tt.value, centers)
		if got != tt.want {
			t.Errorf("nearestClusterIndex(%v, %v) = %d, want %d", tt.value, centers, got, tt.want)
		}
	}
}

// ── parseTesseractTSVWords ────────────────────────────────────────────────────

func TestParseTesseractTSVWords_ParsesValidRows(t *testing.T) {
	// TSV header + two data rows (12 tab-separated columns, col 11 is the word text)
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t10\t20\t30\t15\t95.5\tApril\n" +
		"5\t1\t1\t1\t1\t2\t50\t20\t25\t15\t88.0\t2026\n"

	words := parseTesseractTSVWords(tsv)
	if len(words) != 2 {
		t.Fatalf("expected 2 words, got %d", len(words))
	}
	if words[0].Text != "April" || words[0].Left != 10 || words[0].Top != 20 {
		t.Errorf("words[0]=%+v", words[0])
	}
	if words[1].Text != "2026" || words[1].Confidence != 88.0 {
		t.Errorf("words[1]=%+v", words[1])
	}
}

func TestParseTesseractTSVWords_SkipsMalformedAndEmpty(t *testing.T) {
	tsv := "header\n" +
		// too few columns
		"5\t1\t1\n" +
		// empty text (col 11 is empty)
		"5\t1\t1\t1\t1\t1\t10\t20\t30\t15\t90.0\t\n" +
		// non-numeric coordinate
		"5\t1\t1\t1\t1\t1\tX\t20\t30\t15\t90.0\tWord\n" +
		// valid
		"5\t1\t1\t1\t1\t1\t5\t5\t20\t12\t99.0\tOK\n"

	words := parseTesseractTSVWords(tsv)
	if len(words) != 1 || words[0].Text != "OK" {
		t.Fatalf("expected 1 valid word 'OK', got %v", words)
	}
}

func TestCluster1D_FailsWithFewerValuesThanClusters(t *testing.T) {
	_, ok := cluster1D([]float64{1.0, 2.0}, 5)
	if ok {
		t.Fatal("expected cluster1D to fail when len(values) < k")
	}
}

func TestCluster1D_FailsWithZeroClusters(t *testing.T) {
	_, ok := cluster1D([]float64{1.0, 2.0, 3.0}, 0)
	if ok {
		t.Fatal("expected cluster1D to fail with k=0")
	}
}
