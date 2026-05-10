// Package templates provides templ components and helper functions for Open Docket.
package templates

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/philspins/opendocket/internal/store"
)

type PartyStyleRule struct {
	Match string `json:"match"`
	Style string `json:"style"`
}

type PartyThemeConfig struct {
	FederalDefaultParty    string            `json:"federal_default_party"`
	DefaultStyle           string            `json:"default_style"`
	ProvinceFallbackParty  string            `json:"province_fallback_party"`
	PartyStyleRules        []PartyStyleRule  `json:"party_style_rules"`
	ProvinceGoverningParty map[string]string `json:"province_governing_party"`
}

var (
	partyThemeOnce sync.Once
	partyThemeCfg  PartyThemeConfig
)

const MemberVotesPerPage = 10

func loadPartyTheme() PartyThemeConfig {
	partyThemeOnce.Do(func() {
		cfg := defaultPartyThemeConfig()
		path := os.Getenv("PARTY_THEME_FILE")
		if strings.TrimSpace(path) == "" {
			path = "config/party-theme.json"
		}
		if b, err := os.ReadFile(path); err == nil {
			var fileCfg PartyThemeConfig
			if json.Unmarshal(b, &fileCfg) == nil {
				cfg = mergePartyThemeConfig(cfg, fileCfg)
			}
		}
		partyThemeCfg = cfg
	})
	return partyThemeCfg
}

func mergePartyThemeConfig(base, override PartyThemeConfig) PartyThemeConfig {
	out := base
	if strings.TrimSpace(override.FederalDefaultParty) != "" {
		out.FederalDefaultParty = override.FederalDefaultParty
	}
	if strings.TrimSpace(override.DefaultStyle) != "" {
		out.DefaultStyle = override.DefaultStyle
	}
	if strings.TrimSpace(override.ProvinceFallbackParty) != "" {
		out.ProvinceFallbackParty = override.ProvinceFallbackParty
	}
	if len(override.PartyStyleRules) > 0 {
		out.PartyStyleRules = override.PartyStyleRules
	}
	if len(override.ProvinceGoverningParty) > 0 {
		if out.ProvinceGoverningParty == nil {
			out.ProvinceGoverningParty = map[string]string{}
		}
		for k, v := range override.ProvinceGoverningParty {
			out.ProvinceGoverningParty[strings.ToUpper(strings.TrimSpace(k))] = v
		}
	}
	return out
}

func defaultPartyThemeConfig() PartyThemeConfig {
	return PartyThemeConfig{
		FederalDefaultParty:   "Liberal",
		DefaultStyle:          "background:linear-gradient(90deg,#d4dde7,#b8c6d6);color:#1f3346",
		ProvinceFallbackParty: "Government Party",
		PartyStyleRules: []PartyStyleRule{
			{Match: "progressive conservative", Style: "background:linear-gradient(90deg,#4f8ff0,#3d74c1);color:#082348"},
			{Match: "united conservative", Style: "background:linear-gradient(90deg,#3f7fdd,#2e63b3);color:#071d3c"},
			{Match: "conservative", Style: "background:linear-gradient(90deg,#4c8fe9,#3f77c8);color:#082348"},
			{Match: "liberal", Style: "background:linear-gradient(90deg,#ef7d7d,#db5353);color:#4b0f0f"},
			{Match: "ndp", Style: "background:linear-gradient(90deg,#f4b060,#e79335);color:#4b2a08"},
			{Match: "new democrat", Style: "background:linear-gradient(90deg,#f4b060,#e79335);color:#4b2a08"},
			{Match: "green", Style: "background:linear-gradient(90deg,#92cc7e,#65ad4b);color:#16360d"},
			{Match: "bloc", Style: "background:linear-gradient(90deg,#8dc9f4,#59a7dd);color:#0f3252"},
			{Match: "coalition avenir quebec", Style: "background:linear-gradient(90deg,#79b7e6,#4f8fcd);color:#0d2b45"},
			{Match: "saskatchewan party", Style: "background:linear-gradient(90deg,#69b45f,#4a9141);color:#11330d"},
			{Match: "consensus government", Style: "background:linear-gradient(90deg,#189491,#7f96ad);color:#1e3248"},
		},
		ProvinceGoverningParty: map[string]string{
			"AB":                        "United Conservative",
			"ALBERTA":                   "United Conservative",
			"BC":                        "New Democratic",
			"BRITISH COLUMBIA":          "New Democratic",
			"MB":                        "New Democratic",
			"MANITOBA":                  "New Democratic",
			"NB":                        "Progressive Conservative",
			"NEW BRUNSWICK":             "Progressive Conservative",
			"NL":                        "Liberal",
			"NEWFOUNDLAND AND LABRADOR": "Liberal",
			"NS":                        "Progressive Conservative",
			"NOVA SCOTIA":               "Progressive Conservative",
			"NT":                        "Consensus Government",
			"NORTHWEST TERRITORIES":     "Consensus Government",
			"NU":                        "Consensus Government",
			"NUNAVUT":                   "Consensus Government",
			"ON":                        "Progressive Conservative",
			"ONTARIO":                   "Progressive Conservative",
			"PE":                        "Progressive Conservative",
			"PRINCE EDWARD ISLAND":      "Progressive Conservative",
			"QC":                        "Coalition Avenir Quebec",
			"QUEBEC":                    "Coalition Avenir Quebec",
			"SK":                        "Saskatchewan Party",
			"SASKATCHEWAN":              "Saskatchewan Party",
			"YT":                        "Liberal",
			"YUKON":                     "Liberal",
		},
	}
}

func defaultFederalParty() string {
	return loadPartyTheme().FederalDefaultParty
}

func partyBannerStyle(party string) string {
	cfg := loadPartyTheme()
	low := strings.ToLower(party)
	for _, rule := range cfg.PartyStyleRules {
		if strings.Contains(low, strings.ToLower(rule.Match)) {
			return rule.Style
		}
	}
	return cfg.DefaultStyle
}

func provinceGoverningParty(province string) string {
	cfg := loadPartyTheme()
	key := strings.ToUpper(strings.TrimSpace(province))
	if p, ok := cfg.ProvinceGoverningParty[key]; ok {
		return p
	}
	return cfg.ProvinceFallbackParty
}

// StageEntry pairs a stage key with its human-readable label.
type StageEntry struct {
	Key   string
	Label string
}

// StageOrder defines the canonical bill-progress order shown in the progress bar.
var StageOrder = []StageEntry{
	{Key: "1st_reading", Label: "1st"},
	{Key: "2nd_reading", Label: "2nd"},
	{Key: "committee", Label: "Cmte"},
	{Key: "report_stage", Label: "Report"},
	{Key: "3rd_reading", Label: "3rd"},
	{Key: "senate", Label: "Senate"},
	{Key: "royal_assent", Label: "Assent"},
}

// Stages is the ordered list of stage keys for filter dropdowns.
var Stages = func() []string {
	keys := make([]string, len(StageOrder))
	for i, s := range StageOrder {
		keys[i] = s.Key
	}
	return keys
}()

// Categories is the list of known bill categories.
var Categories = []string{
	"Budget", "Criminal Justice", "Environment", "Health",
	"Housing", "Immigration", "Indigenous", "Infrastructure",
	"Justice", "Labour", "National Security", "Social Policy",
	"Trade", "Veterans",
}

// StageLabel returns a human-readable label for a stage key.
func StageLabel(key string) string {
	for _, s := range StageOrder {
		if s.Key == key {
			return s.Label + " Reading"
		}
	}
	// Fallback: replace underscores with spaces and capitalise each word.
	words := strings.Fields(strings.ReplaceAll(key, "_", " "))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// StageIndexOf returns the 0-based index of a stage in StageOrder, or -1 if not found.
func StageIndexOf(key string) int {
	for i, s := range StageOrder {
		if s.Key == key {
			return i
		}
	}
	return -1
}

// FormatDate converts an ISO date string (2006-01-02) to a short readable form.
func FormatDate(d string) string {
	if d == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.Format("Jan 2, 2006")
}

// ShortOrFullTitle returns the short title if set, otherwise the full title.
func ShortOrFullTitle(b store.BillRow) string {
	if b.ShortTitle != "" {
		return b.ShortTitle
	}
	return b.Title
}

// IsProvincialBill returns true when the bill is sourced from a provincial legislature.
func IsProvincialBill(b store.BillRow) bool {
	chamber := strings.ToLower(strings.TrimSpace(b.Chamber))
	if chamber == "commons" || chamber == "senate" {
		return false
	}
	id := strings.ToLower(strings.TrimSpace(b.ID))
	if strings.HasPrefix(id, "on-") ||
		strings.HasPrefix(id, "ab-") ||
		strings.HasPrefix(id, "bc-") ||
		strings.HasPrefix(id, "mb-") ||
		strings.HasPrefix(id, "nb-") ||
		strings.HasPrefix(id, "nl-") ||
		strings.HasPrefix(id, "ns-") ||
		strings.HasPrefix(id, "pe-") ||
		strings.HasPrefix(id, "qc-") ||
		strings.HasPrefix(id, "sk-") {
		return true
	}
	return false
}

// BillLevelLabel returns the jurisdiction label for bill badges.
func BillLevelLabel(b store.BillRow) string {
	if IsProvincialBill(b) {
		return "Provincial"
	}
	return "Federal"
}

// chamberDisplayNames maps raw chamber values (as stored in the DB) to human-readable names.
// Multi-word provincial chambers are stored as snake_case, which CSS capitalize cannot fix.
var chamberDisplayNames = map[string]string{
	"commons":               "House of Commons",
	"senate":                "Senate",
	"alberta":               "Alberta",
	"british_columbia":      "British Columbia",
	"manitoba":              "Manitoba",
	"new_brunswick":         "New Brunswick",
	"newfoundland_labrador": "Newfoundland and Labrador",
	"nova_scotia":           "Nova Scotia",
	"ontario":               "Ontario",
	"pei":                   "Prince Edward Island",
	"quebec":                "Quebec",
	"saskatchewan":          "Saskatchewan",
}

// ChamberLabel converts a raw chamber value to a display name.
func ChamberLabel(chamber string) string {
	if name, ok := chamberDisplayNames[chamber]; ok {
		return name
	}
	words := strings.Fields(strings.ReplaceAll(chamber, "_", " "))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// BillLevelBadgeClass returns structural/background Tailwind classes for bill badges.
func BillLevelBadgeClass(b store.BillRow) string {
	if IsProvincialBill(b) {
		return "text-xs px-2 py-0.5 bg-fuchsia-300 font-medium"
	}
	return "text-xs px-2 py-0.5 bg-lime-300 font-medium"
}

// PartyClass returns a Tailwind text-color class for a party name.
func PartyClass(party string) string {
	switch {
	case strings.Contains(party, "Liberal"):
		return "text-red-600"
	case strings.Contains(party, "Conservative"):
		return "text-blue-700"
	case strings.Contains(party, "NDP"), strings.Contains(party, "New Democrat"):
		return "text-orange-600"
	case strings.Contains(party, "Bloc"):
		return "text-sky-600"
	case strings.Contains(party, "Green"):
		return "text-green-600"
	default:
		return "text-gray-600"
	}
}

// VoteBadgeClass returns a CSS class for a vote value.
func VoteBadgeClass(vote string) string {
	switch vote {
	case "Yea":
		return "vote-yea"
	case "Nay":
		return "vote-nay"
	case "Split":
		return "vote-other"
	default:
		return "vote-other"
	}
}

func categoryBGColor(category string) string {
	colors := map[string]string{
		"Budget":            "#93c5fd",
		"Criminal Justice":  "#fca5a5",
		"Environment":       "#6ee7b7",
		"Health":            "#f9a8d4",
		"Housing":           "#fdba74",
		"Immigration":       "#c4b5fd",
		"Indigenous":        "#fcd34d",
		"Infrastructure":    "#d1d5db",
		"Justice":           "#fca5a5",
		"Labour":            "#7dd3fc",
		"National Security": "#93c5fd",
		"Social Policy":     "#c4b5fd",
		"Trade":             "#6ee7b7",
		"Veterans":          "#fcd34d",
	}
	if bg, ok := colors[category]; ok {
		return bg
	}
	return "#d1d5db"
}

// CategoryBadge returns a badge component with both background-color and text colour
// set as inline styles, bypassing Templ's style-attribute sanitiser (which drops
// everything after the first semicolon, making multi-property inline styles impossible).
func CategoryBadge(category, classes string) templ.Component {
	bg := categoryBGColor(category)
	safe := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(category)
	return templ.Raw(
		`<span class="badge-category ` + classes + `" style="background-color:` + bg + `;color:#1a1a1a">` + safe + `</span>`,
	)
}

// BillLevelBadge returns a Federal/Provincial badge with inline colour so it renders
// correctly in both light and dark mode regardless of Tailwind cascade order.
func BillLevelBadge(b store.BillRow) templ.Component {
	if IsProvincialBill(b) {
		return templ.Raw(`<span class="text-xs px-2 py-0.5 bg-fuchsia-300 font-medium" style="color:#1a1a1a">Provincial</span>`)
	}
	return templ.Raw(`<span class="text-xs px-2 py-0.5 bg-lime-300 font-medium" style="color:#1a1a1a">Federal</span>`)
}

// PageInfo holds pagination state for rendering prev/next links.
type PageInfo struct {
	Current  int
	Total    int
	HasPrev  bool
	HasNext  bool
	PrevPage int
	NextPage int
	PerPage  int
}

// ordinal returns the ordinal suffix for a number (1st, 2nd, 3rd, 4th...).
func ordinal(n int) string {
	switch n % 100 {
	case 11, 12, 13:
		return "th"
	}
	switch n % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// initial returns the first character of s, or "?" if s is empty.
func initial(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return "?"
	}
	return string(runes[0])
}

// NewPageInfo computes PageInfo from the current page, total items, and page size.
func NewPageInfo(page, total, perPage int) PageInfo {
	if perPage <= 0 {
		perPage = 10
	}
	pages := (total + perPage - 1) / perPage
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	return PageInfo{
		Current:  page,
		Total:    pages,
		HasPrev:  page > 1,
		HasNext:  page < pages,
		PrevPage: page - 1,
		NextPage: page + 1,
		PerPage:  perPage,
	}
}

// paginationPages returns page numbers to display, using -1 as an ellipsis sentinel.
// Mirrors the visiblePages logic in votes_pagination.templ JS.
func paginationPages(pi PageInfo) []int {
	total, current := pi.Total, pi.Current
	if total <= 5 {
		pages := make([]int, total)
		for i := range pages {
			pages[i] = i + 1
		}
		return pages
	}
	pages := []int{1}
	start, end := current-1, current+1
	if start < 2 {
		start = 2
		end = 4
	}
	if end > total-1 {
		end = total - 1
		start = total - 3
	}
	if start < 2 {
		start = 2
	}
	if start > 2 {
		pages = append(pages, -1)
	}
	for p := start; p <= end; p++ {
		pages = append(pages, p)
	}
	if end < total-1 {
		pages = append(pages, -1)
	}
	pages = append(pages, total)
	return pages
}

// ── Summary helpers ───────────────────────────────────────────────────────────

// ParsedSummary represents a parsed AI-generated bill summary.
type ParsedSummary struct {
	OneSentence           string   `json:"one_sentence"`
	PlainSummary          string   `json:"plain_summary"`
	KeyChanges            []string `json:"key_changes"`
	WhoIsAffected         []string `json:"who_is_affected"`
	NotableConsiderations []string `json:"notable_considerations"`
	EstimatedCost         string   `json:"estimated_cost"`
	Category              string   `json:"category"`
}

// ParseAISummary parses a JSON-encoded summary string. Returns zero value if parsing fails.
func ParseAISummary(summaryJSON string) ParsedSummary {
	if strings.TrimSpace(summaryJSON) == "" {
		return ParsedSummary{}
	}
	var result ParsedSummary
	_ = json.Unmarshal([]byte(summaryJSON), &result)
	return result
}

// truncate returns the first n characters of a string, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// HasSummary checks if a bill has an AI summary.
func HasSummary(b store.BillRow) bool {
	return strings.TrimSpace(b.SummaryAI) != ""
}

func ReactionPercent(count, total int) int {
	if total <= 0 {
		return 0
	}
	return (count * 100) / total
}

// ReactionPieChartSVG returns an inline SVG donut chart showing the proportions
// of support (green), oppose (red), and neutral (gray) reactions.
func ReactionPieChartSVG(support, oppose, neutral, total int) string {
	const (
		r  = 40.0
		cx = 50.0
		cy = 50.0
		sw = 20.0 // stroke-width (creates the donut ring)
	)
	C := 2 * math.Pi * r // circumference ≈ 251.33

	if total <= 0 {
		return `<svg viewBox="0 0 100 100" width="120" height="120" aria-hidden="true"><circle cx="50" cy="50" r="40" fill="none" stroke="#e5e7eb" stroke-width="20"/></svg>`
	}

	supLen := float64(support) / float64(total) * C
	oppLen := float64(oppose) / float64(total) * C
	neuLen := float64(neutral) / float64(total) * C

	// Each colored circle starts at a rotation derived from the cumulative
	// fraction of prior segments (−90° = top of circle).
	supRot := -90.0
	oppRot := supRot + float64(support)/float64(total)*360.0
	neuRot := oppRot + float64(oppose)/float64(total)*360.0

	return fmt.Sprintf(
		`<svg viewBox="0 0 100 100" width="120" height="120" aria-hidden="true">`+
			`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="#e5e7eb" stroke-width="%.0f"/>`+
			`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="#22c55e" stroke-width="%.0f" stroke-dasharray="%.2f %.2f" transform="rotate(%.4f 50 50)"/>`+
			`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="#ef4444" stroke-width="%.0f" stroke-dasharray="%.2f %.2f" transform="rotate(%.4f 50 50)"/>`+
			`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="#9ca3af" stroke-width="%.0f" stroke-dasharray="%.2f %.2f" transform="rotate(%.4f 50 50)"/>`+
			`</svg>`,
		cx, cy, r, sw,
		cx, cy, r, sw, supLen, C-supLen, supRot,
		cx, cy, r, sw, oppLen, C-oppLen, oppRot,
		cx, cy, r, sw, neuLen, C-neuLen, neuRot,
	)
}

// safeExternalURL validates that rawURL has an http or https scheme and returns
// templ.SafeURL(rawURL). If the scheme is not http/https (e.g. "javascript:"),
// it returns templ.SafeURL("#") to prevent XSS via unsafe URL schemes.
func safeExternalURL(rawURL string) templ.SafeURL {
	if rawURL == "" {
		return templ.SafeURL("#")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return templ.SafeURL("#")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return templ.SafeURL("#")
	}
	return templ.SafeURL(rawURL)
}

// safeMailtoURL validates an email address and returns a templ.SafeURL for a
// mailto: link. Returns templ.SafeURL("#") when the email is empty, contains
// characters that could enable RFC 2822 header injection (e.g. newlines, query
// params introduced by '?' or '&'), or does not have the shape local@domain.tld.
func safeMailtoURL(email string) templ.SafeURL {
	if email == "" {
		return templ.SafeURL("#")
	}
	// Reject characters that could inject extra headers or malform the URI.
	// Intentionally stricter than RFC 5321: quoted local-parts with spaces
	// (e.g. "john doe"@example.com) are not common in practice and the
	// additional complexity is not worth the risk for external API input.
	if strings.ContainsAny(email, "\r\n\t ?&<>\"'\\") {
		return templ.SafeURL("#")
	}
	// Must have exactly one '@' with non-empty local and domain parts.
	at := strings.Index(email, "@")
	if at <= 0 || at >= len(email)-1 || strings.Contains(email[at+1:], "@") {
		return templ.SafeURL("#")
	}
	// Domain must contain at least one dot.
	if !strings.Contains(email[at+1:], ".") {
		return templ.SafeURL("#")
	}
	return templ.SafeURL("mailto:" + email)
}

// PlacesAPIKey returns the Google Maps API key used for the Places autocomplete
// widget. It reads GOOGLE_MAPS_API_KEY from the environment so the layout can
// inject the autocomplete script on every page without extra template parameters.
func PlacesAPIKey() string {
	return strings.TrimSpace(os.Getenv("GOOGLE_MAPS_API_KEY"))
}

// GovernmentLevelBadge returns a small inline badge component for the member's
// government level ("federal" or "provincial"). Unknown values render as "".
func GovernmentLevelBadge(level string) templ.Component {
	switch strings.ToLower(level) {
	case "federal":
		return templ.Raw(`<span class="inline-block text-xs px-1.5 py-0.5 bg-lime-300 font-medium" style="color:#1a1a1a">Federal</span>`)
	case "provincial":
		return templ.Raw(`<span class="inline-block text-xs px-1.5 py-0.5 bg-fuchsia-300 font-medium" style="color:#1a1a1a">Provincial</span>`)
	default:
		return templ.Raw(``)
	}
}
