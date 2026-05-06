package templates

import (
	"strings"
	"testing"

	"github.com/philspins/opendocket/internal/store"
)

// ── billIsSubscribed ──────────────────────────────────────────────────────────

func TestBillIsSubscribed(t *testing.T) {
	ids := []string{"45-1-c-47", "45-1-s-5", "44-1-c-11"}

	tests := []struct {
		billID string
		want   bool
	}{
		{"45-1-c-47", true},
		{"45-1-s-5", true},
		{"44-1-c-11", true},
		{"45-1-c-99", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := billIsSubscribed(tt.billID, ids); got != tt.want {
			t.Errorf("billIsSubscribed(%q) = %v, want %v", tt.billID, got, tt.want)
		}
	}
}

func TestBillIsSubscribed_EmptyList(t *testing.T) {
	if billIsSubscribed("45-1-c-47", nil) {
		t.Error("billIsSubscribed with nil list should return false")
	}
}

// ── billPageParams ────────────────────────────────────────────────────────────

func TestBillPageParams(t *testing.T) {
	tests := []struct {
		name   string
		filter store.BillFilter
		want   string
	}{
		{"empty filter returns empty string", store.BillFilter{}, ""},
		{"search only", store.BillFilter{Search: "healthcare"}, "&q=healthcare"},
		{"stage only", store.BillFilter{Stage: "2nd_reading"}, "&stage=2nd_reading"},
		{"category only", store.BillFilter{Category: "Health"}, "&category=Health"},
		{"chamber only", store.BillFilter{Chamber: "commons"}, "&chamber=commons"},
		{"level only", store.BillFilter{Level: "federal"}, "&level=federal"},
		{"province only", store.BillFilter{Province: "ON"}, "&province=ON"},
		{"sort only", store.BillFilter{Sort: "date_asc"}, "&sort=date_asc"},
		{
			"multiple fields concatenated",
			store.BillFilter{Search: "tax", Stage: "3rd_reading", Category: "Budget"},
			"&q=tax&stage=3rd_reading&category=Budget",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := billPageParams(tt.filter); got != tt.want {
				t.Errorf("billPageParams() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── billsQueryString ──────────────────────────────────────────────────────────

func TestBillsQueryString_AlwaysIncludesPerPage(t *testing.T) {
	f := store.BillFilter{PerPage: 25}
	got := billsQueryString(f)
	if !strings.HasPrefix(got, "per_page=25") {
		t.Errorf("billsQueryString() = %q, should start with per_page=25", got)
	}
}

func TestBillsQueryString_IncludesOptionalFilters(t *testing.T) {
	f := store.BillFilter{PerPage: 10, Search: "health", Stage: "senate", Sort: "date_asc"}
	got := billsQueryString(f)

	for _, want := range []string{"per_page=10", "q=health", "stage=senate", "sort=date_asc"} {
		if !strings.Contains(got, want) {
			t.Errorf("billsQueryString() = %q, missing %q", got, want)
		}
	}
}

// ── billsURLWithSort ──────────────────────────────────────────────────────────

func TestBillsURLWithSort(t *testing.T) {
	f := store.BillFilter{PerPage: 10, Search: "climate", Page: 3}
	got := billsURLWithSort(f, "date_asc")

	if !strings.HasPrefix(got, "/bills?") {
		t.Errorf("billsURLWithSort() = %q, should start with /bills?", got)
	}
	if !strings.Contains(got, "sort=date_asc") {
		t.Errorf("billsURLWithSort() = %q, should contain sort=date_asc", got)
	}
	if !strings.Contains(got, "q=climate") {
		t.Errorf("billsURLWithSort() = %q, should preserve search filter", got)
	}
}

// ── billsURLWithout ───────────────────────────────────────────────────────────

func TestBillsURLWithout(t *testing.T) {
	base := store.BillFilter{
		PerPage:  10,
		Search:   "foo",
		Stage:    "2nd_reading",
		Category: "Health",
		Chamber:  "commons",
		Level:    "federal",
		Province: "ON",
	}

	tests := []struct {
		field   string
		removed string // substring that must be absent from the result
	}{
		{"q", "q=foo"},
		{"stage", "stage=2nd_reading"},
		{"category", "category=Health"},
		{"chamber", "chamber=commons"},
		{"level", "level=federal"},
		{"province", "province=ON"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got := billsURLWithout(base, tt.field)
			if !strings.HasPrefix(got, "/bills?") {
				t.Errorf("billsURLWithout() = %q, should start with /bills?", got)
			}
			if tt.removed != "" && strings.Contains(got, tt.removed) {
				t.Errorf("billsURLWithout(%q): result %q still contains %q", tt.field, got, tt.removed)
			}
		})
	}
}

// ── billChamberLabel ──────────────────────────────────────────────────────────

func TestBillChamberLabel(t *testing.T) {
	tests := []struct {
		chamber string
		want    string
	}{
		{"commons", "House"},
		{"senate", "Senate"},
		{"ontario", "ontario"}, // unknown values pass through
		{"", ""},
	}
	for _, tt := range tests {
		if got := billChamberLabel(tt.chamber); got != tt.want {
			t.Errorf("billChamberLabel(%q) = %q, want %q", tt.chamber, got, tt.want)
		}
	}
}

// ── billLevelLabel ────────────────────────────────────────────────────────────

func TestBillLevelLabelHelper(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"federal", "Federal"},
		{"provincial", "Provincial"},
		{"municipal", "municipal"}, // unknown passes through
		{"", ""},
	}
	for _, tt := range tests {
		if got := billLevelLabel(tt.level); got != tt.want {
			t.Errorf("billLevelLabel(%q) = %q, want %q", tt.level, got, tt.want)
		}
	}
}
