package scraper

import (
	"strings"
	"sync/atomic"
	"testing"
)

// ── federalProvincialMembersByProvinceSummary ─────────────────────────────────

func TestFederalProvincialMembersByProvinceSummary_BothEmpty(t *testing.T) {
	got := federalProvincialMembersByProvinceSummary(nil, nil)
	if got != "none" {
		t.Errorf("got %q, want none", got)
	}
}

func TestFederalProvincialMembersByProvinceSummary_FederalOnly(t *testing.T) {
	federal := []MemberProfile{
		{Province: "ON"},
		{Province: "ON"},
		{Province: "BC"},
	}
	got := federalProvincialMembersByProvinceSummary(federal, nil)
	if !strings.Contains(got, "BC federal=1") {
		t.Errorf("expected BC federal=1 in %q", got)
	}
	if !strings.Contains(got, "ON federal=2") {
		t.Errorf("expected ON federal=2 in %q", got)
	}
}

func TestFederalProvincialMembersByProvinceSummary_ProvincialOnly(t *testing.T) {
	provincial := []MemberProfile{
		{Province: "QC"},
		{Province: "QC"},
	}
	got := federalProvincialMembersByProvinceSummary(nil, provincial)
	// Format is "province federal=N provincial=N" even when federal=0.
	if !strings.Contains(got, "QC") || !strings.Contains(got, "provincial=2") {
		t.Errorf("expected QC and provincial=2 in %q", got)
	}
}

func TestFederalProvincialMembersByProvinceSummary_Mixed(t *testing.T) {
	federal := []MemberProfile{{Province: "AB"}}
	provincial := []MemberProfile{{Province: "AB"}, {Province: "AB"}}
	got := federalProvincialMembersByProvinceSummary(federal, provincial)
	if !strings.Contains(got, "AB federal=1 provincial=2") {
		t.Errorf("expected AB federal=1 provincial=2 in %q", got)
	}
}

func TestFederalProvincialMembersByProvinceSummary_BlankProvinceBecomesUnknown(t *testing.T) {
	federal := []MemberProfile{{Province: "  "}}
	got := federalProvincialMembersByProvinceSummary(federal, nil)
	if !strings.Contains(got, "Unknown federal=1") {
		t.Errorf("expected blank province to map to Unknown in %q", got)
	}
}

func TestFederalProvincialMembersByProvinceSummary_SortedAlphabetically(t *testing.T) {
	federal := []MemberProfile{{Province: "SK"}, {Province: "AB"}, {Province: "ON"}}
	got := federalProvincialMembersByProvinceSummary(federal, nil)
	abIdx := strings.Index(got, "AB")
	onIdx := strings.Index(got, "ON")
	skIdx := strings.Index(got, "SK")
	if abIdx > onIdx || onIdx > skIdx {
		t.Errorf("provinces not sorted alphabetically in %q", got)
	}
}

// ── toDBMembers ───────────────────────────────────────────────────────────────

func TestToDBMembers_EmptySlice(t *testing.T) {
	got := toDBMembers(nil)
	if len(got) != 0 {
		t.Errorf("toDBMembers(nil) = %d items, want 0", len(got))
	}
}

func TestToDBMembers_MapsAllFields(t *testing.T) {
	profiles := []MemberProfile{
		{
			ID:              "123",
			Name:            "Jane Doe",
			Party:           "Liberal",
			Riding:          "Toronto Centre",
			Province:        "ON",
			Role:            "MP",
			PhotoURL:        "https://example.com/photo.jpg",
			Email:           "jane@example.com",
			Website:         "https://jane.example.com",
			Chamber:         "commons",
			Active:          true,
			LastScraped:     "2024-01-01",
			GovernmentLevel: "federal",
		},
	}
	got := toDBMembers(profiles)
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	r := got[0]
	if r.ID != "123" || r.Name != "Jane Doe" || r.Party != "Liberal" {
		t.Errorf("basic fields not mapped: %+v", r)
	}
	if !r.Active || r.GovernmentLevel != "federal" {
		t.Errorf("Active/GovernmentLevel not mapped: %+v", r)
	}
	if r.Email != "jane@example.com" || r.Website != "https://jane.example.com" {
		t.Errorf("Email/Website not mapped: %+v", r)
	}
}

func TestToDBMembers_PreservesOrder(t *testing.T) {
	profiles := []MemberProfile{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got := toDBMembers(profiles)
	for i, id := range []string{"a", "b", "c"} {
		if got[i].ID != id {
			t.Errorf("got[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
}

// ── DefaultParallelism ────────────────────────────────────────────────────────

func TestDefaultParallelism_NoEnvReturns5(t *testing.T) {
	t.Setenv("CRAWLER_PARALLELISM", "")
	if got := DefaultParallelism(); got != 5 {
		t.Errorf("DefaultParallelism() = %d, want 5", got)
	}
}

func TestDefaultParallelism_ValidEnvReturnsValue(t *testing.T) {
	t.Setenv("CRAWLER_PARALLELISM", "12")
	if got := DefaultParallelism(); got != 12 {
		t.Errorf("DefaultParallelism() = %d, want 12", got)
	}
}

func TestDefaultParallelism_InvalidEnvFallsBack(t *testing.T) {
	t.Setenv("CRAWLER_PARALLELISM", "not-a-number")
	if got := DefaultParallelism(); got != 5 {
		t.Errorf("DefaultParallelism() = %d, want 5", got)
	}
}

func TestDefaultParallelism_ZeroFallsBack(t *testing.T) {
	t.Setenv("CRAWLER_PARALLELISM", "0")
	if got := DefaultParallelism(); got != 5 {
		t.Errorf("DefaultParallelism() = %d, want 5 for zero value", got)
	}
}

// ── RunParallel ───────────────────────────────────────────────────────────────

func TestRunParallel_ExecutesAllFunctions(t *testing.T) {
	var count atomic.Int32
	fns := make([]func(), 10)
	for i := range fns {
		fns[i] = func() { count.Add(1) }
	}
	RunParallel(3, fns)
	if got := int(count.Load()); got != 10 {
		t.Errorf("RunParallel executed %d functions, want 10", got)
	}
}

func TestRunParallel_EmptySlice(t *testing.T) {
	RunParallel(3, nil) // should not panic or block
}

func TestRunParallel_ZeroParallelismFallsBackToOne(t *testing.T) {
	var count atomic.Int32
	RunParallel(0, []func(){func() { count.Add(1) }, func() { count.Add(1) }})
	if got := int(count.Load()); got != 2 {
		t.Errorf("RunParallel(0,...) executed %d, want 2", got)
	}
}
