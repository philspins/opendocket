package provincial

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/open-democracy/internal/utils"
)

// ── PEI constants ─────────────────────────────────────────────────────────────

var peiBillLinkRe = regexp.MustCompile(`(?i)(legislative-business|bill)`)
var peiVotesLinkRe = regexp.MustCompile(`(?i)(legislative-business|votes|proceedings)`)

const (
	peiWorkflowBills       = "LegislativeAssemblyBillProgress"
	peiWDFActivityBills    = "LegislativeAssemblyBillSearch"
	peiWDFActivityBillView = "LegislativeAssemblyBillView"
	peiWDFActivityJournals = "LegislativeAssemblyJournalsSearch"
	peiWorkflowJournals    = "LegislativeAssemblyJournals"

	peiBillsIndexURL    = peiAssemblyBase + "/legislative-business/house-records/bills"
	peiJournalsIndexURL = peiAssemblyBase + "/legislative-business/house-records/journals"
	peiAssemblyBase     = "https://www.assembly.pe.ca"
	peiWDFAPIBase       = "https://wdf.princeedwardisland.ca"

	peiCaptchaSignature    = "captcha.perfdrive.com"
	peiBotManagerSignature = "perfdrive.com"

	peiDefaultDelay = 6 * time.Second
)

const peiGeneralAssembly = 67
const peiAssemblySession = 3

// ── WDF types (shared between PEI bills and votes) ────────────────────────────

type wdfNode struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Data     json.RawMessage `json:"data"`
	Children []wdfNode       `json:"children"`
}

type wdfTreeResponse struct {
	ProcessInstanceID string `json:"processInstanceId"`
	Messages          struct {
		Error []string `json:"error"`
	} `json:"messages"`
	Data []wdfNode `json:"data"`
}

type wdfCellData struct {
	Text *string `json:"text"`
}

type wdfLinkData struct {
	Text        string            `json:"text"`
	Href        *string           `json:"href"`
	RouterLink  *string           `json:"routerLink"`
	QueryParams map[string]string `json:"queryParams"`
}

func wdfCollectRows(nodes []wdfNode) []wdfNode {
	var out []wdfNode
	for _, n := range nodes {
		if n.Type == "TableV2Row" {
			out = append(out, n)
		}
		out = append(out, wdfCollectRows(n.Children)...)
	}
	return out
}

// ── PEI WDF HTTP helpers ──────────────────────────────────────────────────────

func isPEICaptchaBody(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, peiCaptchaSignature) || strings.Contains(lower, peiBotManagerSignature)
}

func postPEIWorkflow(ctx context.Context, wdfBase, workflowName, activityName string, queryVars map[string]string, client *http.Client, delay time.Duration) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	isTestServer := strings.HasPrefix(wdfBase, "http://127.0.0.1") || strings.HasPrefix(wdfBase, "http://localhost")
	if !isTestServer {
		data, err := invokePEIFetchJS(ctx, workflowName, activityName, queryVars)
		if err != nil {
			log.Printf("[pe-wdf] pei_fetch.js error: %v; will fall back to HTML", err)
			return nil, nil
		}
		if data != nil {
			time.Sleep(delay)
		}
		return data, nil
	}
	return postPEIWorkflowHTTP(ctx, wdfBase, workflowName, activityName, queryVars, client, delay)
}

func invokePEIFetchJS(ctx context.Context, workflowName, activityName string, queryVars map[string]string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return nil, nil
	}
	scriptPath := filepath.Join("scripts", "pei_fetch.js")
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, nil
	}
	qvJSON, err := json.Marshal(queryVars)
	if err != nil {
		return nil, fmt.Errorf("marshal queryVars: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, nodePath, scriptPath, workflowName, activityName, string(qvJSON))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("[pe-wdf] pei_fetch.js: %v stderr=%s", err, stderr.String())
		return nil, nil
	}
	return stdout.Bytes(), nil
}

func postPEIWorkflowHTTP(ctx context.Context, wdfBase, workflowName, activityName string, queryVars map[string]string, client *http.Client, delay time.Duration) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	apiURL := strings.TrimRight(wdfBase, "/") + "/legislative-assembly/services/api/workflow"

	merged := make(map[string]interface{}, len(queryVars)+2)
	merged["service"] = workflowName
	merged["activity"] = activityName
	for k, v := range queryVars {
		merged[k] = v
	}
	payload := map[string]interface{}{
		"appName":     workflowName,
		"featureName": workflowName,
		"metaVars":    map[string]interface{}{"service_id": nil, "save_location": nil},
		"queryVars":   merged,
		"queryName":   activityName,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("pe wdf marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("pe wdf request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Client-Show-Status", "true")

	transport := http.RoundTripper(http.DefaultTransport)
	if client != nil && client.Transport != nil {
		transport = client.Transport
	}
	timeout := 20 * time.Second
	if client != nil && client.Timeout > 0 {
		timeout = client.Timeout
	}
	noRedirect := &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pe wdf do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[pe-wdf] %s returned HTTP %d; will fall back to HTML", workflowName, resp.StatusCode)
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("pe wdf read: %w", err)
	}
	time.Sleep(delay)
	return data, nil
}

// ── PEI HTTP client ───────────────────────────────────────────────────────────

func peiSourceClient(url string, client *http.Client) *http.Client {
	if client == nil {
		return nil
	}
	if strings.HasPrefix(url, "http://127.0.0.1") || strings.HasPrefix(url, "http://localhost") {
		return client
	}
	return nil
}

type peiTransport struct {
	base  http.RoundTripper
	delay time.Duration
}

func (t *peiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	clone.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	clone.Header.Set("Accept-Language", "en-CA,en-US;q=0.7,en;q=0.3")
	clone.Header.Set("Sec-Fetch-Dest", "document")
	clone.Header.Set("Sec-Fetch-Mode", "navigate")
	clone.Header.Set("Sec-Fetch-Site", "none")
	resp, err := t.base.RoundTrip(clone)
	time.Sleep(t.delay)
	return resp, err
}

func newPEIHTTPClient(delay time.Duration) *http.Client {
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: &peiTransport{base: http.DefaultTransport, delay: delay},
	}
}

// ── PEI assembly session detection ───────────────────────────────────────────

func fetchPEICurrentAssemblySession() (int, int, bool) {
	queryVars := map[string]string{
		"year":          strconv.Itoa(time.Now().Year()),
		"search":        "year",
		"search_bills":  "true",
		"wdf_url_query": "true",
	}
	data, err := invokePEIFetchJS(context.Background(), peiWorkflowBills, peiWDFActivityBills, queryVars)
	if err != nil || data == nil {
		return 0, 0, false
	}
	var resp wdfTreeResponse
	if err := json.Unmarshal(data, &resp); err != nil || resp.Data == nil {
		return 0, 0, false
	}
	for _, row := range wdfCollectRows(resp.Data) {
		for _, cell := range row.Children {
			for _, child := range cell.Children {
				if child.Type != "LinkV2" {
					continue
				}
				var ld wdfLinkData
				if json.Unmarshal(child.Data, &ld) != nil {
					continue
				}
				if l, s, ok := peiAssemblySessionFromQueryParams(ld.QueryParams); ok {
					return l, s, true
				}
				if ld.RouterLink != nil && *ld.RouterLink != "" {
					if candidates := extractLegislatureSessionCandidates("pe", *ld.RouterLink, 50); len(candidates) > 0 {
						best := candidates[0]
						for _, c := range candidates[1:] {
							if c.Score > best.Score {
								best = c
							}
						}
						return best.Legislature, best.Session, true
					}
				}
			}
		}
	}
	return 0, 0, false
}

func FetchPEICurrentAssemblySession() (int, int, bool) {
	return fetchPEICurrentAssemblySession()
}

func PEIFallbackAssembly() int {
	return peiGeneralAssembly
}

func PEIFallbackSession() int {
	return peiAssemblySession
}

func peiAssemblySessionFromQueryParams(params map[string]string) (int, int, bool) {
	if len(params) == 0 {
		return 0, 0, false
	}
	var legislature, session int
	for k, v := range params {
		kl := strings.ToLower(k)
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n <= 0 {
			continue
		}
		switch {
		case strings.Contains(kl, "assembly") || strings.Contains(kl, "legislature") || kl == "ga":
			legislature = n
		case strings.Contains(kl, "session"):
			session = n
		}
	}
	if legislature > 0 && session > 0 {
		return legislature, session, true
	}
	return 0, 0, false
}

// ── PEI bills ─────────────────────────────────────────────────────────────────

func firstWDFLinkHref(nodes []wdfNode) string {
	for _, node := range nodes {
		if node.Type == "LinkV2" {
			var ld wdfLinkData
			if json.Unmarshal(node.Data, &ld) == nil && ld.Href != nil && strings.TrimSpace(*ld.Href) != "" {
				return strings.TrimSpace(*ld.Href)
			}
		}
		if href := firstWDFLinkHref(node.Children); href != "" {
			return href
		}
	}
	return ""
}

func fetchPEIBillDetailURL(wdfBase, billDocID string, client *http.Client, delay time.Duration) string {
	if strings.TrimSpace(billDocID) == "" {
		return ""
	}
	body, err := postPEIWorkflow(context.Background(), wdfBase, peiWorkflowBills, peiWDFActivityBillView, map[string]string{
		"id": billDocID,
	}, client, delay)
	if err != nil || body == nil {
		return ""
	}
	var resp wdfTreeResponse
	if err := json.Unmarshal(body, &resp); err != nil || resp.Data == nil {
		return ""
	}
	return firstWDFLinkHref(resp.Data)
}

func crawlPEIBillsFromWorkflow(wdfBase string, year, legislature, session int, client *http.Client, delay time.Duration) ([]ProvincialBillStub, error) {
	params := map[string]string{
		"search_bills":  "true",
		"wdf_url_query": "true",
	}
	if legislature > 0 && session > 0 {
		params["search"] = "assembly"
		params["general_assembly"] = strconv.Itoa(legislature)
		params["session"] = strconv.Itoa(session)
	} else {
		params["year"] = strconv.Itoa(year)
		params["search"] = "year"
	}
	body, err := postPEIWorkflow(context.Background(), wdfBase, peiWorkflowBills, peiWDFActivityBills, params, client, delay)
	if err != nil || body == nil {
		return nil, err
	}

	var resp wdfTreeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		log.Printf("[pe-bills] wdf tree decode: %v; falling back to HTML", err)
		return nil, nil
	}
	if resp.Data == nil {
		log.Printf("[pe-bills] wdf returned null data; falling back to HTML")
		return nil, nil
	}

	rows := wdfCollectRows(resp.Data)
	if len(rows) == 0 {
		log.Printf("[pe-bills] wdf returned 0 bill rows; falling back to HTML")
		return nil, nil
	}

	seen := make(map[string]bool)
	out := make([]ProvincialBillStub, 0, len(rows))
	for _, row := range rows {
		if len(row.Children) < 2 {
			continue
		}
		var title, detailURL, billDocID string
		if len(row.Children[0].Children) > 0 {
			lnk := row.Children[0].Children[0]
			if lnk.Type == "LinkV2" {
				var ld wdfLinkData
				if json.Unmarshal(lnk.Data, &ld) == nil {
					title = strings.TrimSpace(ld.Text)
					if id, ok := ld.QueryParams["id"]; ok {
						billDocID = strings.TrimSpace(id)
					}
					if ld.Href != nil && *ld.Href != "" {
						detailURL = *ld.Href
					} else if ld.RouterLink != nil && *ld.RouterLink != "" {
						detailURL = *ld.RouterLink
					}
				}
			}
		}

		billNumber := ""
		var cd1 wdfCellData
		if json.Unmarshal(row.Children[1].Data, &cd1) == nil && cd1.Text != nil {
			billNumber = strings.ToUpper(strings.TrimSpace(*cd1.Text))
		}
		if billNumber == "" {
			billNumber = ExtractProvincialBillNumber(title)
		}
		if billNumber == "" {
			continue
		}

		lastActivity := ""
		if len(row.Children) > 3 {
			var cd wdfCellData
			if json.Unmarshal(row.Children[3].Data, &cd) == nil && cd.Text != nil {
				lastActivity = utils.FindDateInText(*cd.Text)
			}
		}

		id := ProvincialBillID("pe", legislature, session, billNumber)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		if title == "" {
			title = "Bill " + billNumber
		}

		if billDocID != "" {
			if pdfURL := fetchPEIBillDetailURL(wdfBase, billDocID, client, delay); pdfURL != "" {
				detailURL = pdfURL
			}
		}
		detailURL = resolveRelativeURL(peiAssemblyBase, detailURL)

		out = append(out, ProvincialBillStub{
			ID:               id,
			ProvinceCode:     "pe",
			Parliament:       legislature,
			Session:          session,
			Number:           billNumber,
			Title:            title,
			Chamber:          "pei",
			DetailURL:        detailURL,
			SourceURL:        strings.TrimRight(wdfBase, "/") + "/legislative-assembly/services/api/workflow",
			LastActivityDate: lastActivity,
			LastScraped:      utils.NowISO(),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	log.Printf("[pe-bills] wdf parsed %d bills", len(out))
	return out, nil
}

func crawlPrinceEdwardIslandBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	defaultURL := indexURL == ""
	if defaultURL {
		indexURL = peiBillsIndexURL
	}
	delay := time.Duration(0)
	if client == nil {
		delay = peiDefaultDelay
		client = newPEIHTTPClient(delay)
	}

	wdfBase := peiWDFAPIBase
	if !defaultURL {
		wdfBase = indexURL
	}
	year := time.Now().Year()
	bills, werr := crawlPEIBillsFromWorkflow(wdfBase, year, legislature, session, client, delay)
	if werr == nil && len(bills) > 0 {
		return bills, nil
	}
	if werr != nil {
		log.Printf("[pe-bills] wdf api: %v; falling back to HTML", werr)
	}

	return crawlProvincialBillsFromIndexWithMatcher(indexURL, "pe", legislature, session, "pei", client, peiBillLinkRe)
}

// CrawlPrinceEdwardIslandBills crawls PEI bills pages.
func CrawlPrinceEdwardIslandBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlPrinceEdwardIslandBills(indexURL, legislature, session, client)
}

// ── PEI journal PDF division parsers ─────────────────────────────────────────

var peiDivTriggerRe = regexp.MustCompile(`(?i)A\s+Recorded\s+Division\s+being\s+sought[^.]*?the\s+names\s+were\s+recorded[^.]*?as\s+follows:`)
var peiDivCountRe = regexp.MustCompile(`(?i)(Nays?|Yeas?)\s*\(?\s*(\d[\d\s]*)\s*\\`)
var peiDivOutcomeRe = regexp.MustCompile(`(?i)(?:Motion\s+(?:was\s+)?(?:CARRIED|NEGATIVED)|CARRIED\s+UNANIMOUSLY|Motion\s+resolved\s+in\s+the)`)
var peiJournalPageHeaderRe = regexp.MustCompile(`JOURNAL OF THE LEGISLATIVE ASSEMBLY`)
var peiOctalEscapeRe = regexp.MustCompile(`\\(\d{3})`)
var peiPremierRe = regexp.MustCompile(`(?i)Hon\.\s+Premier`)

var peiTitlePrefixes = []string{
	"Hon. Leader of the Opposition",
	"Hon. Leader of the Third Party",
	"Leader of the Third Party",
	"Leader of the Opposition",
}

var peiRidingStartWords = map[string]struct{}{
	"Charlottetown": {}, "Summerside": {}, "Stanhope": {}, "Mermaid": {},
	"Morell": {}, "Kellys": {}, "New": {}, "Borden": {}, "Brackley": {},
	"Evangeline": {}, "Alberton": {}, "Tignish": {}, "O'Leary": {},
	"Tyne": {}, "Kensington": {}, "Crapaud": {}, "Georgetown": {},
	"Vernon": {}, "Murray": {}, "Rustico": {},
	"Cornwall": {}, "Souris": {}, "Stratford": {}, "Kinkora": {},
}

func parsePEIJournalDivisions(rawText, pdfURL string, legislature, session, startDivNum int, date string) []ProvincialDivisionResult {
	text := peiOctalEscapeRe.ReplaceAllStringFunc(rawText, func(m string) string {
		n, err := strconv.ParseInt(m[1:], 8, 32)
		if err != nil || n < 32 || n > 255 {
			return "'"
		}
		return string(rune(n))
	})
	text = peiJournalPageHeaderRe.ReplaceAllString(text, " ")
	text = strings.Join(strings.Fields(text), " ")

	triggers := peiDivTriggerRe.FindAllStringIndex(text, -1)
	if len(triggers) == 0 {
		return nil
	}

	var results []ProvincialDivisionResult
	divNum := startDivNum
	for i, trigger := range triggers {
		blockStart := trigger[1]
		blockEnd := len(text)
		if i+1 < len(triggers) {
			blockEnd = triggers[i+1][0]
		}
		if m := peiDivOutcomeRe.FindStringIndex(text[blockStart:blockEnd]); m != nil {
			end := blockStart + m[1] + 80
			if end < blockEnd {
				blockEnd = end
			}
		}
		block := text[blockStart:blockEnd]

		counts := peiDivCountRe.FindAllStringSubmatchIndex(block, -1)
		if len(counts) == 0 {
			continue
		}

		var yeas, nays int
		var yeasBlock, naysBlock string

		firstLabel := strings.ToLower(block[counts[0][2]:counts[0][3]])
		firstCountStr := strings.ReplaceAll(block[counts[0][4]:counts[0][5]], " ", "")
		firstCount, _ := strconv.Atoi(firstCountStr)

		if len(counts) >= 2 {
			firstMemberBlock := block[counts[0][1]:counts[1][0]]
			secondCountStr := strings.ReplaceAll(block[counts[1][4]:counts[1][5]], " ", "")
			secondCount, _ := strconv.Atoi(secondCountStr)
			secondMemberBlock := block[counts[1][1]:]

			if strings.HasPrefix(firstLabel, "yea") {
				yeas, yeasBlock = firstCount, firstMemberBlock
				nays, naysBlock = secondCount, secondMemberBlock
			} else {
				nays, naysBlock = firstCount, firstMemberBlock
				yeas, yeasBlock = secondCount, secondMemberBlock
			}
		} else {
			memberBlock := block[counts[0][1]:]
			if strings.HasPrefix(firstLabel, "yea") {
				yeas, yeasBlock = firstCount, memberBlock
			} else {
				nays, naysBlock = firstCount, memberBlock
			}
		}

		if yeas == 0 && nays == 0 {
			continue
		}

		desc := "Recorded division"
		descStart := trigger[0] - 250
		if descStart < 0 {
			descStart = 0
		}
		if ctx := strings.TrimSpace(text[descStart:trigger[0]]); ctx != "" {
			if idx := strings.LastIndexAny(ctx, ".;"); idx >= 0 {
				ctx = strings.TrimSpace(ctx[idx+1:])
			}
			if len(ctx) > 200 {
				ctx = ctx[:200]
			}
			if ctx = strings.Join(strings.Fields(ctx), " "); ctx != "" {
				desc = ctx
			}
		}

		divID := ProvincialDivisionID("pe", legislature, session, divNum, date)
		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		var votes []ProvincialMemberVote
		for _, name := range parsePEIJournalMembers(yeasBlock) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Yea"})
		}
		for _, name := range parsePEIJournalMembers(naysBlock) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Nay"})
		}

		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID: divID, Parliament: legislature, Session: session,
				Number: divNum, Date: date, Description: desc,
				Yeas: yeas, Nays: nays, Result: result,
				Chamber: "pei", DetailURL: pdfURL, LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
		divNum++
	}
	return results
}

func parsePEIJournalMembers(block string) []string {
	if strings.TrimSpace(block) == "" {
		return nil
	}
	parts := strings.Split(block, `\`)
	seen := make(map[string]bool)
	var names []string
	for _, part := range parts {
		for _, n := range peiExtractNamesFromChunk(part) {
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			names = append(names, n)
		}
	}
	return names
}

func peiExtractNamesFromChunk(chunk string) []string {
	chunk = strings.TrimSpace(chunk)
	chunk = strings.Join(strings.Fields(chunk), " ")
	chunk = strings.ReplaceAll(chunk, " - ", "-")
	chunk = regexp.MustCompile(`(?i)^(Nays?|Yeas?)\s*\(?\s*\d+\s*`).ReplaceAllString(chunk, "")
	if chunk == "" || strings.HasPrefix(chunk, "- ") {
		return nil
	}

	var results []string

	if peiPremierRe.MatchString(chunk) {
		chunk = strings.TrimSpace(peiPremierRe.ReplaceAllString(chunk, ""))
		if chunk == "" {
			return results
		}
	}

	for _, title := range peiTitlePrefixes {
		if strings.HasPrefix(chunk, title) {
			chunk = strings.TrimSpace(chunk[len(title):])
			break
		}
	}
	chunk = strings.TrimPrefix(chunk, "Hon. ")
	chunk = strings.TrimSpace(chunk)
	if chunk == "" {
		return results
	}
	lower := strings.ToLower(chunk)
	if strings.HasPrefix(lower, "the motion") || strings.HasPrefix(lower, "motion resolved") || strings.HasPrefix(lower, "ordered") || strings.HasPrefix(lower, "journal of") {
		return results
	}

	words := strings.Fields(chunk)
	if len(words) < 2 {
		return results
	}

	name := make([]string, 0, 3)
	for i, w := range words {
		clean := strings.Trim(w, ",.;:()")
		if clean == "" {
			continue
		}
		if i > 0 {
			if _, isRiding := peiRidingStartWords[clean]; isRiding && len(name) >= 2 {
				break
			}
		}
		if len(name) == 3 {
			break
		}
		name = append(name, clean)
		if len(name) >= 2 && i+1 < len(words) {
			next := strings.Trim(words[i+1], ",.;:()")
			if _, isRiding := peiRidingStartWords[next]; isRiding {
				break
			}
		}
	}

	if len(name) >= 2 {
		results = append(results, strings.Join(name, " "))
	}
	return results
}

// ParsePEIJournalDivisionsForTest is test-only access to the PEI journal parser.
func ParsePEIJournalDivisionsForTest(text, pdfURL string, legislature, session, startDivNum int, date string) []ProvincialDivisionResult {
	return parsePEIJournalDivisions(text, pdfURL, legislature, session, startDivNum, date)
}

// ── PEI votes ─────────────────────────────────────────────────────────────────

func crawlPEIVotesFromWorkflow(wdfBase string, year, legislature, session int, client *http.Client, delay time.Duration) ([]ProvincialDivisionResult, error) {
	queryVars := map[string]string{
		"wdf_url_query": "true",
	}
	if legislature > 0 && session > 0 {
		queryVars["search"] = "assembly"
		queryVars["general_assembly"] = strconv.Itoa(legislature)
		queryVars["session"] = strconv.Itoa(session)
	} else {
		queryVars["year"] = strconv.Itoa(year)
		queryVars["search"] = "year"
	}
	body, err := postPEIWorkflow(context.Background(), wdfBase, peiWorkflowJournals, peiWDFActivityJournals, queryVars, client, delay)
	if err != nil || body == nil {
		return nil, err
	}

	var resp wdfTreeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		log.Printf("[pe-votes] wdf tree decode: %v; falling back to HTML", err)
		return nil, nil
	}
	if resp.Data == nil {
		log.Printf("[pe-votes] wdf returned null data; falling back to HTML")
		return nil, nil
	}

	rows := wdfCollectRows(resp.Data)
	if len(rows) == 0 {
		log.Printf("[pe-votes] wdf returned 0 journal rows; falling back to HTML")
		return nil, nil
	}

	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	var results []ProvincialDivisionResult
	nextDivNum := 1
	for _, row := range rows {
		if len(row.Children) == 0 {
			continue
		}
		var link, date string
		if len(row.Children[0].Children) > 0 {
			lnk := row.Children[0].Children[0]
			if lnk.Type == "LinkV2" {
				var ld wdfLinkData
				if json.Unmarshal(lnk.Data, &ld) == nil {
					if ld.Href != nil && *ld.Href != "" {
						link = *ld.Href
					}
					date = utils.FindDateInText(ld.Text)
				}
			}
		}
		if link == "" {
			continue
		}
		if date == "" {
			date = extractDateFromURL(link)
		}

		if strings.Contains(link, "docs.assembly.pe.ca") {
			safeLink := link
			if u, uerr := neturl.Parse(link); uerr == nil {
				u.RawQuery = u.Query().Encode()
				safeLink = u.String()
			}
			text, terr := downloadAndExtractPDFText(safeLink, "pe", client)
			if terr != nil {
				log.Printf("[pe-votes] wdf journal %s: %v; skipping", link, terr)
				continue
			}
			parsed := parsePEIJournalDivisions(text, link, legislature, session, nextDivNum, date)
			nextDivNum += len(parsed)
			results = append(results, parsed...)
			time.Sleep(delay)
			continue
		}

		fullLink := resolveRelativeURL(peiAssemblyBase, link)
		doc, derr := fetchDoc(fullLink, client)
		if derr != nil {
			log.Printf("[pe-votes] wdf journal %s: %v", fullLink, derr)
			continue
		}
		parsed := parseGenericProvincialVotesDoc(doc, "pe", "pei", legislature, session, date)
		nextDivNum += len(parsed)
		results = append(results, parsed...)
		time.Sleep(delay)
	}

	log.Printf("[pe-votes] wdf parsed %d divisions from %d journals", len(results), len(rows))
	return results, nil
}

func crawlPEIVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	log.Printf("[pe-votes] fetching index: %s", indexURL)
	resp, err := client.Get(indexURL)
	if err != nil {
		return nil, fmt.Errorf("pe votes index: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("pe votes index read: %w", err)
	}

	if isPEICaptchaBody(body) {
		log.Printf("[pe-votes] CAPTCHA detected — assembly.pe.ca is protected by Radware bot-manager; returning 0 divisions.")
		return nil, nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("pe votes parse: %w", err)
	}
	links := discoverProvincialVoteLinksWithMatcher(doc, indexURL, peiVotesLinkRe)
	if len(links) == 0 {
		links = []string{indexURL}
	}
	var results []ProvincialDivisionResult
	for _, link := range links {
		dayDoc, derr := fetchDoc(link, client)
		if derr != nil {
			log.Printf("[pe-votes] skip day link %s: %v", link, derr)
			continue
		}
		date := extractDateFromURL(link)
		parsed := parseGenericProvincialVotesDoc(dayDoc, "pe", "pei", legislature, session, date)
		results = append(results, parsed...)
	}
	log.Printf("[pe-votes] parsed %d divisions", len(results))
	return results, nil
}

func crawlPrinceEdwardIslandVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	defaultURL := indexURL == ""
	if defaultURL {
		indexURL = peiJournalsIndexURL
	}
	delay := time.Duration(0)
	if client == nil {
		delay = peiDefaultDelay
		client = newPEIHTTPClient(delay)
	}

	wdfBase := peiWDFAPIBase
	if !defaultURL {
		wdfBase = indexURL
	}
	year := time.Now().Year()
	divs, err := crawlPEIVotesFromWorkflow(wdfBase, year, legislature, session, client, delay)
	if err == nil && len(divs) > 0 {
		return divs, nil
	}
	if err != nil {
		log.Printf("[pe-votes] wdf api: %v; falling back to HTML", err)
	}

	return crawlPEIVotes(indexURL, legislature, session, client)
}

// CrawlPrinceEdwardIslandVotes crawls PEI votes/proceedings pages.
func CrawlPrinceEdwardIslandVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlPrinceEdwardIslandVotes(indexURL, legislature, session, client)
}
