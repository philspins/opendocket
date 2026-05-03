// Provincial votes scrapers: shared utilities used across multiple province crawlers.
package provincial

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
)

// ── types ─────────────────────────────────────────────────────────────────────

// ProvincialMemberVote records how a single MLA voted in a provincial division.
// MemberName holds the raw display name from the source page (e.g. "Smith" for
// Ontario, "Scott Moe" for Saskatchewan); callers resolve it to a DB member ID.
type ProvincialMemberVote struct {
	DivisionID string
	MemberName string
	Vote       string // "Yea" | "Nay"
}

// ProvincialDivisionResult bundles a parsed division stub with its raw member votes.
type ProvincialDivisionResult struct {
	Division DivisionStub
	Votes    []ProvincialMemberVote
}

// ProvincialDivisionID builds a namespaced division ID for a provincial division.
// Format: "{province}-{legislature}-{session}-{date}-{num}"
// e.g. "on-44-1-2026-04-14-1"
func ProvincialDivisionID(province string, legislature, session, num int, date string) string {
	return fmt.Sprintf("%s-%d-%d-%s-%d", province, legislature, session, date, num)
}

// ── PDF extraction ────────────────────────────────────────────────────────────

func downloadAndExtractPDFText(pdfURL, province string, client *http.Client) (string, error) {
	resp, err := getWithRetry(client, pdfURL)
	if err != nil {
		return "", fmt.Errorf("GET %q: %w", pdfURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("GET %q: status %d - %s", pdfURL, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	tmp, err := os.CreateTemp("", "opendocket-"+province+"-*.pdf")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer func() { _ = tmp.Close(); _ = os.Remove(tmpPath) }()
	written, err := io.Copy(tmp, io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", err
	}
	if written >= 32<<20 {
		return "", fmt.Errorf("pdf too large (>32MB): %s", pdfURL)
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	return extractProvincialPDFText(tmpPath)
}

func extractProvincialPDFText(pdfPath string) (string, error) {
	dir, err := os.MkdirTemp("", "opendocket-nb-content-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	if err := api.ExtractContentFile(pdfPath, dir, nil, nil); err != nil {
		return "", err
	}

	contentFiles, err := filepath.Glob(filepath.Join(dir, "*_Content_page_*.txt"))
	if err != nil {
		return "", err
	}
	sort.Strings(contentFiles)

	var text strings.Builder
	for _, contentPath := range contentFiles {
		fp, err := os.Open(contentPath)
		if err != nil {
			return "", err
		}

		scanner := bufio.NewScanner(fp)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if hasPDFTextShowOperator(line) {
				for _, match := range pdfParenTextRe.FindAllStringSubmatch(line, -1) {
					if len(match) < 2 {
						continue
					}
					text.WriteString(decodePDFStringToken(match[1]))
				}
				text.WriteByte(' ')
			}
		}
		_ = fp.Close()
		if err := scanner.Err(); err != nil {
			return "", err
		}
		text.WriteByte('\f')
	}

	normalized := strings.Join(strings.Fields(strings.ReplaceAll(text.String(), "\u00a0", " ")), " ")
	return normalized, nil
}

var pdfParenTextRe = regexp.MustCompile(`\(([^()]*)\)`)

func hasPDFTextShowOperator(line string) bool {
	line = strings.TrimSpace(line)
	return strings.Contains(line, " Tj") || strings.Contains(line, " TJ") || strings.HasSuffix(line, "Tj") || strings.HasSuffix(line, "TJ")
}

func decodePDFStringToken(token string) string {
	token = strings.ReplaceAll(token, `\\(`, "(")
	token = strings.ReplaceAll(token, `\\)`, ")")
	token = strings.ReplaceAll(token, `\\n`, " ")
	token = strings.ReplaceAll(token, `\\r`, " ")
	token = strings.ReplaceAll(token, `\\t`, " ")
	token = strings.ReplaceAll(token, `\\`, "")
	return token
}

// ── URL/href utilities ────────────────────────────────────────────────────────

func normalizeHref(href string) string {
	href = strings.TrimSpace(href)
	href = strings.ReplaceAll(href, `\`, "/")
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	return href
}

var isoDateFromURLRe = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}|\d{8})`)

func extractDateFromURL(rawURL string) string {
	m := isoDateFromURLRe.FindStringSubmatch(rawURL)
	if len(m) < 2 {
		return ""
	}
	raw := m[1]
	var result string
	if len(raw) == 8 && raw[4] != '-' {
		result = fmt.Sprintf("%s-%s-%s", raw[:4], raw[4:6], raw[6:8])
	} else {
		result = raw
	}
	// Reject implausible years — opaque document IDs (e.g. "49240606") can
	// match the \d{8} pattern and produce nonsense years like 4924.
	if year, err := strconv.Atoi(result[:4]); err != nil || year < 1867 || year > 2200 {
		return ""
	}
	return result
}

// ── parliamentOrdinal ─────────────────────────────────────────────────────────

// parliamentOrdinal converts an integer to its English ordinal string (1→"1st",
// 2→"2nd", 3→"3rd", 4→"4th", …). Used to build the BC LIMS API path and by crawl.go.
func parliamentOrdinal(n int) string {
	var suffix string
	switch {
	case n%100 >= 11 && n%100 <= 13:
		suffix = "th"
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	default:
		suffix = "th"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// ── Generic votes link regexps ────────────────────────────────────────────────

var genericVotesLinkRe = regexp.MustCompile(`(?i)(votes|proceedings|journal|journals|registre-votes|recorded_votes|minutes)`)
var genericYeaRe = regexp.MustCompile(`(?i)(?:yeas?|ayes?|pour)\D*(\d+)`)
var genericNayRe = regexp.MustCompile(`(?i)(?:nays?|contre)\D*(\d+)`)

// ── Shared YEAS/AYES/NAYS parsing helpers ────────────────────────────────────

// genericYeasNaysVoteSectionRe detects the start of a YEAS/AYES vote block in PDF text.
var genericYeasNaysVoteSectionRe = regexp.MustCompile(`(?is)(?:RECORDED\s+DIVISION\s+)?(YEAS?|AYES?|POUR)\s*[-:–—]\s*\d{1,3}\s+`)

// genericYeasNaysVoteCountPairRe extracts both counts from a vote section.
var genericYeasNaysVoteCountPairRe = regexp.MustCompile(`(?is)(YEAS?|AYES?|POUR)\s*[-:–—]\s*(\d{1,3}).*?(NAYS?|CONTRE)\s*[-:–—]\s*(\d{1,3})`)

// genericNaysHalfRe marks the start of the NAYS/CONTRE block within a section.
var genericNaysHalfRe = regexp.MustCompile(`(?is)(NAYS?|CONTRE)\s*[-:–—]\s*\d{1,3}\s+`)

// genericPDFVoteCountRe is a fallback for count-only extraction.
var genericPDFVoteCountRe = regexp.MustCompile(`(?is)(?:YEAS?|AYES?|POUR)\s*[-:–—]?\s*(\d{1,3}).{0,280}?(?:NAYS?|CONTRE)\s*[-:–—]?\s*(\d{1,3})`)

// splitVoteSectionsGeneric splits normalised PDF text into per-division blocks
// using the given section start pattern.
func splitVoteSectionsGeneric(text string, sectionRe *regexp.Regexp) []string {
	idxs := sectionRe.FindAllStringIndex(text, -1)
	if len(idxs) == 0 {
		return nil
	}
	sections := make([]string, 0, len(idxs))
	for i, span := range idxs {
		start := span[0]
		end := len(text)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		section := strings.TrimSpace(text[start:end])
		if section != "" {
			sections = append(sections, section)
		}
	}
	return sections
}

// voteKeywords is the set of uppercase tokens in Canadian legislature PDF vote
// sections that are NOT member surnames. Used by extractPlainVoteNames.
var voteKeywords = map[string]bool{
	"FOR": true, "THE": true, "AMENDMENT": true, "MOTION": true, "BILL": true,
	"AGAINST": true, "QUESTION": true, "READING": true, "THIRD": true,
	"SECOND": true, "FIRST": true, "DIVISION": true, "VOTES": true,
	"PROCEEDINGS": true, "LEGISLATURE": true, "SESSION": true, "AN": true,
	"ACT": true, "TO": true, "AND": true, "OF": true, "IN": true, "ON": true,
	"OR": true, "THAT": true, "SCHEDULE": true, "CARRIED": true,
	"NEGATIVED": true, "MR": true, "MS": true, "HON": true, "DR": true,
	"YEAS": true, "YEA": true, "NAYS": true, "NAY": true, "POUR": true,
	"CONTRE": true, "AYES": true, "AYE": true, "FAVOUR": true, "OPPOSED": true,
	"MAJORITY": true, "RESOLUTION": true, "ORDER": true, "NO": true,
	"YES": true, "A": true, "AS": true, "AT": true, "BE": true, "BY": true,
	"PARLIAMENT": true, "LEGISLATIVE": true, "ASSEMBLY": true, "HOUSE": true,
	"JOURNALS": true, "JOURNAL": true, "SITTING": true,
	"RECORDED": true, "CHAIR": true, "SPEAKER": true, "DEPUTY": true,
	"MINUTES": true, "REPORT": true, "PAGE": true,
}

var voteNamePrefixTokens = map[string]bool{
	"DE": true, "DELA": true, "DEL": true, "DI": true, "DU": true,
	"LA": true, "LE": true, "MAC": true, "MC": true, "SAINT": true,
	"ST": true, "VAN": true, "VON": true,
}

var splitUppercaseNameTokenRe = regexp.MustCompile(`\b([A-Z])\s+([A-Z][A-Z][A-Z''\-]*)\b`)

var proceduralVoteTailRe = regexp.MustCompile(`(?i)\b(?:and\s+)?the\s+debate\s+being\s+ended\b.*$`)
var proceduralVoteSentenceRe = regexp.MustCompile(`(?i)\b(?:and\s+)?the\s+debate\s+being\s+ended\b|\bquestion\s+being\s+put\b|\bringing\s+of\s+the\s+bells\b|\bfollowing\s+recorded\s+division\b|\bcharles\s+iii\b`)
var substantiveVoteDescRe = regexp.MustCompile(`(?i)\b(bill|motion|amendment|resolution|act|third\s+reading|second\s+reading|first\s+reading|be\s+now\s+read|be\s+read)\b`)
var substantiveVoteClauseRe = regexp.MustCompile(`(?i)\bthat\b[^.]{0,320}\b(?:bill|motion|amendment|resolution|act)\b[^.]{0,320}`)
var billVoteClauseRe = regexp.MustCompile(`(?i)\bbill(?:\s*\(no\.\s*\d+\)|\s+no\.\s*\d+|\s+\d+[a-z]?)\b[^.]{0,240}`)
var motionVoteClauseRe = regexp.MustCompile(`(?i)\bmotion\s+\d+[a-z]?\b[^.]{0,240}`)

func compactVoteDesc(text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(text, "\u00a0", " ")), " "))
	if len(text) > 220 {
		text = strings.TrimSpace(text[:220])
	}
	return text
}

func collapseSplitUppercaseNameTokens(text string) string {
	for {
		next := splitUppercaseNameTokenRe.ReplaceAllString(text, `$1$2`)
		if next == text {
			return text
		}
		text = next
	}
}

func cleanPlainVoteToken(tok string) string {
	tok = strings.TrimRight(tok, ".,;:)'\"")
	tok = strings.TrimLeft(tok, "('\"")
	return tok
}

func isPlainVoteNameToken(tok string) bool {
	if len(tok) < 2 {
		return false
	}
	if tok[0] < 'A' || tok[0] > 'Z' {
		return false
	}
	if voteKeywords[strings.ToUpper(tok)] {
		return false
	}
	allDigit := true
	for _, c := range tok {
		if c < '0' || c > '9' {
			allDigit = false
			break
		}
	}
	return !allDigit
}

// extractPlainVoteNames extracts member surnames from a vote-block text where
// names appear as plain capitalized tokens without "Mr./Ms." prefixes (AB, MB, NS).
func extractPlainVoteNames(blockText string) []string {
	blockText = collapseSplitUppercaseNameTokens(strings.ReplaceAll(blockText, "\u00a0", " "))
	rawTokens := strings.Fields(blockText)
	seen := make(map[string]bool)
	names := make([]string, 0)
	for i := 0; i < len(rawTokens); i++ {
		tok := cleanPlainVoteToken(rawTokens[i])
		if !isPlainVoteNameToken(tok) {
			continue
		}
		if i+1 < len(rawTokens) {
			nextTok := cleanPlainVoteToken(rawTokens[i+1])
			if voteNamePrefixTokens[strings.ToUpper(tok)] && isPlainVoteNameToken(nextTok) {
				tok = tok + " " + nextTok
				i++
			}
		}
		key := strings.ToLower(tok)
		if seen[key] {
			continue
		}
		seen[key] = true
		names = append(names, tok)
	}
	return names
}

// newBrunswickDescriptionFromContext extracts a description from context text before
// a match position. Used by NB, MB, and generic parsers.
func newBrunswickDescriptionFromContext(text string, matchStart int) string {
	start := matchStart - 700
	if start < 0 {
		start = 0
	}
	snippet := strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(text[start:matchStart], "\u00a0", " ")), " "))
	if snippet == "" {
		return ""
	}

	// Keep the most recent context before the vote counts.
	if len(snippet) > 700 {
		snippet = snippet[len(snippet)-700:]
	}

	cleaned := strings.TrimSpace(proceduralVoteTailRe.ReplaceAllString(snippet, ""))

	for _, re := range []*regexp.Regexp{substantiveVoteClauseRe, billVoteClauseRe, motionVoteClauseRe} {
		matches := re.FindAllString(cleaned, -1)
		if len(matches) == 0 {
			continue
		}
		desc := compactVoteDesc(matches[len(matches)-1])
		if desc != "" {
			return desc
		}
	}

	parts := strings.Split(cleaned, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := compactVoteDesc(parts[i])
		if candidate == "" {
			continue
		}
		if proceduralVoteSentenceRe.MatchString(candidate) {
			continue
		}
		if substantiveVoteDescRe.MatchString(candidate) {
			return candidate
		}
	}

	if substantiveVoteDescRe.MatchString(cleaned) {
		return compactVoteDesc(cleaned)
	}

	return ""
}

// parsePDFDivisionsYeasNays parses recorded vote divisions from normalised PDF text
// using YEAS/AYES/NAYS/CONTRE section markers. Used by MB, NS, and NL journal parsers.
// extractNames is called on each yea/nay block to produce member names; pass nil to
// get outcome-only results (yea/nay counts with no member rows).
func parsePDFDivisionsYeasNays(
	text, detailURL, province, chamber string,
	legislature, session, startDivisionNumber int,
	date string,
	extractNames func(string) []string,
) []ProvincialDivisionResult {
	sections := splitVoteSectionsGeneric(text, genericYeasNaysVoteSectionRe)
	blocks := make([][4]string, 0, len(sections))
	for _, section := range sections {
		m := genericYeasNaysVoteCountPairRe.FindStringSubmatch(section)
		if len(m) != 5 {
			continue
		}
		yeasBlock := section
		naysBlock := ""
		if split := genericNaysHalfRe.FindStringIndex(section); split != nil {
			yeasBlock = section[:split[0]]
			naysBlock = section[split[0]:]
		}
		blocks = append(blocks, [4]string{m[2], m[4], yeasBlock, naysBlock})
	}
	if len(blocks) == 0 {
		// Fallback: count-only (no member name lists).
		matches := genericPDFVoteCountRe.FindAllStringSubmatchIndex(text, -1)
		if len(matches) == 0 {
			return nil
		}
		results := make([]ProvincialDivisionResult, 0, len(matches))
		for i, m := range matches {
			yeas, _ := strconv.Atoi(text[m[2]:m[3]])
			nays, _ := strconv.Atoi(text[m[4]:m[5]])
			if yeas == 0 && nays == 0 {
				continue
			}
			divNum := startDivisionNumber + i
			divID := ProvincialDivisionID(province, legislature, session, divNum, date)
			desc := newBrunswickDescriptionFromContext(text, m[0])
			if desc == "" {
				desc = "Recorded division"
			}
			result := "Carried"
			if nays > yeas {
				result = "Negatived"
			}
			results = append(results, ProvincialDivisionResult{
				Division: DivisionStub{
					ID: divID, Parliament: legislature, Session: session,
					Number: divNum, Date: date, Description: desc,
					Yeas: yeas, Nays: nays, Result: result,
					Chamber: chamber, DetailURL: detailURL, LastScraped: utils.NowISO(),
				},
			})
		}
		return results
	}

	results := make([]ProvincialDivisionResult, 0, len(blocks))
	for i, block := range blocks {
		yeas, _ := strconv.Atoi(strings.TrimSpace(block[0]))
		nays, _ := strconv.Atoi(strings.TrimSpace(block[1]))
		if yeas == 0 && nays == 0 {
			continue
		}
		divNum := startDivisionNumber + i
		divID := ProvincialDivisionID(province, legislature, session, divNum, date)
		desc := newBrunswickDescriptionFromContext(text, strings.Index(text, block[2]))
		if desc == "" {
			desc = "Recorded division"
		}
		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}
		votes := make([]ProvincialMemberVote, 0, yeas+nays)
		if extractNames != nil {
			for _, name := range extractNames(block[2]) {
				votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Yea"})
			}
			for _, name := range extractNames(block[3]) {
				votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Nay"})
			}
		}
		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID: divID, Parliament: legislature, Session: session,
				Number: divNum, Date: date, Description: desc,
				Yeas: yeas, Nays: nays, Result: result,
				Chamber: chamber, DetailURL: detailURL, LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
	}
	return results
}

// ParsePDFDivisionsYeasNaysForTest is test-only access to the generic YEAS/NAYS parser.
func ParsePDFDivisionsYeasNaysForTest(text, detailURL, province, chamber string, legislature, session, startDivNum int, date string) []ProvincialDivisionResult {
	return parsePDFDivisionsYeasNays(text, detailURL, province, chamber, legislature, session, startDivNum, date, extractPlainVoteNames)
}

// DownloadAndExtractPDFTextForTest exposes downloadAndExtractPDFText for tests.
func DownloadAndExtractPDFTextForTest(pdfURL, province string, client *http.Client) (string, error) {
	return downloadAndExtractPDFText(pdfURL, province, client)
}

// ── Generic crawl helpers ─────────────────────────────────────────────────────

// CrawlGenericProvincialVotes fetches a provincial votes/proceedings index page,
// discovers likely per-day links, then parses divisions from each page using
// resilient heuristics that work across multiple legislature layouts.
func CrawlGenericProvincialVotes(indexURL, provinceCode, chamber string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlGenericProvincialVotes(indexURL, provinceCode, chamber, legislature, session, client)
}

func crawlGenericProvincialVotes(indexURL, provinceCode, chamber string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlGenericProvincialVotesWithMatcher(indexURL, provinceCode, chamber, legislature, session, client, genericVotesLinkRe)
}

func crawlGenericProvincialVotesWithMatcher(indexURL, provinceCode, chamber string, legislature, session int, client *http.Client, linkMatcher *regexp.Regexp) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Infof("[%s-votes] fetching index: %s", provinceCode, indexURL)

	doc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, fmt.Errorf("%s generic index: %w", provinceCode, err)
	}

	links := discoverProvincialVoteLinksWithMatcher(doc, indexURL, linkMatcher)
	if len(links) == 0 {
		links = []string{indexURL}
	}

	results := make([]ProvincialDivisionResult, 0)
	for _, link := range links {
		dayDoc, derr := fetchDoc(link, client)
		if derr != nil {
			clog.Debugf("[%s-votes] skip day link %s: %v", provinceCode, link, derr)
			continue
		}
		date := extractDateFromURL(link)
		parsed := parseGenericProvincialVotesDoc(dayDoc, provinceCode, chamber, legislature, session, date)
		results = append(results, parsed...)
	}

	clog.Infof("[%s-votes] parsed %d divisions", provinceCode, len(results))
	return results, nil
}

func discoverProvincialVoteLinks(doc *goquery.Document, indexURL string) []string {
	return discoverProvincialVoteLinksWithMatcher(doc, indexURL, genericVotesLinkRe)
}

func discoverProvincialVoteLinksWithMatcher(doc *goquery.Document, indexURL string, matcher *regexp.Regexp) []string {
	if matcher == nil {
		matcher = genericVotesLinkRe
	}
	seen := make(map[string]bool)
	links := make([]string, 0)

	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		if href == "" {
			return
		}
		text := a.Text() + " " + href
		if !matcher.MatchString(text) {
			return
		}
		full := resolveRelativeURL(indexURL, href)
		if seen[full] {
			return
		}
		seen[full] = true
		links = append(links, full)
	})

	sort.Strings(links)
	// Keep the most recent slice for speed/safety on very large archives.
	if len(links) > 40 {
		links = links[len(links)-40:]
	}
	return links
}

func parseGenericProvincialVotesDoc(doc *goquery.Document, provinceCode, chamber string, legislature, session int, fallbackDate string) []ProvincialDivisionResult {
	results := make([]ProvincialDivisionResult, 0)
	divNum := 0

	seenFingerprint := make(map[string]bool)
	doc.Find("table, section, article, div").Each(func(_ int, node *goquery.Selection) {
		text := strings.Join(strings.Fields(strings.ReplaceAll(node.Text(), "\u00a0", " ")), " ")
		if text == "" {
			return
		}
		if !genericYeaRe.MatchString(text) || !genericNayRe.MatchString(text) {
			return
		}

		fingerprint := text
		if len(fingerprint) > 200 {
			fingerprint = fingerprint[:200]
		}
		if seenFingerprint[fingerprint] {
			return
		}
		seenFingerprint[fingerprint] = true

		yeas := firstCount(genericYeaRe, text)
		nays := firstCount(genericNayRe, text)
		if yeas == 0 && nays == 0 {
			return
		}

		divNum++
		date := fallbackDate
		if d := utils.FindDateInText(text); d != "" {
			date = d
		}
		if date == "" {
			date = utils.TodayISO()
		}

		desc := strings.TrimSpace(node.PrevAll().Filter("h1,h2,h3,h4,h5,p").First().Text())
		if desc == "" {
			desc = text
			if len(desc) > 200 {
				desc = desc[:200]
			}
		}

		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		divID := ProvincialDivisionID(provinceCode, legislature, session, divNum, date)
		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID:          divID,
				Parliament:  legislature,
				Session:     session,
				Number:      divNum,
				Date:        date,
				Description: desc,
				Yeas:        yeas,
				Nays:        nays,
				Result:      result,
				Chamber:     chamber,
				LastScraped: utils.NowISO(),
			},
			Votes: parseGenericProvincialMemberVotes(node, divID),
		})
	})

	return results
}

func parseGenericProvincialMemberVotes(node *goquery.Selection, divisionID string) []ProvincialMemberVote {
	results := make([]ProvincialMemberVote, 0)
	seen := make(map[string]bool)

	node.Find("li, td, p").Each(func(_ int, s *goquery.Selection) {
		name := strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(s.Text(), "\u00a0", " ")), " "))
		if name == "" || len(name) < 3 {
			return
		}
		upper := strings.ToUpper(name)
		if strings.Contains(upper, "YEA") || strings.Contains(upper, "NAY") || strings.Contains(upper, "POUR") || strings.Contains(upper, "CONTRE") {
			return
		}

		context := strings.ToUpper(strings.Join(strings.Fields(strings.ReplaceAll(s.Parent().Text(), "\u00a0", " ")), " "))
		vote := ""
		switch {
		case strings.Contains(context, "YEA"), strings.Contains(context, "AYE"), strings.Contains(context, "POUR"):
			vote = "Yea"
		case strings.Contains(context, "NAY"), strings.Contains(context, "CONTRE"):
			vote = "Nay"
		default:
			return
		}

		key := vote + "|" + strings.ToLower(name)
		if seen[key] {
			return
		}
		seen[key] = true
		results = append(results, ProvincialMemberVote{DivisionID: divisionID, MemberName: name, Vote: vote})
	})

	return results
}

func firstCount(re *regexp.Regexp, text string) int {
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
