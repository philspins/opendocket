package scraper

import (
	"image"
	"image/color"
	"sort"
	"testing"
	"time"
)

func TestOntarioCalendarDates_SelectsCurrentYearBlock(t *testing.T) {
	body := `
		<h2>Parliamentary calendar 2025</h2>
		<p>The House may meet from Monday to Thursday, from April 14, 2025, to December 11, 2025, with the following exceptions:</p>
		<p>June 9 to October 16</p>
		<h2>Parliamentary calendar 2026</h2>
		<p>The House may meet from Monday to Thursday, from March 23, 2026, to December 10, 2026, with the following exceptions:</p>
		<p>April 6 to 9</p>
		<p>April 27 to 30</p>
	`
	dates, ok := ontarioCalendarDates(body, 2026)
	if !ok {
		t.Fatalf("expected Ontario parser to match 2026 section")
	}
	if len(dates) == 0 {
		t.Fatalf("expected non-empty generated date list")
	}
	for _, d := range []string{"2026-04-06", "2026-04-07", "2026-04-08", "2026-04-09", "2026-04-27", "2026-04-28", "2026-04-29", "2026-04-30"} {
		for _, got := range dates {
			if d == got {
				t.Fatalf("did not expect exception date %s in generated dates", d)
			}
		}
	}
	found := false
	for _, got := range dates {
		if got == "2026-04-22" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 2026-04-22 to be generated as in-session date")
	}
}

func TestExtractCalendarDatesFromText(t *testing.T) {
	in := `<div data-date="2026-04-22"></div><p>April 24, 2026</p><p>24 April 2026</p>`
	got := extractCalendarDatesFromText(in)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 unique dates, got %v", got)
	}
}

func TestDayStartUTC(t *testing.T) {
	n := time.Date(2026, time.April, 22, 15, 30, 0, 0, time.FixedZone("X", -5*3600))
	d := dayStartUTC(n)
	if d.Hour() != 0 || d.Minute() != 0 || d.Second() != 0 {
		t.Fatalf("expected midnight, got %v", d)
	}
}

func TestParsePEIDatesFromCalendarText(t *testing.T) {
	text := `
		Parliamentary Calendar 2026
		Sitting Schedule
		In keeping with the Rules of the Legislative Assembly, the first day of the winter/spring sitting is the fourth Tuesday of February,
		and the first day of the fall sitting is the first Tuesday in November.
		Note on calendar update: The 2nd session of the 67th General Assembly was prorogued February 20, 2026,
		and the opening of the 3rd Session set for 1:00pm, Tuesday, March 24, 2026.
		Legislative Planning Weeks
		one legislative planning week is scheduled for the week prior to the winter/spring sitting and the fall sitting;
		one legislative planning week to coincide with March Break (March 16-20, 2026).
	`
	dates := parsePEIDatesFromCalendarText(text, 2026)
	if len(dates) == 0 {
		t.Fatalf("expected generated PEI dates")
	}

	contains := func(needle string) bool {
		for _, d := range dates {
			if d == needle {
				return true
			}
		}
		return false
	}

	if !contains("2026-04-22") {
		t.Fatalf("expected 2026-04-22 to be included")
	}
	if contains("2026-03-17") {
		t.Fatalf("did not expect March break date 2026-03-17")
	}
	if contains("2026-03-23") {
		t.Fatalf("did not expect Monday non-sitting date 2026-03-23")
	}
	if !contains("2026-11-03") {
		t.Fatalf("expected fall sitting start 2026-11-03 to be included")
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
