package provincial

import (
	"strings"
	"testing"
)

func TestNewBrunswickJournalDate_PrefersPDFTextDate(t *testing.T) {
	pdfURL := "https://example.com/journals/34000015.pdf"
	text := "Journal of Debates March 27, 2025 RECORDED DIVISION YEAS - 14"

	got := newBrunswickJournalDate(pdfURL, text)
	if got != "2025-03-27" {
		t.Fatalf("newBrunswickJournalDate()=%q, want %q", got, "2025-03-27")
	}
}

func TestParseNewBrunswickPDFDivisions_ParsesMemberNamesFromVoteBlock(t *testing.T) {
	text := `RECORDED DIVISION YEAS - 14 Mr. Hogan Mr. Monahan Ms. S. Wilson Ms. M. Johnson Mr. Ames Mr. Cullins Mr. Savoie Mr. Weir Ms. Bockus Ms. Scott - Wallace Ms. Conroy Mr. Lee Mr. Austin Mr. Oliver NAYS - 25 Hon. Mr. Gauvin Hon. Mr. C. Chiasson Mr. J. LeBlanc Mr. M. LeBlanc Hon. Ms. Holt And the question being put`

	divs := ParseNewBrunswickPDFDivisionsForTest(text, "https://example.com/journal.pdf", 61, 1, 1, "2025-03-27")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if divs[0].Division.Yeas != 14 || divs[0].Division.Nays != 25 {
		t.Fatalf("counts=(%d,%d), want (14,25)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
	if len(divs[0].Votes) < 18 {
		t.Fatalf("len(votes)=%d, want >=18", len(divs[0].Votes))
	}
}

func TestExtractDateFromURL_RejectsImplausibleOrInvalidDates(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "valid iso date", url: "https://example.com/journals/2025-03-27.pdf", want: "2025-03-27"},
		{name: "valid compact date", url: "https://example.com/journals/20250327.pdf", want: "2025-03-27"},
		{name: "invalid opaque id", url: "https://example.com/journals/34000015.pdf", want: ""},
		{name: "invalid month/day", url: "https://example.com/journals/20251340.pdf", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractDateFromURL(tc.url); got != tc.want {
				t.Fatalf("extractDateFromURL(%q)=%q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestParseNewBrunswickVoteNames_KeepsInitialAndSurname(t *testing.T) {
	block := `YEAS - 25 Hon. Mr. Hogan Hon. Ms. S. Wilson Ms. Scott - Wallace Mr. J. LeBlanc Hon. Mr. G. Savoie`
	names := parseNewBrunswickVoteNames(block)
	want := []string{"Hogan", "S. Wilson", "Scott-Wallace", "J. LeBlanc", "G. Savoie"}
	if len(names) != len(want) {
		t.Fatalf("len(names)=%d, want %d (%v)", len(names), len(want), names)
	}
	for i, got := range names {
		if got != want[i] {
			t.Fatalf("names[%d]=%q, want %q (all=%v)", i, got, want[i], names)
		}
	}
}

func TestNewBrunswickDescriptionFromContext_PrefersSubstantiveMotionText(t *testing.T) {
	text := `THAT Bill 10 be now read a third time and passed. And the debate being ended, and the question being put on the amendment, it was defeat ed on the following recorded division after leave was granted to dispense with the ten - minute time allotted for the ringing of the bells : RECORDED DIVISION YEAS - 19 Mr. A NAYS - 25 Mr. B`
	matchStart := strings.Index(text, "YEAS - 19")
	if matchStart < 0 {
		t.Fatal("YEAS marker not found in test text")
	}

	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if !strings.Contains(strings.ToLower(desc), "bill 10") {
		t.Fatalf("desc=%q; expected substantive bill context", desc)
	}
	if strings.Contains(strings.ToLower(desc), "debate being ended") {
		t.Fatalf("desc=%q; procedural boilerplate should be stripped", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_ExtractsBillNo(t *testing.T) {
	text := `AN ACT TO AMEND THE MUNICIPALITIES ACT Bill No. 47 YEAS - 23`
	matchStart := strings.Index(text, "YEAS")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if !strings.Contains(desc, "47") {
		t.Errorf("description %q should contain bill number 47", desc)
	}
	if !strings.Contains(desc, "Bill") {
		t.Errorf("description %q should contain 'Bill' (not just the bare number fragment)", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersAgreedToBoilerplate(t *testing.T) {
	// "It was agreed to, on the following division" is procedural outcome text, not a
	// meaningful description. The boilerplate filter must strip it so an earlier
	// substantive sentence is returned instead.
	text := `THAT the proposed amendment be accepted. It was agreed to, on the following division: YEAS - 20`
	matchStart := strings.Index(text, "YEAS")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if strings.Contains(strings.ToLower(desc), "it was agreed") {
		t.Errorf("desc=%q; boilerplate outcome phrase should be filtered", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersNegativedBoilerplate(t *testing.T) {
	text := `THAT the motion be adopted. And the question being put. It was negatived, on the following division: YEAS - 5`
	matchStart := strings.Index(text, "YEAS")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if strings.Contains(strings.ToLower(desc), "it was negatived") {
		t.Errorf("desc=%q; boilerplate outcome phrase should be filtered", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersSpeakerResumedChair(t *testing.T) {
	// OCR of "Madam Speaker resumed the chair" appears as procedural text in MB journals.
	text := `Some debate occurred. And after some time, M adam Speaker resumed the chair. YEAS - 30`
	matchStart := strings.Index(text, "YEAS")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if strings.Contains(strings.ToLower(desc), "speaker resumed") {
		t.Errorf("desc=%q; 'speaker resumed the chair' should be filtered as boilerplate", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersOnFollowingDivisionWithoutRecorded(t *testing.T) {
	// Manitoba journals use "on the following division" (no "recorded"); verify it's filtered.
	text := `THAT Bill No. 10 be read a third time. And the Question being put. It was agreed to, on the following division: AYE Smith Jones`
	matchStart := strings.Index(text, "AYE")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if strings.Contains(strings.ToLower(desc), "on the following division") {
		t.Errorf("desc=%q; 'on the following division' should be filtered as boilerplate", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersAllCapsNameList(t *testing.T) {
	// When a previous division's voter names appear in the context window they must
	// not be returned as the description — they're a list of all-caps surnames.
	text := `CORBETT CROSS DELA CRUZ DEVGAN FONTAINE KENNEDY KINEW KOSTYSHYN LOISELLE MALOWAY MARCELINO ....33 And the Question being put on the main motion. It was agreed to, on the following division: AYE`
	matchStart := strings.Index(text, "AYE")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	// The all-caps name list must not appear as the description.
	if strings.Contains(desc, "CORBETT") || strings.Contains(desc, "MALOWAY") {
		t.Errorf("desc=%q; all-caps voter name list should be filtered", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersShortFragment(t *testing.T) {
	// Sentence-split fragments like "Mr" (from "Hon. Mr. KINEW having spoken") are too
	// short to be meaningful descriptions and must be skipped.
	text := `Some substantive motion text here that is clearly meaningful. Hon. Mr. KINEW having spoken, And the Question being put on the amendment. It was negatived, on the following division: AYE`
	matchStart := strings.Index(text, "AYE")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if desc == "Mr" || desc == "Hon" || len(strings.TrimSpace(desc)) < 5 {
		t.Errorf("desc=%q; very short fragments should be filtered", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersSingleAllCapsToken(t *testing.T) {
	// A single all-caps surname from a previous division's vote tail (e.g. "WOWCHUK")
	// must not be returned as the description.
	text := `WOWCHUK .....33 And the Question being put on the next motion. It was agreed to, on the following division: AYE`
	matchStart := strings.Index(text, "AYE")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if desc == "WOWCHUK" || desc == "33" {
		t.Errorf("desc=%q; single all-caps voter name should be filtered", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersDebateContinuingBoilerplate(t *testing.T) {
	// "And the debate continuing on the amendment, And Mrs. HIEBERT having spoken"
	// is procedural attribution text, not a meaningful description.
	text := `THAT the amendment be adopted. And the debate continuing on the amendment, And Mrs. HIEBERT having spoken. It was negatived, on the following division: AYE`
	matchStart := strings.Index(text, "AYE")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if strings.Contains(strings.ToLower(desc), "debate continuing") {
		t.Errorf("desc=%q; 'debate continuing' procedural text should be filtered", desc)
	}
	if strings.Contains(strings.ToLower(desc), "having spoken") {
		t.Errorf("desc=%q; 'having spoken' attribution should be filtered", desc)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersMovedByHonFragment(t *testing.T) {
	// "Motion 56, moved by Hon" is a truncated attribution sentence (the period
	// after "Hon." creates the sentence-split).  It must not be returned as a
	// description; a preceding substantive sentence should be preferred.
	text := `THAT the amendment be adopted. Motion 56, moved by Hon. Mr. Gauvin. It was agreed to, on the following division: YEAS`
	matchStart := strings.Index(text, "YEAS")
	desc := newBrunswickDescriptionFromContext(text, matchStart)
	if strings.Contains(strings.ToLower(desc), "moved by hon") {
		t.Errorf("desc=%q; truncated 'moved by Hon' attribution should be filtered", desc)
	}
}
