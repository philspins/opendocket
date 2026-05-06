package provincial

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type provincialMemberCandidate struct {
	ID        string
	Name      string
	Riding    string
	TermStart string
	TermEnd   string
}

// NormalisePersonName strips titles and normalises whitespace/punctuation for name matching.
func NormalisePersonName(s string) string { return normalisePersonName(s) }

func normalisePersonName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " - ", "-")
	s = strings.ReplaceAll(s, "- ", "-")
	s = strings.ReplaceAll(s, " -", "-")
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "(", " ")
	s = strings.ReplaceAll(s, ")", " ")
	s = strings.ReplaceAll(s, "'", "")
	fields := strings.Fields(s)
	filtered := fields[:0]
	for _, field := range fields {
		switch field {
		case "hon", "mr", "mrs", "ms", "mme", "mlle", "dr", "kc", "k", "c":
			continue
		default:
			filtered = append(filtered, field)
		}
	}
	s = strings.Join(filtered, " ")
	return s
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func loadProvincialMemberCandidates(conn *sql.DB, province string) ([]provincialMemberCandidate, error) {
	rows, err := conn.Query(`
		SELECT id, name, COALESCE(riding, ''), COALESCE(term_start, ''), COALESCE(term_end, '') FROM members
		WHERE government_level = 'provincial' AND lower(province) = lower(?)`, province)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]provincialMemberCandidate, 0)
	for rows.Next() {
		var c provincialMemberCandidate
		if err := rows.Scan(&c.ID, &c.Name, &c.Riding, &c.TermStart, &c.TermEnd); err != nil {
			continue
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan provincial members: %w", err)
	}
	return list, nil
}

func resolveProvincialMemberID(conn *sql.DB, province, sourceName string) (string, error) {
	return resolveProvincialMemberIDAtDate(conn, province, sourceName, "")
}

func resolveProvincialMemberIDAtDate(conn *sql.DB, province, sourceName, divisionDate string) (string, error) {
	list, err := loadProvincialMemberCandidates(conn, province)
	if err != nil {
		return "", err
	}
	return resolveProvincialMemberIDFromCandidatesAtDate(list, sourceName, divisionDate), nil
}

func resolveProvincialMemberIDFromCandidates(list []provincialMemberCandidate, sourceName string) string {
	return resolveProvincialMemberIDFromCandidatesAtDate(list, sourceName, "")
}

func resolveProvincialMemberIDFromCandidatesAtDate(list []provincialMemberCandidate, sourceName, divisionDate string) string {
	list = temporalCandidates(list, divisionDate)

	// Handle "Surname (Riding)" format used by Ontario V&P documents when multiple
	// members share a last name (e.g. "Smith (Parry Sound—Muskoka)").
	if idx := strings.Index(sourceName, "("); idx > 0 {
		namePart := strings.TrimSpace(sourceName[:idx])
		ridingPart := ""
		if end := strings.Index(sourceName[idx:], ")"); end >= 0 {
			ridingPart = strings.TrimSpace(sourceName[idx+1 : idx+end])
		}
		wantLast := normalisePersonName(namePart)
		wantRiding := strings.ToLower(strings.TrimSpace(ridingPart))

		var matches []provincialMemberCandidate
		for _, c := range list {
			nameParts := strings.Fields(normalisePersonName(c.Name))
			if len(nameParts) > 0 && nameParts[len(nameParts)-1] == wantLast {
				matches = append(matches, c)
			}
		}
		if len(matches) == 1 {
			return matches[0].ID
		}
		if len(matches) > 1 && wantRiding != "" {
			for _, c := range matches {
				cRiding := strings.ToLower(strings.TrimSpace(c.Riding))
				if cRiding == wantRiding {
					return c.ID
				}
			}
		}
	}

	want := normalisePersonName(sourceName)
	if want == "" {
		return ""
	}

	for _, c := range list {
		if normalisePersonName(c.Name) == want {
			return c.ID
		}
	}

	parts := strings.Fields(want)

	// Hyphenated surnames like "Wong-Tam" normalise to two tokens ["wong", "tam"].
	// Try matching the tail of a candidate's normalised name against all source tokens.
	if len(parts) >= 2 && len(parts[0]) > 1 {
		for _, c := range list {
			nameParts := strings.Fields(normalisePersonName(c.Name))
			if len(nameParts) < len(parts) {
				continue
			}
			tail := nameParts[len(nameParts)-len(parts):]
			match := true
			for i, p := range parts {
				if tail[i] != p {
					match = false
					break
				}
			}
			if match {
				return c.ID
			}
		}
	}

	if len(parts) == 2 && len(parts[0]) == 1 {
		initial := parts[0]
		last := parts[1]
		matchedID := ""
		for _, c := range list {
			nameParts := strings.Fields(normalisePersonName(c.Name))
			if len(nameParts) < 2 {
				continue
			}
			candidateLast := nameParts[len(nameParts)-1]
			candidateFirst := nameParts[0]
			if candidateLast != last || !strings.HasPrefix(candidateFirst, initial) {
				continue
			}
			if matchedID != "" {
				matchedID = ""
				break
			}
			matchedID = c.ID
		}
		if matchedID != "" {
			return matchedID
		}
	}

	// Ontario and some journals list only the surname in vote lists.
	if len(parts) == 1 {
		last := parts[0]
		for _, c := range list {
			nameParts := strings.Fields(normalisePersonName(c.Name))
			if len(nameParts) > 0 && nameParts[len(nameParts)-1] == last {
				return c.ID
			}
		}
		bestID := ""
		bestScore := 0
		tie := false
		for _, c := range list {
			nameParts := strings.Fields(normalisePersonName(c.Name))
			if len(nameParts) < 2 {
				continue
			}
			score := 0
			for i := 1; i < len(nameParts); i++ {
				p := commonPrefixLen(last, nameParts[i])
				if p > score {
					score = p
				}
			}
			if score < 4 {
				continue
			}
			if score > bestScore {
				bestScore = score
				bestID = c.ID
				tie = false
			} else if score == bestScore {
				tie = true
			}
		}
		if bestID != "" && !tie {
			return bestID
		}
	}

	// OCR-heavy provincial journals (especially PEI PDFs) may merge surname with
	// riding text (e.g., "thompsoagriculture"). Fall back to a deterministic
	// first-name + surname-prefix match when it produces one clear best candidate.
	if len(parts) >= 2 {
		wantFirst := parts[0]
		wantSurnameLike := parts[1]

		bestID := ""
		bestScore := 0
		tie := false

		for _, c := range list {
			nameParts := strings.Fields(normalisePersonName(c.Name))
			if len(nameParts) < 2 || nameParts[0] != wantFirst {
				continue
			}
			score := 0
			for i := 1; i < len(nameParts); i++ {
				p := commonPrefixLen(wantSurnameLike, nameParts[i])
				if p > score {
					score = p
				}
			}
			if score < 4 {
				continue
			}
			if score > bestScore {
				bestScore = score
				bestID = c.ID
				tie = false
			} else if score == bestScore {
				tie = true
			}
		}

		if bestID != "" && !tie {
			return bestID
		}

		// Last-resort fallback: if the first name is OCR-corrupted, resolve by a
		// unique strong surname-prefix match across candidates for this province.
		bestID = ""
		bestScore = 0
		tie = false
		for _, c := range list {
			nameParts := strings.Fields(normalisePersonName(c.Name))
			if len(nameParts) < 2 {
				continue
			}
			score := 0
			for i := 1; i < len(nameParts); i++ {
				p := commonPrefixLen(wantSurnameLike, nameParts[i])
				if p > score {
					score = p
				}
			}
			if score < 4 {
				continue
			}
			if score > bestScore {
				bestScore = score
				bestID = c.ID
				tie = false
			} else if score == bestScore {
				tie = true
			}
		}
		if bestID != "" && !tie {
			return bestID
		}
	}

	return ""
}

func temporalCandidates(list []provincialMemberCandidate, divisionDate string) []provincialMemberCandidate {
	voteDate := canonicalISODate(divisionDate)
	if voteDate == "" {
		return list
	}
	filtered := make([]provincialMemberCandidate, 0, len(list))
	for _, c := range list {
		if candidateActiveOnDate(c, voteDate) {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		return list
	}
	return filtered
}

func candidateActiveOnDate(c provincialMemberCandidate, voteDate string) bool {
	start := canonicalISODate(c.TermStart)
	end := canonicalISODate(c.TermEnd)
	if start != "" && voteDate < start {
		return false
	}
	if end != "" && voteDate > end {
		return false
	}
	return true
}

func canonicalISODate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		candidate := s[:10]
		if _, err := time.Parse("2006-01-02", candidate); err == nil {
			return candidate
		}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	return ""
}
