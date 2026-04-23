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
