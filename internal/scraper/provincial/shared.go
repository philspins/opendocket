package provincial

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// DivisionStub holds a row from a votes index page.
type DivisionStub struct {
	ID          string
	Parliament  int
	Session     int
	Number      int
	Date        string
	BillNumber  string
	Description string
	Yeas        int
	Nays        int
	Paired      int
	Result      string
	Chamber     string
	DetailURL   string
	LastScraped string
}

func fetchDoc(url string, client *http.Client) (*goquery.Document, error) {
	const attempts = 3
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("GET %q: %w", url, err)
			if attempt < attempts && isTransientFetchError(err) {
				time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
				continue
			}
			return nil, lastErr
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("GET %q: status %d — %s", url, resp.StatusCode, body)
		}

		doc, derr := goquery.NewDocumentFromReader(resp.Body)
		_ = resp.Body.Close()
		if derr != nil {
			lastErr = derr
			if attempt < attempts && isTransientFetchError(derr) {
				time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
				continue
			}
			return nil, derr
		}
		return doc, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("GET %q: unknown fetch error", url)
}

func isTransientFetchError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "closed network connection")
}

type legislatureSession struct {
	Legislature int
	Session     int
	Score       int
}

var parliamentSessionRe = regexp.MustCompile(`(?i)(\d{1,3})(?:st|nd|rd|th)\s*parliament[^\d]{0,40}(\d{1,2})(?:st|nd|rd|th)\s*session`)
var legislatureSessionRe = regexp.MustCompile(`(?i)(\d{1,3})(?:st|nd|rd|th)\s*(?:legislature|general assembly)[^\d]{0,40}(\d{1,2})(?:st|nd|rd|th)?\s*session`)
var parliamentSessionURLRe = regexp.MustCompile(`(?i)(\d{1,3})(?:st|nd|rd|th)?[-_/]parliament[-_/](\d{1,2})(?:st|nd|rd|th)?[-_/]session`)
var assemblySessionURLRe = regexp.MustCompile(`(?i)assembly[-_/](\d{1,3})[-_/]session[-_/](\d{1,2})(?:/|$)`)
var compactLegSessionURLRe = regexp.MustCompile(`(?i)/(\d{1,3})-(\d{1,2})(?:/|$)`)
var albertaLegislatureSessionLabelRe = regexp.MustCompile(`(?i)legislature\s*,?\s*session\s+(\d{1,3})-(\d{1,2})`)
var albertaLegislatureSessionCommaRe = regexp.MustCompile(`(?i)legislature\s+(\d{1,3})\s*,\s*session\s+(\d{1,2})`)
var albertaLegislatureSessionQueryRe = regexp.MustCompile(`(?i)[?&]legl=(\d{1,3})&session=(\d{1,2})(?:[&#]|$)`)
var manitobaLegislatureSessionPairRe = regexp.MustCompile(`(?i)\b(\d{1,3})\s*-\s*(\d{1,2})\s*\((?:\d{4}|current)`) // e.g. 43 - 3 (2025- )

func candidateScore(text string, base int) int {
	score := base
	lower := strings.ToLower(text)
	if strings.Contains(lower, "current") || strings.Contains(lower, "overview") || strings.Contains(lower, "active") {
		score += 20
	}
	if strings.Contains(lower, "latest") || strings.Contains(lower, "today") {
		score += 10
	}
	if strings.Contains(lower, "archive") || strings.Contains(lower, "archives") || strings.Contains(lower, "historical") {
		score -= 30
	}
	if strings.Contains(lower, "journal indices") || strings.Contains(lower, "appendices") {
		score -= 20
	}
	return score
}

func extractLegislatureSessionCandidates(provinceCode, text string, baseScore int) []legislatureSession {
	out := make([]legislatureSession, 0)
	for _, re := range []*regexp.Regexp{parliamentSessionRe, legislatureSessionRe, parliamentSessionURLRe, assemblySessionURLRe} {
		matches := re.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) < 3 {
				continue
			}
			l, lerr := strconv.Atoi(m[1])
			s, serr := strconv.Atoi(m[2])
			if lerr != nil || serr != nil || l <= 0 || s <= 0 {
				continue
			}
			out = append(out, legislatureSession{Legislature: l, Session: s, Score: candidateScore(text, baseScore)})
		}
	}

	if provinceCode == "ab" {
		for _, re := range []*regexp.Regexp{albertaLegislatureSessionLabelRe, albertaLegislatureSessionCommaRe, albertaLegislatureSessionQueryRe} {
			matches := re.FindAllStringSubmatch(text, -1)
			for _, m := range matches {
				if len(m) < 3 {
					continue
				}
				l, lerr := strconv.Atoi(m[1])
				s, serr := strconv.Atoi(m[2])
				if lerr != nil || serr != nil || l <= 0 || s <= 0 {
					continue
				}
				out = append(out, legislatureSession{Legislature: l, Session: s, Score: candidateScore(text, baseScore+20)})
			}
		}
	}

	if provinceCode == "mb" {
		for _, re := range []*regexp.Regexp{compactLegSessionURLRe, manitobaLegislatureSessionPairRe} {
			matches := re.FindAllStringSubmatch(text, -1)
			for _, m := range matches {
				if len(m) < 3 {
					continue
				}
				l, lerr := strconv.Atoi(m[1])
				s, serr := strconv.Atoi(m[2])
				if lerr != nil || serr != nil || l <= 0 || s <= 0 {
					continue
				}
				if l > 99 || s > 9 {
					continue
				}
				out = append(out, legislatureSession{Legislature: l, Session: s, Score: candidateScore(text, baseScore+20)})
			}
		}
	}

	if provinceCode == "qc" {
		matches := compactLegSessionURLRe.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) < 3 {
				continue
			}
			l, lerr := strconv.Atoi(m[1])
			s, serr := strconv.Atoi(m[2])
			if lerr != nil || serr != nil || l <= 0 || s <= 0 {
				continue
			}
			if l > 99 || s > 9 {
				continue
			}
			out = append(out, legislatureSession{Legislature: l, Session: s, Score: candidateScore(text, baseScore+15)})
		}
	}
	return out
}

func maxLegislatureSession(candidates []legislatureSession) (legislatureSession, bool) {
	if len(candidates) == 0 {
		return legislatureSession{}, false
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.Score > best.Score ||
			(c.Score == best.Score && c.Legislature > best.Legislature) ||
			(c.Score == best.Score && c.Legislature == best.Legislature && c.Session > best.Session) {
			best = c
		}
	}
	return best, true
}
