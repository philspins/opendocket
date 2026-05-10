// Provincial votes scrapers: shared utilities used across multiple province crawlers.
package provincial

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
)

var errNonPDFResponse = errors.New("non-pdf response")

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
	const attempts = 3
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		text, err := downloadAndExtractPDFTextOnce(pdfURL, province, client)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if attempt < attempts && isTransientPDFError(err) {
			time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
			continue
		}
		return "", err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("GET %q: unknown PDF extraction error", pdfURL)
}

func downloadAndExtractPDFTextOnce(pdfURL, province string, client *http.Client) (string, error) {
	resp, err := client.Get(pdfURL)
	if err != nil {
		return "", fmt.Errorf("GET %q: %w", pdfURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("GET %q: status %d - %s", pdfURL, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	peek := make([]byte, 5)
	n, _ := io.ReadFull(resp.Body, peek)
	peek = peek[:n]
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	isPDFHeader := len(peek) >= 5 && string(peek[:5]) == "%PDF-"
	if !isPDFHeader && strings.Contains(contentType, "text/html") {
		return "", fmt.Errorf("%w: GET %q redirected to HTML (status %d, content-type %q)", errNonPDFResponse, pdfURL, resp.StatusCode, contentType)
	}
	reader := io.MultiReader(strings.NewReader(string(peek)), resp.Body)
	tmp, err := os.CreateTemp("", "opendocket-"+province+"-*.pdf")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer func() { _ = tmp.Close(); _ = os.Remove(tmpPath) }()
	written, err := io.Copy(tmp, io.LimitReader(reader, 32<<20))
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

func isTransientPDFError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "no header version available") ||
		strings.Contains(msg, "xreftable failed") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset")
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
	if len(raw) == 8 && raw[4] != '-' {
		raw = fmt.Sprintf("%s-%s-%s", raw[:4], raw[4:6], raw[6:8])
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return ""
	}
	year := parsed.Year()
	if year < 1900 || year > time.Now().UTC().Year()+1 {
		return ""
	}
	return parsed.Format("2006-01-02")
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
	// Month and day names that appear in AB/MB V&P PDF text blocks.
	"JANUARY": true, "FEBRUARY": true, "MARCH": true, "APRIL": true,
	"MAY": true, "JUNE": true, "JULY": true, "AUGUST": true,
	"SEPTEMBER": true, "OCTOBER": true, "NOVEMBER": true, "DECEMBER": true,
	"MONDAY": true, "TUESDAY": true, "WEDNESDAY": true, "THURSDAY": true, "FRIDAY": true,
	// Parliamentary and government titles/abbreviations.
	"MLA": true, "MLC": true, "PREMIER": true, "MINISTER": true, "MEMBER": true,
	"GOVERNMENT": true, "OPPOSITION": true, "LEADER": true,
	// Alberta-specific boilerplate that bleeds into name blocks from PDF extraction.
	"APPROPRIATION": true, "SUPPLEMENTARY": true, "FISCAL": true,
	"BUDGET": true, "ESTIMATES": true, "SUPPLY": true, "LOAN": true,
	"ALBERTA": true, "PROVINCE": true, "PROVINCIAL": true,
	// Parliamentary procedural words that appear after the name list in AB/MB PDFs.
	"COMMITTEE": true, "COMMITTEES": true, "WHOLE": true, "BACK": true, "WITH": true,
	"TABLE": true, "TABLING": true, "TABLINGS": true, "PROGRESS": true,
	"ORDERS": true, "DAY": true, "MOTIONS": true, "MOVED": true,
	"PURSUANT": true, "STANDING": true, "ACCORDING": true, "CERTAIN": true,
	"PAPER": true, "BILLS": true, "PASSED": true, "REJECTED": true,
	"REFERRED": true, "TABLED": true, "DEFERRED": true, "DEFEATED": true,
	"AGREED": true, "SESSIONAL": true,
	// Debate/procedural flow words.
	"ADJOURNMENT": true, "DEBATE": true, "ORAL": true, "ROUTINE": true,
	"STATEMENTS": true, "URGENT": true, "WRITTEN": true, "DAILY": true,
	"FALL": true, "THRONE": true, "ADDRESS": true,
	"NOTICES": true, "QUESTIONS": true, "PETITIONS": true, "REPLIES": true,
	"DIVISIONS": true, "INTRODUCTION": true,
	// Officers/titles that appear in AB V&P PDF bodies.
	"AUDITOR": true, "CLERK": true, "LIEUTENANT": true, "GOVERNOR": true,
	"GAZETTE": true, "PRESIDENT": true, "TREASURER": true, "BOARD": true,
	"BUREAU": true, "COMMISSION": true, "COUNCIL": true, "OMBUDSMAN": true,
	// Government/ministry subject nouns.
	"ANNUAL": true, "SPECIAL": true, "FEDERAL": true, "MUNICIPAL": true,
	"NATIONAL": true, "REGIONAL": true, "SOCIAL": true, "HEALTH": true,
	"EDUCATION": true, "LABOUR": true, "JUSTICE": true, "PUBLIC": true,
	"GENERAL": true, "REVIEW": true, "GOVERNANCE": true, "AUDIT": true,
	"SERVICES": true, "AFFAIRS": true, "FINANCE": true, "TREASURY": true,
	"UTILITIES": true, "INFRASTRUCTURE": true, "TOURISM": true, "ENERGY": true,
	"ENVIRONMENT": true, "AGRICULTURE": true, "FAMILIES": true, "CHILDREN": true,
	"SENIORS": true, "MENTAL": true, "COMMUNITY": true, "HOUSING": true,
	"INDIGENOUS": true, "PARKS": true, "FORESTRY": true,
	// Direction/geographic words that appear in AB riding names but not as surnames.
	"NORTH": true, "SOUTH": true, "EAST": true, "WEST": true,
	"CENTRAL": true, "CENTRE": true, "RURAL": true,
	// Common English words observed as AB V&P noise.
	"TITLE": true, "SPEECH": true, "REPLY": true,
	"PRIVATE": true, "INTERGOVERNMENTAL": true, "INTERSESSIONAL": true,
	"MEASURES": true, "DEPOSITS": true, "STATUTES": true,
}

// abRidingPrefixSet is the set of Alberta city/region names that begin
// hyphenated riding names (e.g. "Calgary-North", "Edmonton-Riverview").
// These tokens are never valid member surnames.
var abRidingPrefixSet = map[string]bool{
	"CALGARY": true, "EDMONTON": true, "AIRDRIE": true, "CHESTERMERE": true,
	"COCHRANE": true, "BONNYVILLE": true, "BANFF": true, "CAMROSE": true,
	"LETHBRIDGE": true, "CYPRESS": true, "LIVINGSTONE": true,
	"STRATHMORE": true, "OLDS": true, "LACOMBE": true,
	"SHERWOOD": true, "INNISFAIL": true, "STETTLER": true,
	"VEGREVILLE": true, "WAINWRIGHT": true, "VERMILION": true,
	"CARDSTON": true, "PINCHER": true, "CROWFOOT": true,
	"HIGHWOOD": true, "MACLEOD": true,
}

var voteNamePrefixTokens = map[string]bool{
	"DE": true, "DELA": true, "DEL": true, "DI": true, "DU": true,
	"LA": true, "LE": true, "MAC": true, "MC": true, "SAINT": true,
	"ST": true, "VAN": true, "VON": true,
}

var newBrunswickDescRecordedDivisionTailRe = regexp.MustCompile(`(?is)\bRECORDED\s+DIVISION\b.*$`)
var newBrunswickDescBoilerplateRe = regexp.MustCompile(`(?is)\b(?:and\s+the\s+debate\s+being\s+ended|the\s+debate\s+being\s+ended|and\s+the\s+debate\s+continuing|and\s+the\s+question\s+being\s+put|the\s+question\s+being\s+put|leave\s+was\s+granted\s+to\s+dispense\s+with\s+the\s+ten\s*-?\s*minute\s+time\s+allotted\s+for\s+the\s+ringing\s+of\s+the\s+bells|on\s+the\s+following\s+(?:recorded\s+)?division|it\s+was\s+(?:agreed|negatived)\s+to|speaker\s+resumed\s+the\s+chair|having\s+spoken|moved\s+by\s+(?:hon|mr|ms|mrs))\b`)

// billNoInContextRe matches "Bill No. X" or "Bill X" in provincial journal text.
var billNoInContextRe = regexp.MustCompile(`(?i)\bBill\s+(?:No\.?\s*)?(\d+\w*)`)

// ── Parliamentary motion classifier ──────────────────────────────────────────

// motionReadingRe matches "read a [first|second|third] time".
var motionReadingRe = regexp.MustCompile(`(?i)\bread\s+a\s+(first|second|third)\s+time`)

// motionNotNowReadRe matches the "be not now read a Nth time" amendment pattern
// (must be checked before motionReadingRe to avoid misclassifying amendments).
var motionNotNowReadRe = regexp.MustCompile(`(?i)\bnot\s+now\s+read\s+a\s+(first|second|third)\s+time`)

// motionForReadingRe matches "motion for Nth reading"; fo\s*r handles the common
// OCR artifact "fo r" (space inserted mid-word by some PDF extractors).
var motionForReadingRe = regexp.MustCompile(`(?i)\bmotion\s+fo\s*r\s+(first|second|third)\s+reading`)

// motionBillPassesRe matches "the said Bill does pass" and similar passage phrases.
var motionBillPassesRe = regexp.MustCompile(`(?i)\b(?:said\s+)?[Bb]ill\b.{0,40}?\bpass(?:es)?\b`)

// motionAddressReplyRe matches "Address in Reply" (throne speech response motions).
var motionAddressReplyRe = regexp.MustCompile(`(?i)\bAddress\s+in\s+Reply\b`)

// motionCommitteeWholeRe matches "Committee of the Whole".
var motionCommitteeWholeRe = regexp.MustCompile(`(?i)\bCommittee\s+of\s+the\s+Whole\b`)

// motionBudgetRe matches Manitoba budget approval votes.
var motionBudgetRe = regexp.MustCompile(`(?i)\bbudgetary\s+policy\b`)

// motionDescJournalHeaderRe matches journal date-header lines that leak into the
// context window (e.g. "Monday , November 3 , 2025 424").
var motionDescJournalHeaderRe = regexp.MustCompile(`(?i)^(?:monday|tuesday|wednesday|thursday|friday|saturday|sunday)\s*,`)

// motionDescVoterLeakRe matches "single initial + ALL_CAPS surname" voter-name
// patterns that sometimes bleed in from an adjacent division's name list.
var motionDescVoterLeakRe = regexp.MustCompile(`^[A-Z]\s+[A-Z]{2,}`)

var readingOrdinalLabels = map[string]string{
	"first": "First", "second": "Second", "third": "Third",
}

// classifyParliamentaryMotion returns a compact description for standard
// parliamentary motion types: bill readings, amendments to readings, committee
// of the whole, budget votes, and address-in-reply motions. Returns "" when no
// pattern matches and the caller should fall back to sentence extraction.
//
// Handles common PDF artifacts: backslash line-break characters (Manitoba
// journals) and OCR spacing splits such as "fo r" instead of "for".
func classifyParliamentaryMotion(snippet string) string {
	// Normalize backslash PDF line-break artifacts before pattern matching.
	snippet = strings.ReplaceAll(snippet, `\`, " ")
	snippet = strings.Join(strings.Fields(snippet), " ")

	billNum := ""
	if bm := billNoInContextRe.FindStringSubmatch(snippet); len(bm) == 2 {
		billNum = strings.ToUpper(strings.TrimSpace(bm[1]))
	}
	billSuffix := ""
	if billNum != "" {
		billSuffix = ": Bill " + billNum
	}

	lower := strings.ToLower(snippet)

	// "not now read a Nth time" → amendment to that reading.
	// Must come before motionReadingRe to avoid matching the reading itself.
	if m := motionNotNowReadRe.FindStringSubmatch(snippet); len(m) == 2 {
		ord := readingOrdinalLabels[strings.ToLower(m[1])]
		return "Amendment to " + ord + " Reading" + billSuffix
	}

	// "motion for Nth reading" → amended reading or plain reading depending on context.
	if m := motionForReadingRe.FindStringSubmatch(snippet); len(m) == 2 {
		ord := readingOrdinalLabels[strings.ToLower(m[1])]
		if strings.Contains(lower, "amend") || strings.Contains(lower, "not now") {
			return "Amendment to " + ord + " Reading" + billSuffix
		}
		return ord + " Reading" + billSuffix
	}

	// "read a Nth time" → bill reading (plain).
	if m := motionReadingRe.FindStringSubmatch(snippet); len(m) == 2 {
		ord := readingOrdinalLabels[strings.ToLower(m[1])]
		return ord + " Reading" + billSuffix
	}

	// "the said Bill does pass" → third reading / passage.
	if motionBillPassesRe.MatchString(snippet) {
		return "Third Reading" + billSuffix
	}

	// "Address in Reply" motions.
	if motionAddressReplyRe.MatchString(snippet) {
		if strings.Contains(lower, "amend") {
			return "Address in Reply: Amendment"
		}
		return "Address in Reply to the Throne Speech"
	}

	// "Committee of the Whole".
	if motionCommitteeWholeRe.MatchString(snippet) {
		return "Committee of the Whole"
	}

	// Manitoba budget approval votes.
	if motionBudgetRe.MatchString(snippet) {
		return "Budget Vote"
	}

	return ""
}

var splitUppercaseNameTokenRe = regexp.MustCompile(`\b([A-Z])\s+([A-Z][A-Z][A-Z''\-]*)\b`)

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
	tok = strings.TrimRight(tok, ".,;:)'\"\\")
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
	// Reject hyphenated tokens whose first part is a known AB riding-name city
	// (e.g. "Calgary-North", "Edmonton-Riverview"). These are never surnames.
	if idx := strings.IndexByte(tok, '-'); idx > 0 {
		if abRidingPrefixSet[strings.ToUpper(tok[:idx])] {
			return false
		}
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

// extractPlainVoteNamesN extracts member surnames from a vote-block text where
// names appear as plain capitalized tokens without "Mr./Ms." prefixes (AB, MB, NS).
// max > 0 caps the number of names returned, preventing overflow into boilerplate
// that follows the name list in AB/MB PDF documents.
func extractPlainVoteNamesN(blockText string, max int) []string {
	blockText = collapseSplitUppercaseNameTokens(strings.ReplaceAll(blockText, "\u00a0", " "))
	rawTokens := strings.Fields(blockText)
	seen := make(map[string]bool)
	names := make([]string, 0)
	for i := 0; i < len(rawTokens); i++ {
		if max > 0 && len(names) >= max {
			break
		}
		// Skip raw tokens with backslash (PDF path artifacts, Windows-1252 curly
		// quotes encoded as 0x92/0x94, corrupted bill/section references, etc.).
		if strings.ContainsAny(rawTokens[i], "\\\x91\x92\x93\x94") {
			continue
		}
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

// extractPlainVoteNames extracts member surnames from a vote-block text where
// names appear as plain capitalized tokens without "Mr./Ms." prefixes (AB, MB, NS).
func extractPlainVoteNames(blockText string) []string {
	return extractPlainVoteNamesN(blockText, 0)
}

// newBrunswickDescriptionFromContext extracts a description from context text before
// a match position. Used by NB, MB, and generic parsers.
func newBrunswickDescriptionFromContext(text string, matchStart int) string {
	start := matchStart - 700
	if start < 0 {
		start = 0
	}
	raw := strings.ReplaceAll(text[start:matchStart], "\u00a0", " ")
	// Normalize PDF backslash line-break artifacts (common in MB journals) before
	// all downstream processing so classifiers and bill-number extraction both see
	// clean text.
	raw = strings.ReplaceAll(raw, `\`, " ")
	snippet := strings.TrimSpace(strings.Join(strings.Fields(raw), " "))
	if snippet == "" {
		return ""
	}

	snippet = strings.TrimSpace(newBrunswickDescRecordedDivisionTailRe.ReplaceAllString(snippet, ""))

	// Parliamentary motion classifier: recognises reading stages, amendments,
	// committee of the whole, budget votes, and address-in-reply motions and
	// returns a compact label (e.g. "Second Reading: Bill 5") without requiring
	// sentence-splitting of verbose motion text.
	if classified := classifyParliamentaryMotion(snippet); classified != "" {
		return classified
	}

	// Primary: look for "Bill No. X" in the context. Sentence-splitting on "." breaks
	// "Bill No." into separate fragments, losing the bill number. Search directly first.
	if m := billNoInContextRe.FindStringSubmatch(snippet); len(m) == 2 {
		billNum := strings.ToUpper(strings.TrimSpace(m[1]))
		if billNum != "" {
			// Attempt to find an ALL-CAPS title in the 200 chars preceding the match.
			matchPos := billNoInContextRe.FindStringIndex(snippet)
			beforeBill := snippet
			if matchPos != nil && matchPos[0] > 0 {
				start := matchPos[0] - 200
				if start < 0 {
					start = 0
				}
				beforeBill = snippet[start:matchPos[0]]
			}
			words := strings.Fields(beforeBill)
			var titleWords []string
			for i := len(words) - 1; i >= 0; i-- {
				w := words[i]
				if w != strings.ToUpper(w) {
					break
				}
				stripped := strings.Trim(w, `.,;:\/'"-`)
				if stripped == "" {
					break
				}
				titleWords = append([]string{w}, titleWords...)
			}
			if title := strings.TrimSpace(strings.Join(titleWords, " ")); title != "" {
				return fmt.Sprintf("%s (Bill %s)", title, billNum)
			}
			return "Bill " + billNum
		}
	}

	parts := strings.FieldsFunc(snippet, func(r rune) bool {
		return r == '.' || r == ';' || r == ':'
	})
	for i := len(parts) - 1; i >= 0; i-- {
		desc := strings.TrimSpace(parts[i])
		if desc == "" || len(desc) < 5 {
			continue
		}
		if newBrunswickDescBoilerplateRe.MatchString(desc) {
			continue
		}
		// Skip all-caps name lists (voter names from adjacent divisions that leak
		// into the context window when two divisions appear on the same PDF page).
		if isAllCapsWordList(desc) {
			continue
		}
		// Skip sentence fragments that begin with a lowercase letter \u2014 these are
		// tails of sentences split on an earlier period (e.g. "r Honour" from "Your Honour").
		if len(desc) > 0 && desc[0] >= 'a' && desc[0] <= 'z' {
			continue
		}
		// Skip journal page-header lines (e.g. "Monday , November 3 , 2025 424").
		if motionDescJournalHeaderRe.MatchString(desc) {
			continue
		}
		// Skip leaked voter-name patterns: a single capital initial followed by an
		// ALL_CAPS surname (e.g. "E WASKO and Hon") from an adjacent division's list.
		if motionDescVoterLeakRe.MatchString(desc) {
			continue
		}
		if len(desc) > 220 {
			desc = strings.TrimSpace(desc[len(desc)-220:])
		}
		if desc != "" {
			return desc
		}
	}

	return ""
}

// isAllCapsWordList reports whether desc contains no lowercase letters and at
// least one word — i.e., looks like a voter name or all-caps header token that
// leaked into the description context.  Legitimate descriptions always contain
// at least one lowercase word.
func isAllCapsWordList(desc string) bool {
	if strings.TrimSpace(desc) == "" {
		return false
	}
	for _, r := range desc {
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return true
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

func crawlGenericProvincialVotesWithAllSittings(indexURL, provinceCode, chamber string, legislature, session int, client *http.Client, allSittings bool, linkMatcher *regexp.Regexp) ([]ProvincialDivisionResult, error) {
	return crawlGenericProvincialVotesWithMatcherOpts(indexURL, provinceCode, chamber, legislature, session, client, allSittings, linkMatcher)
}

func crawlGenericProvincialVotesWithMatcher(indexURL, provinceCode, chamber string, legislature, session int, client *http.Client, linkMatcher *regexp.Regexp) ([]ProvincialDivisionResult, error) {
	return crawlGenericProvincialVotesWithMatcherOpts(indexURL, provinceCode, chamber, legislature, session, client, false, linkMatcher)
}

func crawlGenericProvincialVotesWithMatcherOpts(indexURL, provinceCode, chamber string, legislature, session int, client *http.Client, allSittings bool, linkMatcher *regexp.Regexp) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Debugf("[%s-votes] fetching index: %s", provinceCode, indexURL)

	doc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, fmt.Errorf("%s generic index: %w", provinceCode, err)
	}

	links := discoverProvincialVoteLinksWithMatcher(doc, indexURL, linkMatcher)
	if !allSittings && len(links) > 40 {
		links = links[len(links)-40:]
	}
	if len(links) == 0 {
		links = []string{indexURL}
	}

	results := make([]ProvincialDivisionResult, 0)
	for _, link := range links {
		dayDoc, derr := fetchDoc(link, client)
		if derr != nil {
			clog.Infof("[%s-votes] skip day link %s: %v", provinceCode, link, derr)
			continue
		}
		date := extractDateFromURL(link)
		parsed := parseGenericProvincialVotesDoc(dayDoc, provinceCode, chamber, legislature, session, date)
		results = append(results, parsed...)
	}

	clog.Debugf("[%s-votes] parsed %d divisions", provinceCode, len(results))
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
