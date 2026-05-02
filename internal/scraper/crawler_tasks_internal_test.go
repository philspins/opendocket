package scraper

import "testing"

func TestResolveFederalMemberIDFromCandidates_UsesRidingQualifier(t *testing.T) {
	candidates := []federalMemberCandidate{
		{ID: "mp-davies-vk", Name: "Don Davies", Riding: "Vancouver Kingsway", Active: true},
		{ID: "mp-davies-ns", Name: "Fred Davies", Riding: "Niagara South", Active: true},
	}

	got := resolveFederalMemberIDFromCandidates(candidates, "Davies (Niagara South)")
	if got != "mp-davies-ns" {
		t.Fatalf("resolveFederalMemberIDFromCandidates returned %q, want %q", got, "mp-davies-ns")
	}
}

func TestResolveFederalMemberIDFromCandidates_SurnameOnlyBestEffort(t *testing.T) {
	candidates := []federalMemberCandidate{
		{ID: "z-id", Name: "Jane Smith", Riding: "Ottawa Centre", Active: true},
		{ID: "a-id", Name: "John Smith", Riding: "Calgary Nose Hill", Active: true},
	}

	got := resolveFederalMemberIDFromCandidates(candidates, "Smith")
	if got != "a-id" {
		t.Fatalf("resolveFederalMemberIDFromCandidates returned %q, want deterministic best-effort %q", got, "a-id")
	}
}

func TestResolveFederalMemberIDFromCandidates_PrefersActiveWhenAmbiguous(t *testing.T) {
	candidates := []federalMemberCandidate{
		{ID: "inactive-id", Name: "Mark Lee", Riding: "Toronto Centre", Active: false},
		{ID: "active-id", Name: "Sarah Lee", Riding: "Toronto Centre", Active: true},
	}

	got := resolveFederalMemberIDFromCandidates(candidates, "Lee")
	if got != "active-id" {
		t.Fatalf("resolveFederalMemberIDFromCandidates returned %q, want %q", got, "active-id")
	}
}
