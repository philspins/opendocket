package provincial

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/opendocket/internal/utils"
)

// ── Quebec regexps ────────────────────────────────────────────────────────────

var quebecBillLinkRe = regexp.MustCompile(`(?i)(travaux-parlementaires|projets-de-loi|bill)`)
var quebecVotesLinkRe = regexp.MustCompile(`(?i)(registre-des-votes|registre-votes|votes-nominaux|votes\.html|votes-appels-nominaux|/votes(?:/|$))`)

// ── Quebec types ──────────────────────────────────────────────────────────────

type quebecVoteListing struct {
	DateVote string `json:"DateVote"`
	Titre    string `json:"Titre"`
	Numero   string `json:"Numero"`
	VoteURL  string `json:"VoteURL"`
}

type quebecVotesSearchData struct {
	NumeroPage         int                 `json:"NumeroPage"`
	QuantiteParPage    int                 `json:"QuantiteParPage"`
	NombreTotalDonnees int                 `json:"NombreTotalDonnees"`
	NomRequete         string              `json:"NomRequete"`
	Donnees            []quebecVoteListing `json:"Donnees"`
}

type quebecVotesEnvelope struct {
	D quebecVotesSearchData `json:"d"`
}

// ── Quebec bills ──────────────────────────────────────────────────────────────

func crawlQuebecBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if indexURL == "" {
		indexURL = "https://www.assnat.qc.ca/en/travaux-parlementaires/projets-loi/index.html"
	}
	return crawlProvincialBillsFromIndexWithMatcher(indexURL, "qc", legislature, session, "quebec", client, quebecBillLinkRe)
}

// CrawlQuebecBills crawls Quebec bills pages.
func CrawlQuebecBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlQuebecBills(indexURL, legislature, session, client)
}

// ── Quebec votes ──────────────────────────────────────────────────────────────

func quebecSessionLegislatureValue(doc *goquery.Document, legislature, session int) string {
	if doc == nil {
		return ""
	}
	legRe := regexp.MustCompile(fmt.Sprintf(`(?i)\b%d(?:st|nd|rd|th)?\s+legislature\b`, legislature))
	sessionRe := regexp.MustCompile(fmt.Sprintf(`(?i)\b%d(?:st|nd|rd|th)?\s+session\b`, session))

	fallback := ""
	doc.Find("select.sessionLegislature option").Each(func(_ int, opt *goquery.Selection) {
		if fallback != "" {
			return
		}
		value, _ := opt.Attr("value")
		value = strings.TrimSpace(value)
		if value != "" && value != "-1" {
			fallback = value
		}
	})

	resolved := ""
	doc.Find("select.sessionLegislature option").Each(func(_ int, opt *goquery.Selection) {
		if resolved != "" {
			return
		}
		value, _ := opt.Attr("value")
		value = strings.TrimSpace(value)
		if value == "" || value == "-1" {
			return
		}
		title, _ := opt.Attr("title")
		text := strings.TrimSpace(title + " " + opt.Text())
		if legRe.MatchString(text) && sessionRe.MatchString(text) {
			resolved = value
		}
	})

	if resolved != "" {
		return resolved
	}
	return fallback
}

func quebecVotesEndpoint(indexURL, endpointPath string) string {
	base := "https://www.assnat.qc.ca"
	if u, err := neturl.Parse(indexURL); err == nil && u.Scheme != "" && u.Host != "" {
		base = u.Scheme + "://" + u.Host
	}
	return base + endpointPath
}

func quebecSearchVotes(indexURL, sessionLegislature string, page, perPage int, refresh bool, client *http.Client) (quebecVotesSearchData, error) {
	payload := map[string]string{
		"motsCles":                 "",
		"sessionLegislature":       sessionLegislature,
		"colonneTri":               "thDefaut",
		"directionTri":             "1",
		"numPage":                  strconv.Itoa(page),
		"quantiteParPage":          strconv.Itoa(perPage),
		"codeLangue":               "en",
		"rafraichirEtatPagination": strconv.FormatBool(refresh),
	}
	var envelope quebecVotesEnvelope
	if err := quebecPostJSON(client, indexURL, "/Gabarits/RegistreDesVotes.aspx/Rechercher", payload, &envelope); err != nil {
		return quebecVotesSearchData{}, fmt.Errorf("qc votes search: %w", err)
	}
	if envelope.D.QuantiteParPage <= 0 {
		envelope.D.QuantiteParPage = perPage
	}
	return envelope.D, nil
}

func quebecPaginateVotes(indexURL, queryName string, page, perPage int, client *http.Client) (quebecVotesSearchData, error) {
	payload := map[string]string{
		"nomRequete":      queryName,
		"numPage":         strconv.Itoa(page),
		"quantiteParPage": strconv.Itoa(perPage),
		"codeLangue":      "en",
	}
	var envelope quebecVotesEnvelope
	if err := quebecPostJSON(client, indexURL, "/Gabarits/RegistreDesVotes.aspx/PaginerRecherche", payload, &envelope); err != nil {
		return quebecVotesSearchData{}, fmt.Errorf("qc votes paginate page=%d: %w", page, err)
	}
	if envelope.D.QuantiteParPage <= 0 {
		envelope.D.QuantiteParPage = perPage
	}
	return envelope.D, nil
}

func quebecPostJSON(client *http.Client, indexURL, endpointPath string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := quebecVotesEndpoint(indexURL, endpointPath)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", indexURL)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("POST %q: status %d - %s", url, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}

func parseQuebecVoteDetailDoc(doc *goquery.Document, divisionID string) ([]ProvincialMemberVote, int, int) {
	yeas, _ := strconv.Atoi(strings.TrimSpace(doc.Find("#nbPour").AttrOr("value", "0")))
	nays, _ := strconv.Atoi(strings.TrimSpace(doc.Find("#nbContre").AttrOr("value", "0")))

	votes := make([]ProvincialMemberVote, 0, yeas+nays)
	seen := make(map[string]bool)
	appendPanel := func(selector, vote string) {
		doc.Find(selector).Each(func(_ int, member *goquery.Selection) {
			name := strings.TrimSpace(member.Find("span.nom").First().Text())
			if name == "" {
				name = strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(member.Text(), "\u00a0", " ")), " "))
			}
			if name == "" {
				return
			}
			key := vote + "|" + strings.ToLower(name)
			if seen[key] {
				return
			}
			seen[key] = true
			votes = append(votes, ProvincialMemberVote{DivisionID: divisionID, MemberName: name, Vote: vote})
		})
	}

	appendPanel("#ctl00_ColCentre_ContenuColonneGauche_pnlPour .depute", "Yea")
	appendPanel("#ctl00_ColCentre_ContenuColonneGauche_pnlContre .depute", "Nay")
	return votes, yeas, nays
}

// CrawlQuebecVotes crawls Quebec registre/votes pages.
func crawlQuebecVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if indexURL == "" {
		indexURL = "https://www.assnat.qc.ca/en/travaux-parlementaires/registre-des-votes/index.html"
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}

	indexDoc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, fmt.Errorf("qc votes index: %w", err)
	}

	sessionLegislature := quebecSessionLegislatureValue(indexDoc, legislature, session)
	if sessionLegislature == "" {
		log.Printf("[qc-votes] sessionLegislature not found; falling back to generic parser")
		return crawlGenericProvincialVotesWithMatcher(indexURL, "qc", "quebec", legislature, session, client, quebecVotesLinkRe)
	}

	firstPage, err := quebecSearchVotes(indexURL, sessionLegislature, 0, 25, true, client)
	if err != nil {
		log.Printf("[qc-votes] JSON search failed (%v); falling back to generic parser", err)
		return crawlGenericProvincialVotesWithMatcher(indexURL, "qc", "quebec", legislature, session, client, quebecVotesLinkRe)
	}

	votes := append([]quebecVoteListing{}, firstPage.Donnees...)
	if firstPage.NombreTotalDonnees > len(firstPage.Donnees) {
		totalPages := (firstPage.NombreTotalDonnees + firstPage.QuantiteParPage - 1) / firstPage.QuantiteParPage
		for page := 1; page < totalPages; page++ {
			nextPage, perr := quebecPaginateVotes(indexURL, firstPage.NomRequete, page, firstPage.QuantiteParPage, client)
			if perr != nil {
				log.Printf("[qc-votes] pagination page=%d failed: %v", page, perr)
				continue
			}
			votes = append(votes, nextPage.Donnees...)
		}
	}

	results := make([]ProvincialDivisionResult, 0, len(votes))
	fallbackNum := 0
	for _, v := range votes {
		fallbackNum++
		divNum, _ := strconv.Atoi(strings.TrimSpace(v.Numero))
		if divNum <= 0 {
			divNum = fallbackNum
		}

		detailURL := resolveRelativeURL(indexURL, strings.TrimSpace(v.VoteURL))
		if detailURL == "" {
			continue
		}

		date := strings.TrimSpace(v.DateVote)
		if date == "" {
			date = extractDateFromURL(detailURL)
		}
		if date == "" {
			date = utils.TodayISO()
		}

		detailDoc, derr := fetchDoc(detailURL, client)
		if derr != nil {
			log.Printf("[qc-votes] skip vote detail %s: %v", detailURL, derr)
			continue
		}

		divID := ProvincialDivisionID("qc", legislature, session, divNum, date)
		memberVotes, yeas, nays := parseQuebecVoteDetailDoc(detailDoc, divID)

		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		results = append(results, ProvincialDivisionResult{
			Division: DivisionStub{
				ID:          divID,
				Parliament:  legislature,
				Session:     session,
				Number:      divNum,
				Date:        date,
				Description: strings.TrimSpace(strings.Join(strings.Fields(v.Titre), " ")),
				Yeas:        yeas,
				Nays:        nays,
				Result:      result,
				Chamber:     "quebec",
				DetailURL:   detailURL,
				LastScraped: utils.NowISO(),
			},
			Votes: memberVotes,
		})
	}

	log.Printf("[qc-votes] parsed %d divisions", len(results))
	return results, nil
}

// CrawlQuebecVotes crawls Quebec votes pages.
func CrawlQuebecVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlQuebecVotes(indexURL, legislature, session, client)
}

// QuebecCalendarDatesFromPDF extracts Quebec assembly sitting dates from the calendar PDF.
func QuebecCalendarDatesFromPDF(pdfBytes []byte, year int) ([]string, bool) {
	text, err := extractPDFTextPages(pdfBytes, 2, 2)
	if err != nil || strings.TrimSpace(text) == "" {
		var err2 error
		text, err2 = extractTextWithPDFToText(pdfBytes)
		if err2 != nil || strings.TrimSpace(text) == "" {
			return nil, false
		}
	}
	return ParseQCScheduleText(text, year)
}

var qcDateRangeLongRe = regexp.MustCompile(`(\d{1,2})\s+(\S+)\s+au\s+(\d{1,2})\s+(\S+)\s+(\d{4})`)
var qcDateRangeShortRe = regexp.MustCompile(`(\d{1,2})\s+au\s+(\d{1,2})\s+(\S+)\s+(\d{4})`)

// ParseQCScheduleText parses Quebec assembly schedule text into sitting dates.
func ParseQCScheduleText(text string, year int) ([]string, bool) {
	textLower := strings.ToLower(text)
	intensifIdx := strings.Index(textLower, "intensif")

	var regularSection, intensiveSection string
	if intensifIdx >= 0 {
		regularSection = text[:intensifIdx]
		intensiveSection = text[intensifIdx:]
	} else {
		regularSection = text
	}

	seen := map[string]struct{}{}

	for _, rng := range parseQCDateRanges(regularSection, year) {
		addSpecificWeekdays(seen, rng.start, rng.end,
			[]time.Weekday{time.Tuesday, time.Wednesday, time.Thursday})
	}
	if intensiveSection != "" {
		for _, rng := range parseQCDateRanges(intensiveSection, year) {
			addSpecificWeekdays(seen, rng.start, rng.end,
				[]time.Weekday{time.Tuesday, time.Wednesday, time.Thursday, time.Friday})
		}
	}

	if len(seen) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, true
}

type qcDateRange struct{ start, end time.Time }

func parseQCDateRanges(text string, year int) []qcDateRange {
	var ranges []qcDateRange

	for _, m := range qcDateRangeLongRe.FindAllStringSubmatch(text, -1) {
		startDay, _ := strconv.Atoi(m[1])
		startMonth := parseFrenchMonth(m[2])
		endDay, _ := strconv.Atoi(m[3])
		endMonth := parseFrenchMonth(m[4])
		rangeYear, _ := strconv.Atoi(m[5])
		if rangeYear != year || startMonth == 0 || endMonth == 0 {
			continue
		}
		start := time.Date(year, time.Month(startMonth), startDay, 0, 0, 0, 0, time.UTC)
		end := time.Date(year, time.Month(endMonth), endDay, 0, 0, 0, 0, time.UTC)
		if !start.After(end) {
			ranges = append(ranges, qcDateRange{start, end})
		}
	}

	for _, m := range qcDateRangeShortRe.FindAllStringSubmatch(text, -1) {
		startDay, _ := strconv.Atoi(m[1])
		endDay, _ := strconv.Atoi(m[2])
		month := parseFrenchMonth(m[3])
		rangeYear, _ := strconv.Atoi(m[4])
		if rangeYear != year || month == 0 {
			continue
		}
		start := time.Date(year, time.Month(month), startDay, 0, 0, 0, 0, time.UTC)
		end := time.Date(year, time.Month(month), endDay, 0, 0, 0, 0, time.UTC)
		if !start.After(end) {
			ranges = append(ranges, qcDateRange{start, end})
		}
	}
	return ranges
}

var frenchMonthSubstrs = []struct {
	sub   string
	month int
}{
	{"janv", 1},
	{"vrier", 2},
	{"mars", 3},
	{"avri", 4},
	{"mai", 5},
	{"juin", 6},
	{"juil", 7},
	{"ao", 8},
	{"sept", 9},
	{"octo", 10},
	{"novem", 11},
	{"cembre", 12},
}

func parseFrenchMonth(s string) int {
	s = strings.ToLower(s)
	for _, e := range frenchMonthSubstrs {
		if strings.Contains(s, e.sub) {
			return e.month
		}
	}
	return 0
}

func addSpecificWeekdays(seen map[string]struct{}, start, end time.Time, weekdays []time.Weekday) {
	wdSet := map[time.Weekday]bool{}
	for _, wd := range weekdays {
		wdSet[wd] = true
	}
	for d := dayStartUTC(start); !d.After(end); d = d.AddDate(0, 0, 1) {
		if wdSet[d.Weekday()] {
			seen[d.Format("2006-01-02")] = struct{}{}
		}
	}
}
