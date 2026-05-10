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

// ── Parliamentary motion classifier tests ────────────────────────────────────

func TestClassifyParliamentaryMotion(t *testing.T) {
	cases := []struct {
		snippet string
		want    string
	}{
		// Bill readings
		{"THAT Bill No. 10 be now read a first time", "First Reading: Bill 10"},
		{"THAT the Bill be now read a first time", "First Reading"},
		{"THAT Bill No. 5, An Act Respecting X, be now read a second time", "Second Reading: Bill 5"},
		{"THAT Bill No. 3 be now read a third time and passed", "Third Reading: Bill 3"},
		// Amendments to readings — "not now read"
		{"THAT Bill No. 3 be not now read a second time but that the order for second reading be discharged", "Amendment to Second Reading: Bill 3"},
		{"Bill No. 13, An Act to Amend the Executive Council Act, be not now read a second time", "Amendment to Second Reading: Bill 13"},
		// Amendments to readings — "motion for Nth reading be amended"
		{"THAT the motion for second reading be amended by deleting all of Bill No. 13", "Amendment to Second Reading: Bill 13"},
		{"THAT the motion for third reading of Bill No. 7 be amended", "Amendment to Third Reading: Bill 7"},
		// OCR artifact: "fo r" instead of "for"
		{"THAT the motion fo r third reading be amended", "Amendment to Third Reading"},
		// Bill passes (third reading passage)
		{"that the said Bill does pass", "Third Reading"},
		{"THAT Bill No. 10 be now read a third time and that the said Bill does pass", "Third Reading: Bill 10"},
		// Address in Reply
		{"That the Motion for an Address in Reply to the Speech from the Throne be amended as follows", "Address in Reply: Amendment"},
		{"THAT the Address in Reply to the Throne Speech be adopted", "Address in Reply to the Throne Speech"},
		// Committee of the Whole
		{"THAT the House resolve itself into a Committee of the Whole", "Committee of the Whole"},
		// Budget vote (Manitoba-specific)
		{"That this House approves in general the budgetary policy of the government", "Budget Vote"},
		// Manitoba backslash PDF artifacts
		{"THAT Bill No. 210\\ The Indigenous Veterans Day Act\\ be now read a Second Time", "Second Reading: Bill 210"},
		// No match — generic text falls through
		{"Some unclassifiable procedural text", ""},
		{"THAT the proposed amendment be accepted", ""},
	}
	for _, c := range cases {
		got := classifyParliamentaryMotion(c.snippet)
		if got != c.want {
			t.Errorf("classifyParliamentaryMotion(%q)=%q, want %q", c.snippet, got, c.want)
		}
	}
}

func TestNewBrunswickDescriptionFromContext_ClassifiesReadings(t *testing.T) {
	cases := []struct {
		label string
		text  string
		want  string
	}{
		{
			"second reading",
			"THAT Bill No. 5, An Act to Amend X, be now read a second time. And the question being put: YEAS - 25",
			"Second Reading: Bill 5",
		},
		{
			"amendment to second reading",
			"THAT Bill No. 3, An Act to Amend The Residential Tenancies Act, be not now read a second time but that the order for second reading be discharged. YEAS - 15",
			"Amendment to Second Reading: Bill 3",
		},
		{
			"amendment motion with 'fo r' OCR artifact",
			"THAT the motion fo r third reading be amended by deleting all Bill No. 7. YEAS - 17",
			"Amendment to Third Reading: Bill 7",
		},
		{
			"bill passes",
			"that the said Bill does pass. YEAS - 29",
			"Third Reading",
		},
	}
	for _, c := range cases {
		matchStart := strings.Index(c.text, "YEAS")
		if matchStart < 0 {
			matchStart = len(c.text)
		}
		got := newBrunswickDescriptionFromContext(c.text, matchStart)
		if got != c.want {
			t.Errorf("[%s] newBrunswickDescriptionFromContext()=%q, want %q", c.label, got, c.want)
		}
	}
}

func TestNewBrunswickDescriptionFromContext_ManitobaBackslashArtifacts(t *testing.T) {
	// MB journal PDFs use backslash as a line-break separator. These must be
	// stripped before classification so reading-stage patterns still fire.
	text := `THAT Bill No. 210\ The Indigenous Veterans Day Act Commemoration of Days\ be now read a Second Time. AYE`
	matchStart := strings.Index(text, "AYE")
	got := newBrunswickDescriptionFromContext(text, matchStart)
	if got != "Second Reading: Bill 210" {
		t.Errorf("got=%q, want %q", got, "Second Reading: Bill 210")
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersLowercaseFragment(t *testing.T) {
	// A sentence fragment starting with a lowercase letter is a tail of a split
	// sentence (e.g. "r Honour" from "Your Honour...") and must be skipped.
	text := `Your Honour for the gracious speech. r Honour has addressed us. YEAS - 27`
	matchStart := strings.Index(text, "YEAS")
	got := newBrunswickDescriptionFromContext(text, matchStart)
	if strings.HasPrefix(got, "r ") {
		t.Errorf("got=%q; lowercase-start fragment should be filtered", got)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersJournalDateHeader(t *testing.T) {
	// Journal page headers like "Monday , November 3 , 2025 424" must not be
	// returned as descriptions when they leak into the context window.
	text := `Monday , November 3 , 2025 424 YEAS - 21`
	matchStart := strings.Index(text, "YEAS")
	got := newBrunswickDescriptionFromContext(text, matchStart)
	if strings.Contains(strings.ToLower(got), "monday") {
		t.Errorf("got=%q; journal date header should be filtered", got)
	}
}

func TestNewBrunswickDescriptionFromContext_FiltersVoterNameLeak(t *testing.T) {
	// When a previous division's voter list leaks into the context window, a
	// "single initial + ALL_CAPS surname" token must not be returned as description.
	text := `E WASKO and Hon. Some other context here. YEAS - 20`
	matchStart := strings.Index(text, "YEAS")
	got := newBrunswickDescriptionFromContext(text, matchStart)
	if strings.HasPrefix(got, "E ") {
		t.Errorf("got=%q; voter initial+surname leak should be filtered", got)
	}
}

func TestNewBrunswickDescriptionFromContext_ManitobaBudgetVote(t *testing.T) {
	// MB journals include budget votes with "budgetary policy" language.
	text := `That this House approves in general the budgetary policy of the government having been read. AYE`
	matchStart := strings.Index(text, "AYE")
	got := newBrunswickDescriptionFromContext(text, matchStart)
	if got != "Budget Vote" {
		t.Errorf("got=%q, want %q", got, "Budget Vote")
	}
}
