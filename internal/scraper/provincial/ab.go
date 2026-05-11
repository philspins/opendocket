package provincial

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
)

// ── Alberta regexps ──────────────────────────────────────────────────────────

var albertaBillStatusPDFRe = regexp.MustCompile(`(?i)/LAO/Bills/bsr\d+-\d+\.pdf(?:\?[^"']*)?$`)
var albertaBillStatusEntryRe = regexp.MustCompile(`(?i)Bill\s+(Pr?\d+[A-Z]?)\s+--\s+(.+?)\s+First Reading\s+--`)
var albertaBillLinkRe = regexp.MustCompile(`(?i)(assembly-business|bill|bills)`)

// ── Alberta bills ─────────────────────────────────────────────────────────────

func crawlAlbertaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if indexURL == "" {
		indexURL = "https://www.assembly.ab.ca/assembly-business/bills/bill-status"
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}
	bills, err := crawlAlbertaBillsFromDashboard(indexURL, legislature, session, client)
	if err == nil && len(bills) > 0 {
		return bills, nil
	}
	bills, err = crawlAlbertaBillsFromStatusPDF(indexURL, legislature, session, client)
	if err == nil && len(bills) > 0 {
		return bills, nil
	}
	return crawlProvincialBillsFromIndexWithMatcher(indexURL, "ab", legislature, session, "alberta", client, albertaBillLinkRe)
}

func crawlAlbertaBillsFromStatusPDF(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	if strings.HasSuffix(strings.ToLower(indexURL), ".pdf") {
		return parseAlbertaBillStatusPDF(indexURL, legislature, session, client)
	}
	doc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, err
	}
	wantPDF := fmt.Sprintf("bsr%d-%d.pdf", legislature, session)
	pdfURL := ""
	doc.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !albertaBillStatusPDFRe.MatchString(href) || !strings.Contains(strings.ToLower(href), strings.ToLower(wantPDF)) {
			return true
		}
		pdfURL = resolveRelativeURL(indexURL, href)
		return false
	})
	if pdfURL == "" {
		return nil, fmt.Errorf("alberta bill-status pdf not found for legislature %d session %d", legislature, session)
	}
	return parseAlbertaBillStatusPDF(pdfURL, legislature, session, client)
}

func crawlAlbertaBillsFromDashboard(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	dashboardURL := indexURL
	if dashboardURL == "" || strings.Contains(strings.ToLower(dashboardURL), "bill-status") || !strings.Contains(strings.ToLower(dashboardURL), "assembly-dashboard") {
		dashboardURL = fmt.Sprintf("https://www.assembly.ab.ca/assembly-business/assembly-dashboard?legl=%d&session=%d&sectionb=d&btn=i", legislature, session)
	}
	doc, err := fetchDoc(dashboardURL, client)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	out := make([]ProvincialBillStub, 0)
	doc.Find("div.bills div.bill").Each(func(_ int, card *goquery.Selection) {
		item := card.Find("div.item").First()
		if item.Length() == 0 {
			return
		}
		numberText := strings.TrimSpace(strings.Join(strings.Fields(item.Find("a").First().Text()), " "))
		billNumber := ExtractProvincialBillNumber(numberText)
		if billNumber == "" {
			return
		}
		id := ProvincialBillID("ab", legislature, session, billNumber)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		title := strings.TrimSpace(strings.Join(strings.Fields(item.Find("div").Eq(1).Text()), " "))
		detailURL := dashboardURL
		if link := card.Find("div.doc_item a[href]").First(); link.Length() > 0 {
			detailURL = resolveRelativeURL(dashboardURL, link.AttrOr("href", ""))
		}
		out = append(out, ProvincialBillStub{
			ID:           id,
			ProvinceCode: "ab",
			Parliament:   legislature,
			Session:      session,
			Number:       billNumber,
			Title:        title,
			Chamber:      "alberta",
			DetailURL:    detailURL,
			SourceURL:    dashboardURL,
			LastScraped:  utils.NowISO(),
		})
	})
	return out, nil
}

func parseAlbertaBillStatusPDF(pdfURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	text, err := downloadAndExtractPDFText(pdfURL, "ab", client)
	if err != nil {
		return nil, err
	}
	matches := albertaBillStatusEntryRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, nil
	}
	out := make([]ProvincialBillStub, 0, len(matches))
	seen := make(map[string]bool)
	for _, match := range matches {
		billNumber := strings.TrimSpace(match[1])
		title := strings.TrimSpace(match[2])
		id := ProvincialBillID("ab", legislature, session, billNumber)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, ProvincialBillStub{
			ID:           id,
			ProvinceCode: "ab",
			Parliament:   legislature,
			Session:      session,
			Number:       billNumber,
			Title:        title,
			Chamber:      "alberta",
			DetailURL:    pdfURL,
			SourceURL:    pdfURL,
			LastScraped:  utils.NowISO(),
		})
	}
	return out, nil
}

// CrawlAlbertaBills crawls Alberta bills pages.
func CrawlAlbertaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlAlbertaBills(indexURL, legislature, session, client)
}

// crawlAlbertaAllSittingsBills crawls bills for the current legislature and the
// two preceding ones (≈3 legislatures / ~10 years), trying sessions 1–4 each.
func crawlAlbertaAllSittingsBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	var all []ProvincialBillStub
	seenID := make(map[string]bool)
	for leg := legislature; leg >= legislature-2 && leg >= 1; leg-- {
		for sess := 1; sess <= 4; sess++ {
			clog.Debugf("[ab-bills] all-sittings: crawling legislature %d session %d", leg, sess)
			bills, err := crawlAlbertaBills(indexURL, leg, sess, client)
			if err != nil || len(bills) == 0 {
				break
			}
			for _, b := range bills {
				if !seenID[b.ID] {
					seenID[b.ID] = true
					all = append(all, b)
				}
			}
		}
	}
	return all, nil
}

// ── Alberta votes ─────────────────────────────────────────────────────────────

// albertaVotesPDFLinkRe matches VP PDF hrefs on the AB votes-and-proceedings page.
// AB index pages embed backslash-escaped paths; normalizeHref converts them first.
var albertaVotesPDFLinkRe = regexp.MustCompile(`(?i)docs\.assembly\.ab\.ca[^"'\s]*_vp\.pdf`)

// abForCountRe / abAgainstCountRe extract vote totals from AB V&P PDF text.
var abForCountRe = regexp.MustCompile(`(?i)For\s+the\s+[^:]{1,60}:\s*(\d+)`)
var abAgainstCountRe = regexp.MustCompile(`(?i)Against\s+the\s+[^:]{1,60}:\s*(\d+)`)
var abDivisionSplitRe = regexp.MustCompile(`(?i)DIVISION\s+\d+`)
var abQuestionVoteMarkerRe = regexp.MustCompile(`(?is)The question being put,.*?names being called for were taken as follows:\s*`)
var abReadATimeBillRe = regexp.MustCompile(`(?i)read\s+a\s+(first|second|third)\s+time:\s*bill\s+((?:pr?)?\d+[A-Z]?)\s+(.+?)(?:\s+--|\s+hon\.|\s+a\s+debate\b|\s+the\s+question\b|$)`)
var abReadingBillRe = regexp.MustCompile(`(?i)(first|second|third)\s+reading\s+on\s+bill\s+((?:pr?)?\d+[A-Z]?)\s*[,:-]?\s*(.+?)(?:\s+--|\s+hon\.|\s+the\s+question\b|$)`)
var abBillAmendmentRe = regexp.MustCompile(`(?i)bill\s+((?:pr?)?\d+[A-Z]?)\s+amendment`)
var abAmendmentToBillRe = regexp.MustCompile(`(?i)amendment\s+(?:to|on)\s+bill\s+((?:pr?)?\d+[A-Z]?)`)
var abBillTitleRe = regexp.MustCompile(`(?i)bill\s+((?:pr?)?\d+[A-Z]?)\s+(.+?)(?:\s+--|\s+hon\.|\s+the\s+question\b|$)`)
var abReadingWordRe = regexp.MustCompile(`(?i)\b(first|second|third)\s+reading\b|\bread\s+a\s+(first|second|third)\s+time\b`)

// parseAlbertaVPDivisions parses recorded vote divisions from normalised AB V&P PDF text.
// Alberta uses "For the [phrase]: N" / "Against the [phrase]: N" format with plain surname lists.
func parseAlbertaVPDivisions(text, detailURL string, legislature, session, startDivisionNumber int, date string) []ProvincialDivisionResult {
	divBlocks := abDivisionSplitRe.FindAllStringIndex(text, -1)
	if len(divBlocks) == 0 {
		return parseAlbertaQuestionBlocks(text, detailURL, legislature, session, startDivisionNumber, date)
	}
	results := make([]ProvincialDivisionResult, 0, len(divBlocks))
	for i, span := range divBlocks {
		end := len(text)
		if i+1 < len(divBlocks) {
			end = divBlocks[i+1][0]
		}
		block := text[span[0]:end]

		forM := abForCountRe.FindStringSubmatchIndex(block)
		agaM := abAgainstCountRe.FindStringSubmatchIndex(block)
		if forM == nil || agaM == nil {
			continue
		}
		yeas, _ := strconv.Atoi(block[forM[2]:forM[3]])
		nays, _ := strconv.Atoi(block[agaM[2]:agaM[3]])
		if yeas == 0 && nays == 0 {
			continue
		}

		// Names appear between end-of-"For..." and start-of-"Against...".
		yeaBlock := ""
		nayBlock := ""
		forEnd := forM[1]
		agaStart := agaM[0]
		agaEnd := agaM[1]
		if forEnd <= agaStart {
			yeaBlock = block[forEnd:agaStart]
		}
		if agaEnd < len(block) {
			nayBlock = block[agaEnd:]
		}

		divNum := startDivisionNumber + i
		divID := ProvincialDivisionID("ab", legislature, session, divNum, date)

		// Description: text between division heading and "For the..."
		desc := strings.TrimSpace(block[len(abDivisionSplitRe.FindString(block)):forM[0]])
		desc = strings.Join(strings.Fields(strings.ReplaceAll(desc, "\u00a0", " ")), " ")
		if len(desc) > 220 {
			desc = desc[len(desc)-220:]
		}
		desc = cleanAlbertaDivisionDescription(desc)
		if desc == "" {
			desc = "Recorded division"
		}

		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		votes := make([]ProvincialMemberVote, 0, yeas+nays)
		for _, name := range extractPlainVoteNamesN(yeaBlock, yeas) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Yea"})
		}
		for _, name := range extractPlainVoteNamesN(nayBlock, nays) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Nay"})
		}

		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID: divID, Parliament: legislature, Session: session,
				Number: divNum, Date: date, Description: desc,
				Yeas: yeas, Nays: nays, Result: result,
				Chamber: "alberta", DetailURL: detailURL, LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
	}
	clog.Debugf("[ab-votes] parsed %d divisions", len(results))
	return results
}

func parseAlbertaQuestionBlocks(text, detailURL string, legislature, session, startDivisionNumber int, date string) []ProvincialDivisionResult {
	markers := abQuestionVoteMarkerRe.FindAllStringIndex(text, -1)
	if len(markers) == 0 {
		return nil
	}
	results := make([]ProvincialDivisionResult, 0, len(markers))
	for i, marker := range markers {
		end := len(text)
		if i+1 < len(markers) {
			end = markers[i+1][0]
		}
		block := text[marker[1]:end]
		forM := abForCountRe.FindStringSubmatchIndex(block)
		agaM := abAgainstCountRe.FindStringSubmatchIndex(block)
		if forM == nil || agaM == nil {
			continue
		}
		yeas, _ := strconv.Atoi(block[forM[2]:forM[3]])
		nays, _ := strconv.Atoi(block[agaM[2]:agaM[3]])
		if yeas == 0 && nays == 0 {
			continue
		}

		yeaBlock := ""
		nayBlock := ""
		forEnd := forM[1]
		agaStart := agaM[0]
		agaEnd := agaM[1]
		if forEnd <= agaStart {
			yeaBlock = block[forEnd:agaStart]
		}
		if agaEnd < len(block) {
			nayBlock = block[agaEnd:]
		}

		divNum := startDivisionNumber + len(results)
		divID := ProvincialDivisionID("ab", legislature, session, divNum, date)
		desc := extractAlbertaQuestionDescription(text, marker[0])
		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		votes := make([]ProvincialMemberVote, 0, yeas+nays)
		for _, name := range extractPlainVoteNamesN(yeaBlock, yeas) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Yea"})
		}
		for _, name := range extractPlainVoteNamesN(nayBlock, nays) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Nay"})
		}

		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID: divID, Parliament: legislature, Session: session,
				Number: divNum, Date: date, Description: desc,
				Yeas: yeas, Nays: nays, Result: result,
				Chamber: "alberta", DetailURL: detailURL, LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
	}
	clog.Debugf("[ab-votes] parsed %d divisions", len(results))
	return results
}

func extractAlbertaQuestionDescription(text string, markerStart int) string {
	start := markerStart - 500
	if start < 0 {
		start = 0
	}
	context := strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(text[start:markerStart], "\u00a0", " ")), " "))
	if context == "" {
		return "Recorded division"
	}
	lower := strings.ToLower(context)
	anchors := []string{
		"on the motion that",
		"be it resolved that",
		" moved pursuant to ",
		" moved adjournment of ",
		" moved the following amendment",
	}
	best := -1
	for _, anchor := range anchors {
		if idx := strings.LastIndex(lower, anchor); idx > best {
			best = idx
		}
	}
	if best >= 0 {
		context = strings.TrimSpace(context[best:])
	}
	context = regexp.MustCompile(`^(?:[A-Z][A-Z\s]{3,}\s+)+`).ReplaceAllString(context, "")
	context = cleanAlbertaDivisionDescription(context)
	if context == "" {
		return "Recorded division"
	}
	return context
}

func cleanAlbertaDivisionDescription(context string) string {
	context = strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(context, "\u00a0", " ")), " "))
	if context == "" {
		return ""
	}

	if m := abReadATimeBillRe.FindStringSubmatch(context); len(m) == 4 {
		reading := strings.Title(strings.ToLower(m[1])) + " Reading"
		bill := strings.TrimSpace(m[2])
		title := strings.Trim(strings.TrimSpace(m[3]), "-:;, ")
		if title != "" {
			return fmt.Sprintf("Bill %s %s - %s", bill, title, reading)
		}
		return fmt.Sprintf("Bill %s - %s", bill, reading)
	}

	if m := abReadingBillRe.FindStringSubmatch(context); len(m) == 4 {
		reading := strings.Title(strings.ToLower(m[1])) + " Reading"
		bill := strings.TrimSpace(m[2])
		title := strings.Trim(strings.TrimSpace(m[3]), "-:;, ")
		if title != "" {
			return fmt.Sprintf("Bill %s %s - %s", bill, title, reading)
		}
		return fmt.Sprintf("Bill %s - %s", bill, reading)
	}

	if m := abBillAmendmentRe.FindStringSubmatch(context); len(m) == 2 {
		return fmt.Sprintf("Bill %s amendment", strings.TrimSpace(m[1]))
	}
	if m := abAmendmentToBillRe.FindStringSubmatch(context); len(m) == 2 {
		return fmt.Sprintf("Bill %s amendment", strings.TrimSpace(m[1]))
	}

	if m := abBillTitleRe.FindStringSubmatch(context); len(m) == 3 {
		bill := strings.TrimSpace(m[1])
		title := strings.Trim(strings.TrimSpace(m[2]), "-:;, ")
		reading := ""
		if rm := abReadingWordRe.FindStringSubmatch(context); len(rm) > 0 {
			word := ""
			if len(rm) > 1 && rm[1] != "" {
				word = rm[1]
			} else if len(rm) > 2 {
				word = rm[2]
			}
			if word != "" {
				reading = strings.Title(strings.ToLower(word)) + " Reading"
			}
		}
		if title != "" && reading != "" {
			return fmt.Sprintf("Bill %s %s - %s", bill, title, reading)
		}
		if title != "" {
			return fmt.Sprintf("Bill %s %s", bill, title)
		}
		if reading != "" {
			return fmt.Sprintf("Bill %s - %s", bill, reading)
		}
		return fmt.Sprintf("Bill %s", bill)
	}

	if len(context) > 220 {
		context = context[len(context)-220:]
	}
	return context
}

// ParseAlbertaVPDivisionsForTest is test-only access to AB V&P parsing logic.
func ParseAlbertaVPDivisionsForTest(text, detailURL string, legislature, session, startDivNum int, date string) []ProvincialDivisionResult {
	return parseAlbertaVPDivisions(text, detailURL, legislature, session, startDivNum, date)
}

// crawlAlbertaVotesFromPDF fetches the AB V&P index page, discovers per-day PDF links
// (fixing the backslash-escaped hrefs), and parses each PDF.
func crawlAlbertaVotesFromPDF(indexURL string, legislature, session int, client *http.Client, allSittings bool) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	clog.Debugf("[ab-votes] fetching index: %s", indexURL)
	indexDoc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, fmt.Errorf("ab votes index: %w", err)
	}

	var pdfLinks []string
	seen := make(map[string]bool)
	indexDoc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !albertaVotesPDFLinkRe.MatchString(href) {
			return
		}
		full := resolveRelativeURL(indexURL, href)
		if seen[full] {
			return
		}
		seen[full] = true
		pdfLinks = append(pdfLinks, full)
	})

	sort.Strings(pdfLinks)
	if allSittings {
		clog.Debugf("[ab-votes] all-sittings: crawling legislature %d session %d", legislature, session)
	} else if len(pdfLinks) > 60 {
		pdfLinks = pdfLinks[len(pdfLinks)-60:]
	}
	if len(pdfLinks) == 0 {
		clog.Infof("[ab-votes] no VP PDFs discovered")
		return nil, nil
	}

	var results []ProvincialDivisionResult
	nextDivNum := 1
	for _, pdfURL := range pdfLinks {
		text, terr := downloadAndExtractPDFText(pdfURL, "ab", client)
		if terr != nil {
			clog.Debugf("[ab-votes] skip pdf %s: %v", pdfURL, terr)
			continue
		}
		date := extractDateFromURL(pdfURL)
		if date == "" {
			date = utils.FindDateInText(text)
		}
		if date == "" {
			date = utils.TodayISO()
		}
		divs := parseAlbertaVPDivisions(text, pdfURL, legislature, session, nextDivNum, date)
		results = append(results, divs...)
		nextDivNum += len(divs)
		if len(divs) == 0 {
			nextDivNum++
		}
	}
	clog.Debugf("[ab-votes] parsed %d divisions from %d PDFs", len(results), len(pdfLinks))
	return results, nil
}

// CrawlAlbertaVotes crawls Alberta votes/proceedings pages.
// The AB assembly page links to per-day VP PDFs via backslash-escaped hrefs; this
// function normalises those hrefs and parses each PDF using the Alberta-specific
// "For the [phrase]: N / Against the [phrase]: N" vote format.
func crawlAlbertaVotes(indexURL string, legislature, session int, client *http.Client, allSittings bool) ([]ProvincialDivisionResult, error) {
	if indexURL == "" {
		indexURL = "https://www.assembly.ab.ca/assembly-business/assembly-records/votes-and-proceedings"
	}
	return crawlAlbertaVotesFromPDF(indexURL, legislature, session, client, allSittings)
}

// CrawlAlbertaVotes crawls Alberta votes/proceedings pages.
func CrawlAlbertaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlAlbertaVotes(indexURL, legislature, session, client, false)
}

// crawlAlbertaAllSittingsVotes iterates 3 legislatures × up to 4 sessions
// using the AB VP URL's ?legl=&session= query params to fetch all historical PDFs.
func crawlAlbertaAllSittingsVotes(legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	var all []ProvincialDivisionResult
	seenID := make(map[string]bool)
	for leg := legislature; leg >= legislature-2 && leg >= 1; leg-- {
		for sess := 1; sess <= 4; sess++ {
			vpURL := fmt.Sprintf("https://www.assembly.ab.ca/assembly-business/assembly-records/votes-and-proceedings?legl=%d&session=%d", leg, sess)
			divs, err := crawlAlbertaVotesFromPDF(vpURL, leg, sess, client, true)
			if err != nil || len(divs) == 0 {
				break
			}
			for _, d := range divs {
				if !seenID[d.Division.ID] {
					seenID[d.Division.ID] = true
					all = append(all, d)
				}
			}
		}
	}
	return all, nil
}
