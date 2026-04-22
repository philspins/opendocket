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
