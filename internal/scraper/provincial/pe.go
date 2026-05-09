package provincial

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
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
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/utils"
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

	// Keep PE-specific internal delay disabled so bills/votes can be handed off
	// to summarization as quickly as possible; top-level crawler delay still applies.
	peiDefaultDelay = 0 * time.Second
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

// peBillIDPartsRe matches a PEI bill ID of the form "pe-<legislature>-<session>-<number>".
var peBillIDPartsRe = regexp.MustCompile(`(?i)^pe-(\d+)-(\d+)-([a-z0-9-]+)$`)

// wdfFindBillDocID searches the WDF response tree for the doc-ID associated with
// a given bill number. The bill number match is case-insensitive.
func wdfFindBillDocID(nodes []wdfNode, wantedBillNumber string) string {
	wantedBillNumber = strings.ToUpper(strings.TrimSpace(wantedBillNumber))
	for _, row := range wdfCollectRows(nodes) {
		if len(row.Children) < 2 || len(row.Children[0].Children) == 0 {
			continue
		}
		var number string
		var cd wdfCellData
		if json.Unmarshal(row.Children[1].Data, &cd) == nil && cd.Text != nil {
			number = strings.ToUpper(strings.TrimSpace(*cd.Text))
		}
		if number == "" || number != wantedBillNumber {
			continue
		}
		linkNode := row.Children[0].Children[0]
		if linkNode.Type != "LinkV2" {
			continue
		}
		var ld wdfLinkData
		if json.Unmarshal(linkNode.Data, &ld) != nil {
			continue
		}
		if id := strings.TrimSpace(ld.QueryParams["id"]); id != "" {
			return id
		}
	}
	return ""
}

// IsPEExpiredLinkResponse returns true when the HTTP response indicates that a
// PEI bill-document link has expired. This happens when the docs.assembly.pe.ca
// server returns HTTP 500 with an "Error retrieving file / link is expired" body.
func IsPEExpiredLinkResponse(rawURL string, statusCode int, snippet string) bool {
	if statusCode != http.StatusInternalServerError {
		return false
	}
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	if host != "docs.assembly.pe.ca" && !strings.HasPrefix(host, "127.0.0.1") && !strings.HasPrefix(host, "localhost") {
		return false
	}
	if !strings.Contains(strings.ToLower(u.Path), "/download/dms") {
		return false
	}
	lower := strings.ToLower(snippet)
	return strings.Contains(lower, "error retrieving file") && strings.Contains(lower, "link is expired")
}

// ResolveFreshPEBillTextURL contacts the PEI WDF workflow API to look up a new
// download URL for a bill whose stored URL has expired. billID must be a PEI bill
// ID of the form "pe-<legislature>-<session>-<billNumber>". wdfBase is the root
// of the WDF API server (e.g. "https://wdf.princeedwardisland.ca"); pass a local
// test-server URL in tests to avoid network calls.
func ResolveFreshPEBillTextURL(ctx context.Context, billID, wdfBase string) (string, error) {
	if strings.TrimSpace(billID) == "" {
		return "", fmt.Errorf("empty bill id")
	}
	matches := peBillIDPartsRe.FindStringSubmatch(strings.TrimSpace(billID))
	if len(matches) != 4 {
		return "", fmt.Errorf("not a PE bill id: %s", billID)
	}
	legislature, err := strconv.Atoi(matches[1])
	if err != nil {
		return "", err
	}
	session, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", err
	}
	billNumber := strings.ToUpper(strings.TrimSpace(matches[3]))
	if billNumber == "" {
		return "", fmt.Errorf("missing bill number")
	}

	searchParams := map[string]string{
		"search_bills":     "true",
		"wdf_url_query":    "true",
		"search":           "assembly",
		"general_assembly": strconv.Itoa(legislature),
		"session":          strconv.Itoa(session),
	}
	searchBody, err := postPEIWorkflowHTTP(ctx, wdfBase, peiWorkflowBills, peiWDFActivityBills, searchParams, nil, 0)
	if err != nil {
		return "", err
	}
	if searchBody == nil {
		return "", fmt.Errorf("PE WDF search returned no data for %s", billID)
	}
	var searchResp wdfTreeResponse
	if err := json.Unmarshal(searchBody, &searchResp); err != nil {
		return "", err
	}
	billDocID := wdfFindBillDocID(searchResp.Data, billNumber)
	if billDocID == "" {
		return "", fmt.Errorf("bill doc id not found for %s", billID)
	}

	viewBody, err := postPEIWorkflowHTTP(ctx, wdfBase, peiWorkflowBills, peiWDFActivityBillView, map[string]string{"id": billDocID}, nil, 0)
	if err != nil {
		return "", err
	}
	if viewBody == nil {
		return "", fmt.Errorf("PE WDF view returned no data for %s", billID)
	}
	var viewResp wdfTreeResponse
	if err := json.Unmarshal(viewBody, &viewResp); err != nil {
		return "", err
	}
	return firstWDFLinkHref(viewResp.Data), nil
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
		// Skip pei_fetch.js in production - it requires puppeteer-extra which may not be installed
		// and it's slow/unreliable (hangs for 30+ seconds). Use HTTP POST directly instead.
		if client == nil {
			clog.Infof("[pe-wdf] no HTTP client available for WDF API")
			return nil, nil
		}
		return postPEIWorkflowHTTP(ctx, wdfBase, workflowName, activityName, queryVars, client, delay)
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
		clog.Infof("[pe-wdf] pei_fetch.js: %v stderr=%s", err, stderr.String())
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
		clog.Infof("[pe-wdf] %s returned HTTP %d; will fall back to HTML", workflowName, resp.StatusCode)
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
		clog.Infof("[pe-bills] wdf tree decode: %v; falling back to HTML", err)
		return nil, nil
	}
	if resp.Data == nil {
		clog.Infof("[pe-bills] wdf returned null data; falling back to HTML")
		return nil, nil
	}

	rows := wdfCollectRows(resp.Data)
	if len(rows) == 0 {
		clog.Infof("[pe-bills] wdf returned 0 bill rows; falling back to HTML")
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
	clog.Debugf("[pe-bills] wdf parsed %d bills", len(out))
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
	if strings.HasPrefix(indexURL, "http://127.0.0.1") || strings.HasPrefix(indexURL, "http://localhost") {
		wdfBase = indexURL
	}
	year := time.Now().Year()
	bills, werr := crawlPEIBillsFromWorkflow(wdfBase, year, legislature, session, client, delay)
	if werr == nil && len(bills) > 0 {
		return bills, nil
	}
	if werr != nil {
		clog.Infof("[pe-bills] wdf api: %v; falling back to HTML", werr)
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
var peiDivisionBoilerplateRe = regexp.MustCompile(`(?i)^(?:and\s+the\s+question\s+being\s+put.*|the\s+question\s+being\s+put.*|hon\.\s+mr\.\s+speaker\s+put\s+the\s+question.*|motion\s+resolved.*|the\s+motion\s+was.*)$`)
var peiBillNoInContextRe = regexp.MustCompile(`(?i)\bBill\s+No\.?\s*(\d+\w*)`)

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

		desc := extractPEIDivisionDescription(text, trigger[0])

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

// peiBillDescriptionFromContext returns "TITLE (Bill N)" for the last
// "Bill No. N" match found in ctx, walking backward to collect the
// all-uppercase title words that precede it. Returns "" if no match.
func peiBillDescriptionFromContext(ctx string) string {
	matches := peiBillNoInContextRe.FindAllStringSubmatchIndex(ctx, -1)
	if len(matches) == 0 {
		return ""
	}
	// Use the LAST match (handles omnibus sessions where multiple bills appear).
	last := matches[len(matches)-1]
	billNo := ctx[last[2]:last[3]] // capture group 1 = bill number

	// Walk backward from the match start collecting all-uppercase words for the title.
	before := ctx[:last[0]]
	words := strings.Fields(before)
	var titleWords []string
	for i := len(words) - 1; i >= 0; i-- {
		w := words[i]
		// Stop at the first word that is not all-uppercase letters (allows hyphens).
		if !isAllCaps(w) {
			break
		}
		titleWords = append([]string{w}, titleWords...)
	}
	if len(titleWords) == 0 {
		return fmt.Sprintf("Bill %s", billNo)
	}
	return fmt.Sprintf("%s (Bill %s)", strings.Join(titleWords, " "), billNo)
}

// isAllCaps reports whether s consists entirely of uppercase ASCII letters and hyphens.
func isAllCaps(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '-' && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func extractPEIDivisionDescription(text string, triggerStart int) string {
	const contextWindow = 500
	const maxDescriptionLen = 240

	start := triggerStart - contextWindow
	if start < 0 {
		start = 0
	}
	ctx := strings.TrimSpace(strings.Join(strings.Fields(text[start:triggerStart]), " "))
	if ctx == "" {
		return "Recorded division"
	}

	parts := strings.FieldsFunc(ctx, func(r rune) bool {
		return r == '.' || r == ';'
	})
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(strings.Trim(parts[i], " ,:-"))
		if candidate == "" || peiDivisionBoilerplateRe.MatchString(candidate) {
			continue
		}
		if len(candidate) > maxDescriptionLen {
			candidate = strings.TrimSpace(candidate[:maxDescriptionLen])
		}
		if candidate != "" {
			return candidate
		}
	}

	return "Recorded division"
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
		clog.Debugf("[pe-votes] fetching journals for legislature=%d session=%d via WDF", legislature, session)
	} else {
		queryVars["year"] = strconv.Itoa(year)
		queryVars["search"] = "year"
		clog.Debugf("[pe-votes] fetching journals for year=%d via WDF", year)
	}
	body, err := postPEIWorkflow(context.Background(), wdfBase, peiWorkflowJournals, peiWDFActivityJournals, queryVars, client, delay)
	if err != nil || body == nil {
		clog.Infof("[pe-votes] WDF workflow returned no data: %v", err)
		return nil, err
	}
	clog.Debugf("[pe-votes] WDF returned %d bytes", len(body))

	var resp wdfTreeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		clog.Infof("[pe-votes] wdf tree decode: %v; falling back to HTML", err)
		return nil, nil
	}
	if resp.Data == nil {
		clog.Infof("[pe-votes] wdf returned null data; falling back to HTML")
		return nil, nil
	}

	rows := wdfCollectRows(resp.Data)
	clog.Debugf("[pe-votes] WDF returned %d journal rows", len(rows))
	if len(rows) == 0 {
		clog.Infof("[pe-votes] wdf returned 0 journal rows; falling back to HTML")
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
				clog.Infof("[pe-votes] wdf journal %s: %v; skipping", link, terr)
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
			clog.Infof("[pe-votes] wdf journal %s: %v", fullLink, derr)
			continue
		}
		parsed := parseGenericProvincialVotesDoc(doc, "pe", "pei", legislature, session, date)
		nextDivNum += len(parsed)
		results = append(results, parsed...)
		time.Sleep(delay)
	}

	clog.Debugf("[pe-votes] wdf parsed %d divisions from %d journals", len(results), len(rows))
	return results, nil
}

func crawlPEIVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	clog.Debugf("[pe-votes] fetching index: %s", indexURL)
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
		clog.Infof("[pe-votes] CAPTCHA detected — assembly.pe.ca is protected by Radware bot-manager; returning 0 divisions.")
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
			clog.Infof("[pe-votes] skip day link %s: %v", link, derr)
			continue
		}
		date := extractDateFromURL(link)
		parsed := parseGenericProvincialVotesDoc(dayDoc, "pe", "pei", legislature, session, date)
		results = append(results, parsed...)
	}
	clog.Debugf("[pe-votes] parsed %d divisions", len(results))
	return results, nil
}

func crawlPrinceEdwardIslandVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if indexURL == "" {
		indexURL = peiJournalsIndexURL
	}
	delay := time.Duration(0)
	if client == nil {
		delay = peiDefaultDelay
		client = newPEIHTTPClient(delay)
	}

	// Use the caller-supplied URL as the WDF base when running against a test server;
	// otherwise use the production WDF domain.
	wdfBase := peiWDFAPIBase
	if strings.HasPrefix(indexURL, "http://127.0.0.1") || strings.HasPrefix(indexURL, "http://localhost") {
		wdfBase = indexURL
	}
	year := time.Now().Year()
	results, err := crawlPEIVotesFromWorkflow(wdfBase, year, legislature, session, client, delay)
	if err != nil || len(results) > 0 {
		return results, err
	}
	return crawlPEIVotes(indexURL, legislature, session, client)
}

// CrawlPrinceEdwardIslandVotes crawls PEI votes/proceedings pages.
func CrawlPrinceEdwardIslandVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlPrinceEdwardIslandVotes(indexURL, legislature, session, client)
}

var peiCalendarPDFURLRe = regexp.MustCompile(`(?i)https?://[^\s"']*parliamentary[^\s"']*calendar[^\s"']*\.pdf`)
var peiMarchBreakRangeRe = regexp.MustCompile(`(?i)March\s+(\d{1,2})\s*[-–]\s*(\d{1,2}),\s*(\d{4})`)
var peiSessionOpeningDateRe = regexp.MustCompile(`(?i)opening\s+of\s+the\s+\d+(?:st|nd|rd|th)\s+Session[^\n]*?([A-Za-z]+\s+\d{1,2},\s+\d{4})`)
var peiDayDigitsOnlyRe = regexp.MustCompile(`^\d+$`)

// ExtractPEICalendarPDFURL extracts the PEI parliamentary calendar PDF URL from page HTML.
func ExtractPEICalendarPDFURL(pageHTML string, year int) string {
	urls := peiCalendarPDFURLRe.FindAllString(pageHTML, -1)
	if len(urls) == 0 {
		return ""
	}
	yearToken := strconv.Itoa(year)
	for _, u := range urls {
		if strings.Contains(u, yearToken) {
			return u
		}
	}
	return urls[0]
}

func PEICalendarDates(client *http.Client, pageHTML string, year int) ([]string, bool) {
	pdfURL := ExtractPEICalendarPDFURL(pageHTML, year)
	if pdfURL == "" {
		return nil, false
	}
	pdfBytes, err := FetchCalendarPDFBytes(client, pdfURL)
	if err != nil || len(pdfBytes) == 0 {
		return nil, false
	}
	return PEICalendarDatesFromPDFBytes(pdfBytes, year)
}

func PEICalendarDatesFromPDFBytes(pdfBytes []byte, year int) ([]string, bool) {
	return parsePEIHighlightedSittingDatesFromPDF(pdfBytes, year)
}

// ParsePEIDatesFromCalendarText derives likely PEI sitting dates from the calendar text.
func ParsePEIDatesFromCalendarText(text string, year int) []string {
	norm := normalizeCalendarText(text)
	springStart := fourthTuesdayInFebruary(year)
	if m := peiSessionOpeningDateRe.FindStringSubmatch(norm); len(m) == 2 {
		if t, err := time.Parse("January 2, 2006", m[1]); err == nil && t.Year() == year {
			springStart = dayStartUTC(t)
		}
	}
	fallStart := firstTuesdayInNovember(year)

	planning := map[string]struct{}{}
	addWeekdaysRange(planning, mondayOfWeek(springStart).AddDate(0, 0, -7), mondayOfWeek(springStart).AddDate(0, 0, -3))
	addWeekdaysRange(planning, mondayOfWeek(fallStart).AddDate(0, 0, -7), mondayOfWeek(fallStart).AddDate(0, 0, -3))
	if m := peiMarchBreakRangeRe.FindStringSubmatch(norm); len(m) == 4 {
		startDay, _ := strconv.Atoi(m[1])
		endDay, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		if y == year {
			start := dayStartUTC(time.Date(year, time.March, startDay, 0, 0, 0, 0, time.UTC))
			end := dayStartUTC(time.Date(year, time.March, endDay, 0, 0, 0, 0, time.UTC))
			addWeekdaysRange(planning, start, end)
		}
	}

	springEnd := dayStartUTC(time.Date(year, time.June, 30, 0, 0, 0, 0, time.UTC))
	fallEnd := dayStartUTC(time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC))

	seen := map[string]struct{}{}
	var out []string
	appendSittingDays := func(start, end time.Time) {
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			wd := d.Weekday()
			if wd < time.Tuesday || wd > time.Friday {
				continue
			}
			iso := d.Format("2006-01-02")
			if _, blocked := planning[iso]; blocked {
				continue
			}
			if _, ok := seen[iso]; ok {
				continue
			}
			seen[iso] = struct{}{}
			out = append(out, iso)
		}
	}
	appendSittingDays(springStart, springEnd)
	appendSittingDays(fallStart, fallEnd)
	sort.Strings(out)
	return out
}

func addWeekdaysRange(dst map[string]struct{}, start, end time.Time) {
	for d := dayStartUTC(start); !d.After(dayStartUTC(end)); d = d.AddDate(0, 0, 1) {
		wd := d.Weekday()
		if wd < time.Monday || wd > time.Friday {
			continue
		}
		dst[d.Format("2006-01-02")] = struct{}{}
	}
}

func mondayOfWeek(t time.Time) time.Time {
	t = dayStartUTC(t)
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return t.AddDate(0, 0, -(wd - 1))
}

func fourthTuesdayInFebruary(year int) time.Time {
	d := time.Date(year, time.February, 1, 0, 0, 0, 0, time.UTC)
	for d.Weekday() != time.Tuesday {
		d = d.AddDate(0, 0, 1)
	}
	return d.AddDate(0, 0, 21)
}

func firstTuesdayInNovember(year int) time.Time {
	d := time.Date(year, time.November, 1, 0, 0, 0, 0, time.UTC)
	for d.Weekday() != time.Tuesday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

func parsePEIHighlightedSittingDatesFromPDF(pdfBytes []byte, year int) ([]string, bool) {
	if !hasCommand("pdftoppm") {
		return nil, false
	}

	img, ok := renderCalendarPageImage(pdfBytes)
	if !ok {
		return nil, false
	}

	words, ok := extractPDFBBoxWordsAsOCRWords(pdfBytes, img.Bounds())
	if !ok {
		if !hasCommand("tesseract") {
			return nil, false
		}
		imgOCR, ocrWords, okOCR := renderAndOCRCalendarPage(pdfBytes)
		if !okOCR {
			return nil, false
		}
		img = imgOCR
		words = ocrWords
	}

	bounds := img.Bounds()
	if len(words) == 0 {
		return nil, false
	}

	isLikelyOCR := false
	for _, w := range words {
		if w.Confidence > 0 && w.Confidence < 100 {
			isLikelyOCR = true
			break
		}
	}

	maxCalendarY := bounds.Min.Y + int(float64(bounds.Dy())*0.78)
	dayWords := make([]ocrWord, 0, len(words))
	xCenters := make([]float64, 0, len(words))
	yCenters := make([]float64, 0, len(words))
	for _, w := range words {
		if isLikelyOCR && w.Confidence < 30 {
			continue
		}
		appendPEIDayWordCandidates(&dayWords, &xCenters, &yCenters, w, maxCalendarY, true)
	}
	if len(dayWords) < 40 {
		dayWords = dayWords[:0]
		xCenters = xCenters[:0]
		yCenters = yCenters[:0]
		for _, w := range words {
			if isLikelyOCR && w.Confidence < 30 {
				continue
			}
			appendPEIDayWordCandidates(&dayWords, &xCenters, &yCenters, w, maxCalendarY, false)
		}
		if len(dayWords) < 40 {
			if !isLikelyOCR && hasCommand("tesseract") {
				imgOCR, ocrWords, okOCR := renderAndOCRCalendarPage(pdfBytes)
				if okOCR {
					img = imgOCR
					bounds = img.Bounds()
					maxCalendarY = bounds.Min.Y + int(float64(bounds.Dy())*0.78)
					dayWords = dayWords[:0]
					xCenters = xCenters[:0]
					yCenters = yCenters[:0]
					for _, ow := range ocrWords {
						if ow.Confidence < 30 {
							continue
						}
						appendPEIDayWordCandidates(&dayWords, &xCenters, &yCenters, ow, maxCalendarY, true)
					}
					if len(dayWords) < 40 {
						dayWords = dayWords[:0]
						xCenters = xCenters[:0]
						yCenters = yCenters[:0]
						for _, ow := range ocrWords {
							if ow.Confidence < 30 {
								continue
							}
							appendPEIDayWordCandidates(&dayWords, &xCenters, &yCenters, ow, maxCalendarY, false)
						}
					}
				}
			}
			if len(dayWords) < 40 {
				return nil, false
			}
		}
	}

	colCenters, ok := cluster1D(xCenters, 3)
	if !ok {
		return nil, false
	}
	rowCenters, ok := cluster1D(yCenters, 4)
	if !ok {
		return nil, false
	}
	sort.Float64s(colCenters)
	sort.Float64s(rowCenters)

	greenWeeks := map[time.Time]struct{}{}
	holidayDates := map[string]struct{}{}
	for _, w := range dayWords {
		cx := float64(w.Left + w.Width/2)
		cy := float64(w.Top + w.Height/2)
		col := nearestClusterIndex(cx, colCenters)
		row := nearestClusterIndex(cy, rowCenters)
		month := row*3 + col + 1
		if month < 1 || month > 12 {
			continue
		}
		date := time.Date(year, time.Month(month), w.ParsedNumber, 0, 0, 0, 0, time.UTC)
		if date.Month() != time.Month(month) {
			continue
		}

		cell := image.Rect(w.Left-8, w.Top-8, w.Left+w.Width+8, w.Top+w.Height+8).Intersect(bounds)
		if cell.Empty() {
			continue
		}
		green, violet := classifyPEICalendarCellColors(img, cell)
		if !green {
			continue
		}
		weekStart := mondayOfWeek(date)
		greenWeeks[weekStart] = struct{}{}
		if violet {
			holidayDates[date.Format("2006-01-02")] = struct{}{}
		}
	}

	if len(greenWeeks) == 0 {
		return nil, false
	}

	seen := map[string]struct{}{}
	var out []string
	for weekStart := range greenWeeks {
		for offset := 1; offset <= 4; offset++ {
			d := weekStart.AddDate(0, 0, offset)
			if d.Year() != year {
				continue
			}
			iso := d.Format("2006-01-02")
			if _, holiday := holidayDates[iso]; holiday {
				continue
			}
			if _, exists := seen[iso]; exists {
				continue
			}
			seen[iso] = struct{}{}
			out = append(out, iso)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	sort.Strings(out)
	return out, true
}

func appendPEIDayWordCandidates(dayWords *[]ocrWord, xCenters *[]float64, yCenters *[]float64, w ocrWord, maxCalendarY int, enforceY bool) {
	text := strings.TrimSpace(w.Text)
	if text == "" {
		return
	}

	appendCandidate := func(c ocrWord) {
		cy := c.Top + c.Height/2
		if enforceY && cy > maxCalendarY {
			return
		}
		*dayWords = append(*dayWords, c)
		*xCenters = append(*xCenters, float64(c.Left+c.Width/2))
		*yCenters = append(*yCenters, float64(cy))
	}

	if n, err := strconv.Atoi(text); err == nil {
		if n < 1 || n > 31 {
			if !peiDayDigitsOnlyRe.MatchString(text) {
				return
			}
			ns := splitCalendarDayToken(text)
			if len(ns) == 0 {
				return
			}
			segW := w.Width / len(ns)
			if segW < 1 {
				segW = 1
			}
			for i, v := range ns {
				c := w
				c.Text = strconv.Itoa(v)
				c.ParsedNumber = v
				c.Left = w.Left + i*segW
				c.Width = segW
				appendCandidate(c)
			}
			return
		}
		w.ParsedNumber = n
		appendCandidate(w)
	}
}

func splitCalendarDayToken(s string) []int {
	if !peiDayDigitsOnlyRe.MatchString(s) || len(s) <= 1 {
		return nil
	}
	type key struct {
		idx  int
		prev int
	}
	type best struct {
		score int
		vals  []int
		ok    bool
	}
	memo := map[key]best{}
	var dfs func(i, prev int) best
	dfs = func(i, prev int) best {
		k := key{idx: i, prev: prev}
		if b, ok := memo[k]; ok {
			return b
		}
		if i == len(s) {
			return best{score: 0, vals: []int{}, ok: true}
		}
		out := best{score: -1, ok: false}
		for _, ln := range []int{1, 2} {
			if i+ln > len(s) {
				continue
			}
			n, err := strconv.Atoi(s[i : i+ln])
			if err != nil || n < 1 || n > 31 {
				continue
			}
			next := dfs(i+ln, n)
			if !next.ok {
				continue
			}
			bonus := 1
			if prev > 0 && n == prev+1 {
				bonus += 100
			}
			score := bonus + next.score
			if !out.ok || score > out.score {
				vals := make([]int, 0, 1+len(next.vals))
				vals = append(vals, n)
				vals = append(vals, next.vals...)
				out = best{score: score, vals: vals, ok: true}
			}
		}
		memo[k] = out
		return out
	}
	b := dfs(0, -1)
	if !b.ok || len(b.vals) < 2 {
		return nil
	}
	return b.vals
}

func isPEIGreenLike(c color.NRGBA) bool {
	return c.G >= 95 && int(c.G)-int(c.R) >= 18 && int(c.G)-int(c.B) >= 12
}

func isOliveLike(c color.NRGBA) bool {
	return int(c.R) >= 100 && int(c.G) >= 100 &&
		int(c.R)-int(c.B) >= 40 && int(c.G)-int(c.B) >= 40 &&
		abs(int(c.R)-int(c.G)) <= 20
}

func classifyPEICalendarCellColors(img image.Image, rect image.Rectangle) (sitting bool, violet bool) {
	total := 0
	sittingCount := 0
	violetCount := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r16, g16, b16, _ := img.At(x, y).RGBA()
			c := color.NRGBA{R: uint8(r16 >> 8), G: uint8(g16 >> 8), B: uint8(b16 >> 8), A: 255}
			total++
			if isOliveLike(c) || isPEIGreenLike(c) {
				sittingCount++
			}
			if isVioletLike(c) {
				violetCount++
			}
		}
	}
	if total == 0 {
		return false, false
	}
	sitting = float64(sittingCount)/float64(total) >= 0.08
	violet = violetCount >= 8 && float64(violetCount)/float64(total) >= 0.01
	return sitting, violet
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func isVioletLike(c color.NRGBA) bool {
	if c.R < 90 || c.B < 90 {
		return false
	}
	if c.G > 130 {
		return false
	}
	deltaRB := int(c.R) - int(c.B)
	if deltaRB < 0 {
		deltaRB = -deltaRB
	}
	return deltaRB <= 70
}
