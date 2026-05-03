package provincial

import (
	"database/sql"
	"fmt"
	"strings"
)

type provincialMemberCandidate struct {
	ID   string
	Name string
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
		SELECT id, name FROM members
		WHERE government_level = 'provincial' AND lower(province) = lower(?)`, province)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]provincialMemberCandidate, 0)
	for rows.Next() {
		var c provincialMemberCandidate
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
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
	list, err := loadProvincialMemberCandidates(conn, province)
	if err != nil {
		return "", err
	}
	return resolveProvincialMemberIDFromCandidates(list, sourceName), nil
}

func resolveProvincialMemberIDFromCandidates(list []provincialMemberCandidate, sourceName string) string {
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
	if len(parts) >= 2 && len(parts[0]) == 1 {
		initial := parts[0]
		tail := parts[1:]
		for len(tail) > 1 && len(tail[0]) == 1 {
			tail = tail[1:]
		}

		matchedID := ""
		for _, c := range list {
			nameParts := strings.Fields(normalisePersonName(c.Name))
			if len(nameParts) < 2 {
				continue
			}
			candidateFirst := nameParts[0]
			if !strings.HasPrefix(candidateFirst, initial) {
				continue
			}
			if len(tail) > len(nameParts)-1 {
				continue
			}
			candidateTail := nameParts[len(nameParts)-len(tail):]
			ok := true
			for i := range tail {
				if candidateTail[i] != tail[i] {
					ok = false
					break
				}
			}
			if !ok {
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
