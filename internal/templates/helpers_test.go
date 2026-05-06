package templates

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/a-h/templ"
	"github.com/philspins/opendocket/internal/store"
)

func renderComponent(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf strings.Builder
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestLoadPartyTheme_NoEnvVarFallbacksSuccessfully(t *testing.T) {
	// Ensure PARTY_THEME_FILE is not set so loader must use fallback behavior.
	t.Setenv("PARTY_THEME_FILE", "")

	// Reset package-level cache so the Once fires fresh for this test.
	// We assign a new zero-value Once rather than copying the existing one
	// (copying sync.Once is unsafe per go vet / copylocks).
	partyThemeOnce = sync.Once{}
	partyThemeCfg = PartyThemeConfig{}
	t.Cleanup(func() {
		partyThemeOnce = sync.Once{}
		partyThemeCfg = PartyThemeConfig{}
	})

	cfg := loadPartyTheme()
	if cfg.FederalDefaultParty == "" {
		t.Fatalf("expected FederalDefaultParty to be populated when PARTY_THEME_FILE is unset")
	}
	if len(cfg.PartyStyleRules) == 0 {
		t.Fatalf("expected PartyStyleRules to be populated when PARTY_THEME_FILE is unset")
	}
	if len(cfg.ProvinceGoverningParty) == 0 {
		t.Fatalf("expected ProvinceGoverningParty to be populated when PARTY_THEME_FILE is unset")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"Hello", 10, "Hello"},
		{"Hello World", 5, "Hello…"},
		{"Hello World", 11, "Hello World"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d): got %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}

func TestSafeMailtoURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  templ.SafeURL
	}{
		{"empty string returns #", "", templ.SafeURL("#")},
		{"valid email", "user@example.com", templ.SafeURL("mailto:user@example.com")},
		{"valid email with subdomain", "user@mail.example.com", templ.SafeURL("mailto:user@mail.example.com")},
		{"newline injection blocked", "user@example.com\r\nBcc: evil@example.com", templ.SafeURL("#")},
		{"tab blocked", "user@example.com\t", templ.SafeURL("#")},
		{"query param injection blocked", "user@example.com?subject=spam&bcc=evil@x.com", templ.SafeURL("#")},
		{"missing @ blocked", "userexample.com", templ.SafeURL("#")},
		{"leading @ blocked", "@example.com", templ.SafeURL("#")},
		{"trailing @ blocked", "user@", templ.SafeURL("#")},
		{"multiple @ blocked", "user@host@example.com", templ.SafeURL("#")},
		{"domain without dot blocked", "user@localhost", templ.SafeURL("#")},
		{"HTML injection blocked", "user@example.com<script>", templ.SafeURL("#")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeMailtoURL(tt.input)
			if got != tt.want {
				t.Errorf("safeMailtoURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSafeExternalURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  templ.SafeURL
	}{
		{"empty string returns #", "", templ.SafeURL("#")},
		{"http URL allowed", "http://example.com", templ.SafeURL("http://example.com")},
		{"https URL allowed", "https://example.com/path?q=1", templ.SafeURL("https://example.com/path?q=1")},
		{"javascript scheme blocked", "javascript:alert(1)", templ.SafeURL("#")},
		{"data scheme blocked", "data:text/html,<h1>xss</h1>", templ.SafeURL("#")},
		{"vbscript scheme blocked", "vbscript:msgbox(1)", templ.SafeURL("#")},
		{"ftp scheme blocked", "ftp://example.com", templ.SafeURL("#")},
		{"relative URL blocked", "/relative/path", templ.SafeURL("#")},
		{"uppercase HTTPS allowed", "HTTPS://example.com", templ.SafeURL("HTTPS://example.com")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeExternalURL(tt.input)
			if got != tt.want {
				t.Errorf("safeExternalURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestChamberLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Known mappings
		{"commons", "House of Commons"},
		{"senate", "Senate"},
		{"alberta", "Alberta"},
		{"british_columbia", "British Columbia"},
		{"manitoba", "Manitoba"},
		{"new_brunswick", "New Brunswick"},
		{"newfoundland_labrador", "Newfoundland and Labrador"},
		{"nova_scotia", "Nova Scotia"},
		{"ontario", "Ontario"},
		{"pei", "Prince Edward Island"},
		{"quebec", "Quebec"},
		{"saskatchewan", "Saskatchewan"},
		// snake_case fallback
		{"prince_edward_island", "Prince Edward Island"},
		{"new_south_wales", "New South Wales"},
		// single word fallback
		{"nunavut", "Nunavut"},
		// empty string
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ChamberLabel(tt.input); got != tt.want {
				t.Errorf("ChamberLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBillLevelLabel(t *testing.T) {
	tests := []struct {
		name string
		bill store.BillRow
		want string
	}{
		{name: "federal commons", bill: store.BillRow{ID: "45-1-c-47", Chamber: "commons"}, want: "Federal"},
		{name: "federal senate", bill: store.BillRow{ID: "45-1-s-209", Chamber: "senate"}, want: "Federal"},
		{name: "provincial chamber", bill: store.BillRow{ID: "on-43-1-12", Chamber: "ontario"}, want: "Provincial"},
		{name: "provincial id fallback", bill: store.BillRow{ID: "ab-31-1-10", Chamber: ""}, want: "Provincial"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BillLevelLabel(tt.bill); got != tt.want {
				t.Fatalf("BillLevelLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── StageLabel ────────────────────────────────────────────────────────────────

func TestStageLabel_KnownKeys(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"1st_reading", "1st Reading"},
		{"2nd_reading", "2nd Reading"},
		{"committee", "Cmte Reading"},
		{"report_stage", "Report Reading"},
		{"3rd_reading", "3rd Reading"},
		{"senate", "Senate Reading"},
		{"royal_assent", "Assent Reading"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := StageLabel(tt.key); got != tt.want {
				t.Errorf("StageLabel(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestStageLabel_FallbackCapitalisesSnakeCase(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"some_other_stage", "Some Other Stage"},
		{"single", "Single"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := StageLabel(tt.key); got != tt.want {
			t.Errorf("StageLabel(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

// ── StageIndexOf ──────────────────────────────────────────────────────────────

func TestStageIndexOf(t *testing.T) {
	tests := []struct {
		key  string
		want int
	}{
		{"1st_reading", 0},
		{"2nd_reading", 1},
		{"committee", 2},
		{"report_stage", 3},
		{"3rd_reading", 4},
		{"senate", 5},
		{"royal_assent", 6},
		{"unknown", -1},
		{"", -1},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := StageIndexOf(tt.key); got != tt.want {
				t.Errorf("StageIndexOf(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

// ── BillLevelBadgeClass ───────────────────────────────────────────────────────

func TestBillLevelBadgeClass(t *testing.T) {
	provincial := store.BillRow{ID: "on-43-1-12", Chamber: "ontario"}
	federal := store.BillRow{ID: "45-1-c-47", Chamber: "commons"}

	if got := BillLevelBadgeClass(provincial); !strings.Contains(got, "fuchsia") {
		t.Errorf("BillLevelBadgeClass(provincial) = %q, expected fuchsia class", got)
	}
	if got := BillLevelBadgeClass(federal); !strings.Contains(got, "lime") {
		t.Errorf("BillLevelBadgeClass(federal) = %q, expected lime class", got)
	}
}

// ── PartyClass ────────────────────────────────────────────────────────────────

func TestPartyClass(t *testing.T) {
	tests := []struct {
		party string
		want  string
	}{
		{"Liberal", "text-red-600"},
		{"Liberal Party of Canada", "text-red-600"},
		{"Conservative", "text-blue-700"},
		{"Progressive Conservative", "text-blue-700"},
		{"NDP", "text-orange-600"},
		{"New Democrat", "text-orange-600"},
		{"Bloc Québécois", "text-sky-600"},
		{"Green Party", "text-green-600"},
		{"Independent", "text-gray-600"},
		{"", "text-gray-600"},
	}
	for _, tt := range tests {
		if got := PartyClass(tt.party); got != tt.want {
			t.Errorf("PartyClass(%q) = %q, want %q", tt.party, got, tt.want)
		}
	}
}

// ── VoteBadgeClass ────────────────────────────────────────────────────────────

func TestVoteBadgeClass(t *testing.T) {
	tests := []struct {
		vote string
		want string
	}{
		{"Yea", "vote-yea"},
		{"Nay", "vote-nay"},
		{"Paired", "vote-other"},
		{"Abstain", "vote-other"},
		{"", "vote-other"},
	}
	for _, tt := range tests {
		if got := VoteBadgeClass(tt.vote); got != tt.want {
			t.Errorf("VoteBadgeClass(%q) = %q, want %q", tt.vote, got, tt.want)
		}
	}
}

// ── ordinal ───────────────────────────────────────────────────────────────────

func TestOrdinal(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{1, "st"},
		{2, "nd"},
		{3, "rd"},
		{4, "th"},
		{11, "th"}, // teens are always "th"
		{12, "th"},
		{13, "th"},
		{21, "st"},
		{22, "nd"},
		{23, "rd"},
		{101, "st"},
		{111, "th"}, // 11x are always "th"
		{0, "th"},
	}
	for _, tt := range tests {
		if got := ordinal(tt.n); got != tt.want {
			t.Errorf("ordinal(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

// ── initial ───────────────────────────────────────────────────────────────────

func TestInitial(t *testing.T) {
	tests := []struct {
		s    string
		want string
	}{
		{"Alice", "A"},
		{"Bob Smith", "B"},
		{"", "?"},
		{"日本語", "日"}, // multi-byte rune
	}
	for _, tt := range tests {
		if got := initial(tt.s); got != tt.want {
			t.Errorf("initial(%q) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

// ── NewPageInfo ───────────────────────────────────────────────────────────────

func TestNewPageInfo(t *testing.T) {
	tests := []struct {
		name              string
		page, total, per  int
		wantCurrent       int
		wantTotal         int
		wantHasPrev       bool
		wantHasNext       bool
		wantPrevPage      int
		wantNextPage      int
	}{
		{"first page of 10", 1, 100, 10, 1, 10, false, true, 0, 2},
		{"last page of 10", 10, 100, 10, 10, 10, true, false, 9, 11},
		{"middle page", 5, 100, 10, 5, 10, true, true, 4, 6},
		{"page zero clamped to 1", 0, 100, 10, 1, 10, false, true, 0, 2},
		{"zero perPage defaults to 10", 1, 100, 0, 1, 10, false, true, 0, 2},
		{"empty result set gives one page", 1, 0, 10, 1, 1, false, false, 0, 2},
		{"single page", 1, 5, 10, 1, 1, false, false, 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pi := NewPageInfo(tt.page, tt.total, tt.per)
			if pi.Current != tt.wantCurrent {
				t.Errorf("Current = %d, want %d", pi.Current, tt.wantCurrent)
			}
			if pi.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", pi.Total, tt.wantTotal)
			}
			if pi.HasPrev != tt.wantHasPrev {
				t.Errorf("HasPrev = %v, want %v", pi.HasPrev, tt.wantHasPrev)
			}
			if pi.HasNext != tt.wantHasNext {
				t.Errorf("HasNext = %v, want %v", pi.HasNext, tt.wantHasNext)
			}
		})
	}
}

// ── paginationPages ───────────────────────────────────────────────────────────

func TestPaginationPages(t *testing.T) {
	// -1 is the ellipsis sentinel used by the template.
	const ellipsis = -1

	tests := []struct {
		name    string
		current int
		total   int
		want    []int
	}{
		{"five or fewer pages returns all", 3, 5, []int{1, 2, 3, 4, 5}},
		{"three pages returns all", 2, 3, []int{1, 2, 3}},
		{"start of long list has trailing ellipsis", 1, 20, []int{1, 2, 3, 4, ellipsis, 20}},
		{"end of long list has leading ellipsis", 20, 20, []int{1, ellipsis, 17, 18, 19, 20}},
		{"middle of long list has both ellipses", 10, 20, []int{1, ellipsis, 9, 10, 11, ellipsis, 20}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pi := PageInfo{Current: tt.current, Total: tt.total}
			got := paginationPages(pi)
			if len(got) != len(tt.want) {
				t.Fatalf("paginationPages() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("paginationPages()[%d] = %d, want %d (full: %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

// ── ParseAISummary ────────────────────────────────────────────────────────────

func TestParseAISummary(t *testing.T) {
	t.Run("valid JSON parses all fields", func(t *testing.T) {
		raw := `{"one_sentence":"Brief.","plain_summary":"Longer.","category":"Health","key_changes":["a","b"]}`
		got := ParseAISummary(raw)
		if got.OneSentence != "Brief." {
			t.Errorf("OneSentence = %q, want %q", got.OneSentence, "Brief.")
		}
		if got.Category != "Health" {
			t.Errorf("Category = %q, want %q", got.Category, "Health")
		}
		if len(got.KeyChanges) != 2 {
			t.Errorf("KeyChanges len = %d, want 2", len(got.KeyChanges))
		}
	})
	t.Run("empty string returns zero value", func(t *testing.T) {
		got := ParseAISummary("")
		if got.OneSentence != "" || got.Category != "" {
			t.Error("expected zero ParsedSummary for empty input")
		}
	})
	t.Run("whitespace-only returns zero value", func(t *testing.T) {
		got := ParseAISummary("   ")
		if got.OneSentence != "" {
			t.Error("expected zero ParsedSummary for whitespace input")
		}
	})
	t.Run("invalid JSON returns zero value", func(t *testing.T) {
		got := ParseAISummary("not-json{{{")
		if got.OneSentence != "" {
			t.Error("expected zero ParsedSummary for invalid JSON")
		}
	})
}

// ── HasSummary ────────────────────────────────────────────────────────────────

func TestHasSummary(t *testing.T) {
	if !HasSummary(store.BillRow{SummaryAI: `{"one_sentence":"x"}`}) {
		t.Error("HasSummary should be true when SummaryAI is non-empty")
	}
	if HasSummary(store.BillRow{SummaryAI: ""}) {
		t.Error("HasSummary should be false when SummaryAI is empty")
	}
	if HasSummary(store.BillRow{SummaryAI: "  "}) {
		t.Error("HasSummary should be false when SummaryAI is whitespace only")
	}
}

// ── ReactionPercent ───────────────────────────────────────────────────────────

func TestReactionPercent(t *testing.T) {
	tests := []struct {
		count, total, want int
	}{
		{50, 100, 50},
		{1, 3, 33},
		{0, 100, 0},
		{100, 100, 100},
		{0, 0, 0},  // zero total → 0 (no divide-by-zero)
		{5, 0, 0},  // zero total with non-zero count → 0
	}
	for _, tt := range tests {
		if got := ReactionPercent(tt.count, tt.total); got != tt.want {
			t.Errorf("ReactionPercent(%d, %d) = %d, want %d", tt.count, tt.total, got, tt.want)
		}
	}
}

// ── ReactionPieChartSVG ───────────────────────────────────────────────────────

func TestReactionPieChartSVG_ZeroTotal(t *testing.T) {
	svg := ReactionPieChartSVG(0, 0, 0, 0)
	if !strings.Contains(svg, "<svg") {
		t.Error("expected SVG element")
	}
	if !strings.Contains(svg, "#e5e7eb") {
		t.Error("expected gray placeholder circle for zero-total chart")
	}
}

func TestReactionPieChartSVG_NonZeroTotal(t *testing.T) {
	svg := ReactionPieChartSVG(60, 30, 10, 100)
	if !strings.Contains(svg, "#22c55e") {
		t.Error("expected green support arc")
	}
	if !strings.Contains(svg, "#ef4444") {
		t.Error("expected red oppose arc")
	}
	if !strings.Contains(svg, "#9ca3af") {
		t.Error("expected gray neutral arc")
	}
}

// ── GovernmentLevelBadge ─────────────────────────────────────────────────────

func TestGovernmentLevelBadge(t *testing.T) {
	tests := []struct {
		level    string
		contains string
	}{
		{"federal", "Federal"},
		{"Federal", "Federal"},   // case-insensitive via strings.ToLower
		{"FEDERAL", "Federal"},
		{"provincial", "Provincial"},
		{"Provincial", "Provincial"},
		{"unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			html := renderComponent(t, GovernmentLevelBadge(tt.level))
			if tt.contains != "" && !strings.Contains(html, tt.contains) {
				t.Errorf("GovernmentLevelBadge(%q) = %q, want to contain %q", tt.level, html, tt.contains)
			}
		})
	}
}

// ── mergePartyThemeConfig ─────────────────────────────────────────────────────

func TestMergePartyThemeConfig_OverrideReplaces(t *testing.T) {
	base := defaultPartyThemeConfig()
	override := PartyThemeConfig{
		FederalDefaultParty:   "Override Party",
		DefaultStyle:          "color:red",
		ProvinceFallbackParty: "Override Fallback",
	}

	merged := mergePartyThemeConfig(base, override)

	if merged.FederalDefaultParty != "Override Party" {
		t.Errorf("FederalDefaultParty = %q, want %q", merged.FederalDefaultParty, "Override Party")
	}
	if merged.DefaultStyle != "color:red" {
		t.Errorf("DefaultStyle = %q, want %q", merged.DefaultStyle, "color:red")
	}
	if merged.ProvinceFallbackParty != "Override Fallback" {
		t.Errorf("ProvinceFallbackParty = %q, want %q", merged.ProvinceFallbackParty, "Override Fallback")
	}
}

func TestMergePartyThemeConfig_WhitespaceDoesNotOverride(t *testing.T) {
	base := defaultPartyThemeConfig()
	override := PartyThemeConfig{FederalDefaultParty: "   "}

	merged := mergePartyThemeConfig(base, override)

	if merged.FederalDefaultParty != base.FederalDefaultParty {
		t.Errorf("whitespace override should not replace base FederalDefaultParty (got %q)", merged.FederalDefaultParty)
	}
}

func TestMergePartyThemeConfig_ProvinceMapMerged(t *testing.T) {
	base := defaultPartyThemeConfig()
	override := PartyThemeConfig{
		ProvinceGoverningParty: map[string]string{
			"ab": "New Alberta Party", // lowercase key — should be upcased on merge
		},
	}

	merged := mergePartyThemeConfig(base, override)

	if got := merged.ProvinceGoverningParty["AB"]; got != "New Alberta Party" {
		t.Errorf("ProvinceGoverningParty[AB] = %q, want %q", got, "New Alberta Party")
	}
	// Other provinces from base should be preserved
	if got := merged.ProvinceGoverningParty["ON"]; got == "" {
		t.Error("expected Ontario to remain in merged config")
	}
}

func TestMergePartyThemeConfig_PartyStyleRulesOverridden(t *testing.T) {
	base := defaultPartyThemeConfig()
	override := PartyThemeConfig{
		PartyStyleRules: []PartyStyleRule{{Match: "custom", Style: "color:purple"}},
	}

	merged := mergePartyThemeConfig(base, override)

	if len(merged.PartyStyleRules) != 1 || merged.PartyStyleRules[0].Match != "custom" {
		t.Errorf("PartyStyleRules not overridden correctly: %+v", merged.PartyStyleRules)
	}
}

// ── defaultFederalParty / partyBannerStyle / provinceGoverningParty ───────────

func TestDefaultFederalParty_ReturnsNonEmpty(t *testing.T) {
	got := defaultFederalParty()
	if got == "" {
		t.Error("defaultFederalParty() returned empty string")
	}
}

func TestPartyBannerStyle_KnownPartyMatchesRule(t *testing.T) {
	cfg := defaultPartyThemeConfig()
	// We need at least one rule to match "Liberal".
	if len(cfg.PartyStyleRules) == 0 {
		t.Skip("no PartyStyleRules in default config")
	}
	// Any style returned for a match should differ from an unknown party only if the
	// rule fires. We just verify it returns a non-empty string.
	got := partyBannerStyle("Liberal Party of Canada")
	if got == "" {
		t.Error("partyBannerStyle(Liberal) returned empty string")
	}
}

func TestPartyBannerStyle_UnknownReturnsDefault(t *testing.T) {
	cfg := defaultPartyThemeConfig()
	got := partyBannerStyle("Unknown Party XYZ12345")
	if got != cfg.DefaultStyle {
		t.Errorf("partyBannerStyle(unknown) = %q, want DefaultStyle %q", got, cfg.DefaultStyle)
	}
}

func TestProvinceGoverningParty_KnownProvince(t *testing.T) {
	got := provinceGoverningParty("ON")
	if got == "" {
		t.Error("provinceGoverningParty(ON) returned empty string")
	}
}

func TestProvinceGoverningParty_UnknownReturnsDefault(t *testing.T) {
	cfg := defaultPartyThemeConfig()
	got := provinceGoverningParty("ZZ")
	if got != cfg.ProvinceFallbackParty {
		t.Errorf("provinceGoverningParty(ZZ) = %q, want ProvinceFallbackParty %q", got, cfg.ProvinceFallbackParty)
	}
}

// ── categoryBGColor / CategoryBadge ──────────────────────────────────────────

func TestCategoryBGColor(t *testing.T) {
	tests := []struct {
		category string
		want     string
	}{
		{"Budget", "#93c5fd"},
		{"Health", "#f9a8d4"},
		{"Environment", "#6ee7b7"},
		{"UnknownCategory", "#d1d5db"},
		{"", "#d1d5db"},
	}
	for _, tt := range tests {
		if got := categoryBGColor(tt.category); got != tt.want {
			t.Errorf("categoryBGColor(%q) = %q, want %q", tt.category, got, tt.want)
		}
	}
}

func TestCategoryBadge_RendersSpan(t *testing.T) {
	html := renderComponent(t, CategoryBadge("Health", "extra-class"))
	if !strings.Contains(html, "Health") {
		t.Errorf("CategoryBadge output %q missing category name", html)
	}
	if !strings.Contains(html, "badge-category") {
		t.Errorf("CategoryBadge output %q missing badge-category class", html)
	}
	if !strings.Contains(html, "extra-class") {
		t.Errorf("CategoryBadge output %q missing extra classes", html)
	}
}

func TestCategoryBadge_EscapesHTML(t *testing.T) {
	html := renderComponent(t, CategoryBadge("<script>", ""))
	if strings.Contains(html, "<script>") {
		t.Errorf("CategoryBadge did not escape HTML: %q", html)
	}
}

// ── BillLevelBadge ────────────────────────────────────────────────────────────

func TestBillLevelBadge(t *testing.T) {
	federal := store.BillRow{ID: "45-1-c-47", Chamber: "commons"}
	provincial := store.BillRow{ID: "on-43-1-12", Chamber: "ontario"}

	fedHTML := renderComponent(t, BillLevelBadge(federal))
	if !strings.Contains(fedHTML, "Federal") {
		t.Errorf("BillLevelBadge(federal) = %q, expected 'Federal'", fedHTML)
	}
	if !strings.Contains(fedHTML, "lime") {
		t.Errorf("BillLevelBadge(federal) = %q, expected lime color", fedHTML)
	}

	provHTML := renderComponent(t, BillLevelBadge(provincial))
	if !strings.Contains(provHTML, "Provincial") {
		t.Errorf("BillLevelBadge(provincial) = %q, expected 'Provincial'", provHTML)
	}
	if !strings.Contains(provHTML, "fuchsia") {
		t.Errorf("BillLevelBadge(provincial) = %q, expected fuchsia color", provHTML)
	}
}

// ── FormatDate ────────────────────────────────────────────────────────────────

func TestFormatDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2024-04-03", "Apr 3, 2024"},
		{"2025-01-01", "Jan 1, 2025"},
		{"", ""},
		{"not-a-date", "not-a-date"},
	}
	for _, tt := range tests {
		if got := FormatDate(tt.input); got != tt.want {
			t.Errorf("FormatDate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── ShortOrFullTitle ──────────────────────────────────────────────────────────

func TestShortOrFullTitle(t *testing.T) {
	if got := ShortOrFullTitle(store.BillRow{ShortTitle: "Short", Title: "Full"}); got != "Short" {
		t.Errorf("ShortOrFullTitle with short = %q, want Short", got)
	}
	if got := ShortOrFullTitle(store.BillRow{Title: "Full Title"}); got != "Full Title" {
		t.Errorf("ShortOrFullTitle without short = %q, want Full Title", got)
	}
}
