package server

import (
	"testing"

	"github.com/philspins/opendocket/internal/store"
)

func TestDedupeBillDetailDivisions_RemovesDuplicateDateDescription(t *testing.T) {
	in := []store.DivisionRow{
		{ID: "b-1", Date: "2026-04-28", Description: "Clause 1 of Bill (No. 16) passed.", Yeas: 5, Nays: 4},
		{ID: "b-2", Date: "2026-04-28", Description: "Clause 1 of Bill (No. 16) passed.", Yeas: 5, Nays: 5},
		{ID: "b-3", Date: "2026-04-27", Description: "Second reading carried.", Yeas: 6, Nays: 3},
	}

	out := dedupeBillDetailDivisions(in)
	if len(out) != 2 {
		t.Fatalf("len(out)=%d want 2", len(out))
	}
	if out[0].ID != "b-1" {
		t.Fatalf("expected first occurrence kept for duplicate key, got %q", out[0].ID)
	}
	if out[1].ID != "b-3" {
		t.Fatalf("expected non-duplicate row retained, got %q", out[1].ID)
	}
}

func TestDedupeBillDetailDivisions_NormalizesDescriptionWhitespaceAndCase(t *testing.T) {
	in := []store.DivisionRow{
		{ID: "d-1", Date: "2026-05-01", Description: "Motion on Clause 2"},
		{ID: "d-2", Date: "2026-05-01", Description: "  motion   on   clause 2  "},
	}

	out := dedupeBillDetailDivisions(in)
	if len(out) != 1 {
		t.Fatalf("len(out)=%d want 1", len(out))
	}
	if out[0].ID != "d-1" {
		t.Fatalf("expected first row kept, got %q", out[0].ID)
	}
}
