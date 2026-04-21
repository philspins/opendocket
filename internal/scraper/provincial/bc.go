package provincial

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/open-democracy/internal/utils"
)

// ── BC constants ──────────────────────────────────────────────────────────────

// bcLIMSBase is the base URL for the BC LIMS document-store REST API.
const bcLIMSBase = "https://lims.leg.bc.ca"

// ── BC bill types ─────────────────────────────────────────────────────────────

type bcProgressBillFile struct {
	ReadingTypeID int    `json:"readingTypeId"`
	FileName      string `json:"fileName"`
}

type bcProgressBillRecord struct {
	BillNumber    int    `json:"billNumber"`
	Title         string `json:"title"`
	BillTypeID    int    `json:"billTypeId"`
	FirstReading  string `json:"firstReading"`
	SecondReading string `json:"secondReading"`
	Committee     string `json:"committeeReading"`
	Report        string `json:"reportReading"`
	Amended       string `json:"amendedReading"`
	ThirdReading  string `json:"thirdReading"`
	RoyalAssent   string `json:"royalAssent"`
	Files         struct {
		Nodes []bcProgressBillFile `json:"nodes"`
	} `json:"files"`
}

type bcGraphQLParliamentResponse struct {
	Data struct {
		AllParliaments struct {
			Nodes []struct {
				SessionsByParliamentID struct {
					Nodes []struct {
						ID     int `json:"id"`
						Number int `json:"number"`
					} `json:"nodes"`
				} `json:"sessionsByParliamentId"`
			} `json:"nodes"`
		} `json:"allParliaments"`
	} `json:"data"`
}

// ── BC bills ──────────────────────────────────────────────────────────────────

var bcDynProgressIframeRe = regexp.MustCompile(`https?://dyn\.leg\.bc\.ca/progress-of-bills\?parliament=[^"']+&session=[^"']+`)
var bcBillLinkRe = regexp.MustCompile(`(?i)(bills-and-legislation|bill)`)

func crawlBritishColumbiaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	if indexURL == "" || strings.Contains(indexURL, "leg.bc.ca/") {
		bills, err := crawlBritishColumbiaBillsFromLIMS(legislature, session, client)
		if err == nil && len(bills) > 0 {
			return bills, nil
		}
	}
	progressURL := indexURL
	if progressURL == "" || strings.Contains(progressURL, "leg.bc.ca/parliamentary-business/") {
		progressURL = fmt.Sprintf("https://dyn.leg.bc.ca/progress-of-bills?parliament=%s&session=%s",
			ordinalSuffix(legislature), ordinalSuffix(session))
	}
	doc, err := fetchDoc(progressURL, client)
	if err != nil {
		if indexURL != "" && progressURL != indexURL {
			return crawlProvincialBillsFromIndexWithMatcher(indexURL, "bc", legislature, session, "british_columbia", client, bcBillLinkRe)
		}
		return nil, fmt.Errorf("bc progress of bills: %w", err)
	}
	bills := parseStructuredProvincialBillRows(doc, progressURL, "bc", legislature, session, "british_columbia")
	if len(bills) == 0 && indexURL != "" {
		return crawlProvincialBillsFromIndexWithMatcher(indexURL, "bc", legislature, session, "british_columbia", client, bcBillLinkRe)
	}
	return bills, nil
}

func crawlBritishColumbiaBillsFromLIMS(legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	sessionID, err := fetchBritishColumbiaSessionID(legislature, session, client)
	if err != nil {
		return nil, err
	}
	resp, err := client.Get(fmt.Sprintf("%s/pdms/bills/progress-of-bills/%d", bcLIMSBase, sessionID))
	if err != nil {
		return nil, fmt.Errorf("bc progress api: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bc progress api: status %d", resp.StatusCode)
	}
	var records []bcProgressBillRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, fmt.Errorf("bc progress api decode: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	out := make([]ProvincialBillStub, 0, len(records))
	for _, record := range records {
		billNumber := strconv.Itoa(record.BillNumber)
		if record.BillTypeID == 0 {
			billNumber = "M " + billNumber
		} else if record.BillTypeID == 3 {
			billNumber = "Pr" + billNumber
		}
		detailURL := fmt.Sprintf("https://www.leg.bc.ca/parliamentary-business/overview/%s-parliament/%s-session/bills/progress-of-bills", ordinalSuffix(legislature), ordinalSuffix(session))
		for _, file := range record.Files.Nodes {
			if path, ok := bcBillFileRoute(file.ReadingTypeID); ok && strings.TrimSpace(file.FileName) != "" {
				detailURL = fmt.Sprintf("https://www.leg.bc.ca/parliamentary-business/overview/%s-parliament/%s-session/bills/%s/%s", ordinalSuffix(legislature), ordinalSuffix(session), path, file.FileName)
				break
			}
		}
		lastActivity := strings.TrimSpace(record.RoyalAssent)
		for _, candidate := range []string{record.ThirdReading, record.Amended, record.Report, record.Committee, record.SecondReading, record.FirstReading} {
			if lastActivity == "" && strings.TrimSpace(candidate) != "" {
				lastActivity = strings.TrimSpace(candidate)
			}
		}
		out = append(out, ProvincialBillStub{
			ID:               ProvincialBillID("bc", legislature, session, billNumber),
			ProvinceCode:     "bc",
			Parliament:       legislature,
			Session:          session,
			Number:           billNumber,
			Title:            strings.TrimSpace(record.Title),
			Chamber:          "british_columbia",
			DetailURL:        detailURL,
			SourceURL:        fmt.Sprintf("%s/pdms/bills/progress-of-bills/%d", bcLIMSBase, sessionID),
			LastActivityDate: lastActivity,
			LastScraped:      utils.NowISO(),
		})
	}
	return out, nil
}

func fetchBritishColumbiaSessionID(legislature, session int, client *http.Client) (int, error) {
	body, err := json.Marshal(map[string]string{
		"query": fmt.Sprintf(`query { allParliaments(condition: { active: true, number: %d }, first: 1) { nodes { sessionsByParliamentId(condition: { active: true }, orderBy: NUMBER_DESC, first: 10) { nodes { id number } } } } }`, legislature),
	})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, bcLIMSBase+"/graphql", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("bc graphql: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("bc graphql: status %d", resp.StatusCode)
	}
	var payload bcGraphQLParliamentResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("bc graphql decode: %w", err)
	}
	for _, parliament := range payload.Data.AllParliaments.Nodes {
		for _, candidate := range parliament.SessionsByParliamentID.Nodes {
			if candidate.Number == session {
				return candidate.ID, nil
			}
		}
	}
	return 0, fmt.Errorf("bc session id not found for legislature %d session %d", legislature, session)
}

func bcBillFileRoute(readingTypeID int) (string, bool) {
	switch readingTypeID {
	case 1:
		return "1st_read", true
	case 2:
		return "amend", true
	case 3:
		return "3rd_read", true
	default:
		return "", false
	}
}

// CrawlBritishColumbiaBills crawls British Columbia bills pages.
func CrawlBritishColumbiaBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlBritishColumbiaBills(indexURL, legislature, session, client)
}

// ── BC vote types ─────────────────────────────────────────────────────────────

// bcLIMSVotesFile describes a single V&P HTML document returned by the BC LIMS API.
type bcLIMSVotesFile struct {
	FileName                string `json:"fileName"`
	FilePath                string `json:"filePath"`
	Published               bool   `json:"published"`
	Date                    string `json:"date"`
	VotesAttributesByFileId struct {
		Nodes []struct {
			VoteNumbers string `json:"voteNumbers"`
		} `json:"nodes"`
	} `json:"votesAttributesByFileId"`
}

// bcLIMSVotesResponse is the JSON envelope from
// https://lims.leg.bc.ca/pdms/votes-and-proceedings/{parliament}{session}.
type bcLIMSVotesResponse struct {
	AllParliamentaryFileAttributes struct {
		Nodes []bcLIMSVotesFile `json:"nodes"`
	} `json:"allParliamentaryFileAttributes"`
}

// ── BC votes ──────────────────────────────────────────────────────────────────

var bcVotesLinkRe = regexp.MustCompile(`(?i)(votes-and-proceedings|journals?|/votes(?:/|$))`)

// parseBCDivisionTable parses a single BC VP <table class="division"> element
// and returns yea count, nay count, yea names, and nay names.
//
// The table layout is:
//
//	<tr><td class="head" colspan="4">Yeas — 48</td></tr>
//	<tr><td>Name <br> Name <br></td> … (4 columns) </tr>
//	<tr><td class="head" colspan="4">Nays — 40</td></tr>
//	<tr><td>Name <br> Name <br></td> … (4 columns) </tr>
func parseBCDivisionTable(table *goquery.Selection) (yeas, nays int, yeaNames, nayNames []string) {
	var currentSide string // "yea" or "nay"
	table.Find("tr").Each(func(_ int, row *goquery.Selection) {
		headCell := row.Find("td.head")
		if headCell.Length() > 0 {
			headText := strings.TrimSpace(headCell.Text())
			headLower := strings.ToLower(headText)
			var count int
			if m := genericYeaRe.FindStringSubmatch(headText); len(m) == 2 {
				count, _ = strconv.Atoi(m[1])
			} else if m := genericNayRe.FindStringSubmatch(headText); len(m) == 2 {
				count, _ = strconv.Atoi(m[1])
			}
			if strings.Contains(headLower, "yea") || strings.Contains(headLower, "aye") {
				currentSide = "yea"
				yeas = count
			} else if strings.Contains(headLower, "nay") {
				currentSide = "nay"
				nays = count
			}
			return
		}
		// Data row: collect member names from each <td> cell.
		row.Find("td").Each(func(_ int, cell *goquery.Selection) {
			// Names are separated by <br> elements; get raw HTML and split on <br>.
			cellHTML, _ := cell.Html()
			// Replace <br> variants with newline then strip any remaining tags.
			brRe := regexp.MustCompile(`(?i)<br\s*/?>`)
			tagRe := regexp.MustCompile(`<[^>]+>`)
			cleaned := tagRe.ReplaceAllString(brRe.ReplaceAllString(cellHTML, "\n"), "")
			for _, raw := range strings.Split(cleaned, "\n") {
				name := strings.TrimSpace(raw)
				if name == "" || name == "/" {
					continue
				}
				switch currentSide {
				case "yea":
					yeaNames = append(yeaNames, name)
				case "nay":
					nayNames = append(nayNames, name)
				}
			}
		})
	})
	return
}

func normaliseBCDivisionText(text string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}

func isBCDivisionOutcomeText(text string) bool {
	return strings.Contains(strings.ToLower(normaliseBCDivisionText(text)), "on the following division")
}

func extractBCDivisionDescription(table *goquery.Selection) string {
	desc := ""
	table.PrevAllFiltered("p").EachWithBreak(func(_ int, p *goquery.Selection) bool {
		text := normaliseBCDivisionText(p.Text())
		if text == "" || isBCDivisionOutcomeText(text) {
			return true
		}
		desc = text
		return false
	})
	return desc
}

// parseBCVotesDivisions parses all recorded divisions from a BC V&P HTML document.
func parseBCVotesDivisions(doc *goquery.Document, sourceURL, date, province string, legislature, session, startDivNum int) []ProvincialDivisionResult {
	var results []ProvincialDivisionResult
	divNum := startDivNum

	// Collect all <p> elements and <table class="division"> in document order.
	doc.Find("p, table.division").Each(func(_ int, sel *goquery.Selection) {
		if goquery.NodeName(sel) != "table" {
			return
		}
		desc := extractBCDivisionDescription(sel)

		yeas, nays, yeaNames, nayNames := parseBCDivisionTable(sel)
		if yeas == 0 && nays == 0 {
			return // no recorded counts → skip (voice vote or committee)
		}

		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		divID := ProvincialDivisionID("bc", legislature, session, divNum, date)
		mv := make([]ProvincialMemberVote, 0, len(yeaNames)+len(nayNames))
		for _, name := range yeaNames {
			mv = append(mv, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Yea"})
		}
		for _, name := range nayNames {
			mv = append(mv, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Nay"})
		}

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
				Chamber:     "british_columbia",
				DetailURL:   sourceURL,
				LastScraped: utils.NowISO(),
			},
			Votes: mv,
		})
		divNum++
	})
	return results
}

// crawlBritishColumbiaVotesFromLIMS fetches the BC LIMS document index and then
// parses each per-day VP HTML file for recorded divisions.
//
// limsBase is the base URL of the LIMS document store (normally bcLIMSBase).
// API: GET {limsBase}/pdms/votes-and-proceedings/{parl}{sess}
// Returns JSON listing all VP HTML files for the parliament/session pair.
// Each file is at:  {limsBase}/pdms/ldp/{parl}{sess}/votes/{fileName}
func crawlBritishColumbiaVotesFromLIMS(limsBase, parliament, session string, legislature, sessionNum int, client *http.Client) ([]ProvincialDivisionResult, error) {
	indexURL := fmt.Sprintf("%s/pdms/votes-and-proceedings/%s%s", limsBase, parliament, session)
	log.Printf("[bc-votes] fetching LIMS index: %s", indexURL)

	req, err := http.NewRequest("GET", indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("bc votes LIMS index: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bc votes LIMS index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bc votes LIMS index: status %d", resp.StatusCode)
	}

	var apiResp bcLIMSVotesResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("bc votes LIMS index JSON: %w", err)
	}

	files := apiResp.AllParliamentaryFileAttributes.Nodes
	log.Printf("[bc-votes] LIMS index: %d VP files for %s%s", len(files), parliament, session)

	var results []ProvincialDivisionResult
	nextDivNum := 1

	for _, f := range files {
		if !f.Published {
			continue
		}
		fileURL := fmt.Sprintf("%s/pdms/ldp/%s%s/votes/%s", limsBase, parliament, session, f.FileName)
		date := ""
		if len(f.Date) >= 10 {
			date = f.Date[:10]
		}
		if date == "" {
			date = extractDateFromURL(fileURL)
		}
		if date == "" {
			date = utils.TodayISO()
		}

		fileResp, ferr := client.Get(fileURL)
		if ferr != nil {
			log.Printf("[bc-votes] skip %s: %v", fileURL, ferr)
			continue
		}
		fileDoc, derr := goquery.NewDocumentFromReader(fileResp.Body)
		fileResp.Body.Close()
		if derr != nil {
			log.Printf("[bc-votes] parse error %s: %v", fileURL, derr)
			continue
		}

		divs := parseBCVotesDivisions(fileDoc, fileURL, date, "british_columbia", legislature, sessionNum, nextDivNum)
		if len(divs) > 0 {
			log.Printf("[bc-votes] %s: parsed %d divisions", date, len(divs))
			results = append(results, divs...)
			nextDivNum += len(divs)
		}
	}

	log.Printf("[bc-votes] parsed %d divisions from %d files", len(results), len(files))
	return results, nil
}

// crawlBritishColumbiaVotes crawls BC V&P data from the LIMS document-store API.
//
// Discovery (completed 2026-04):
//   - The V&P index page at leg.bc.ca embeds an <iframe> pointing to the React SPA
//     dyn.leg.bc.ca/votes-and-proceedings?parliament=43rd&session=2nd.
//   - The SPA fetches its data from https://lims.leg.bc.ca/pdms/votes-and-proceedings/{parl}{sess}
//     (a plain JSON REST endpoint, no authentication required).
//   - Each VP file is at https://lims.leg.bc.ca/pdms/ldp/{parl}{sess}/votes/{fileName}.htm
//   - Recorded divisions appear as <table class="division"> with Yeas/Nays headers.
//
// indexURL, when non-empty, overrides the LIMS base URL. This is used in tests to
// point the scraper at a local HTTP server instead of lims.leg.bc.ca.
func crawlBritishColumbiaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if client == nil {
		client = utils.NewHTTPClient()
	}
	limsBase := bcLIMSBase
	if indexURL != "" {
		limsBase = indexURL
	}
	parl := parliamentOrdinal(legislature)
	sess := parliamentOrdinal(session)
	return crawlBritishColumbiaVotesFromLIMS(limsBase, parl, sess, legislature, session, client)
}

// ParseBCVotesDivisionsForTest is test-only access to the BC VP HTML division parser.
// It accepts a raw HTML string and parses it for recorded divisions.
func ParseBCVotesDivisionsForTest(htmlContent, sourceURL, date string, legislature, session, startDivNum int) []ProvincialDivisionResult {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil
	}
	return parseBCVotesDivisions(doc, sourceURL, date, "british_columbia", legislature, session, startDivNum)
}

// ParliamentOrdinalForTest is test-only access to the ordinal-suffix helper.
func ParliamentOrdinalForTest(n int) string { return parliamentOrdinal(n) }

// CrawlBritishColumbiaVotes crawls British Columbia votes/proceedings pages.
func CrawlBritishColumbiaVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlBritishColumbiaVotes(indexURL, legislature, session, client)
}
