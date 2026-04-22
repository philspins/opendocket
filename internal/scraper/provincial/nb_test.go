package provincial

import "testing"

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
