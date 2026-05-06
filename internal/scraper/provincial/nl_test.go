package provincial

import "testing"

func TestParseNLJournalDivisions_OutcomeOnly(t *testing.T) {
	text := `The house considered Bill 3. On the motion that the bill be read a third time, the question was put, and the motion was agreed to. On the amendment to the bill, the question was put, and the amendment was defeated.`
	divs := ParseNLJournalDivisionsForTest(text, "https://example.com/26-04-14.pdf", 51, 1, 1, "2026-04-14")
	if len(divs) == 0 {
		t.Fatal("expected at least one division")
	}
	for _, d := range divs {
		if d.Division.Result != "Carried" && d.Division.Result != "Negatived" {
			t.Fatalf("unexpected result: %q", d.Division.Result)
		}
		if len(d.Votes) != 0 {
			t.Fatalf("expected no member votes for NL outcome-only, got %d", len(d.Votes))
		}
	}
}

func TestParseNLJournalDivisions_AyesNaysWithoutCounts(t *testing.T) {
	text := `The Speaker put the question. The House divided. AYES NAYS A. Furey T. Wakeham J. Hogan B. Petten L. Dempster L. Parrott J. Haggie P. Dinn G. Byrne H. Conway Ottenheimer The Speaker declared the motion carried.`
	divs := ParseNLJournalDivisionsForTest(text, "https://example.com/24-05-02.pdf", 50, 2, 1, "2024-05-02")
	if len(divs) != 1 {
		t.Fatalf("expected 1 division, got %d", len(divs))
	}
	if divs[0].Division.Yeas == 0 || divs[0].Division.Nays == 0 {
		t.Fatalf("expected non-zero yeas and nays, got %d/%d", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
	if len(divs[0].Votes) == 0 {
		t.Fatal("expected member votes to be parsed")
	}
	found := false
	for _, mv := range divs[0].Votes {
		if mv.MemberName == "A. Furey" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected A. Furey in parsed votes")
	}
}
