package server

import (
	"strings"

	"github.com/philspins/opendocket/internal/store"
)

// dedupeBillDetailDivisions removes duplicate DivisionRows where "duplicate"
// means same date and same description (case-folded and whitespace-normalized).
// The first occurrence is kept.
func dedupeBillDetailDivisions(divs []store.DivisionRow) []store.DivisionRow {
	seen := make(map[string]bool, len(divs))
	out := make([]store.DivisionRow, 0, len(divs))
	for _, d := range divs {
		key := d.Date + "\x00" + normalizeDesc(d.Description)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d)
	}
	return out
}

// normalizeDesc folds whitespace and lowercases a description string so that
// visually-identical descriptions compare equal regardless of extra spacing or
// capitalisation differences.
func normalizeDesc(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
