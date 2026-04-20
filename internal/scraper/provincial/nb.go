package provincial

import (
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/open-democracy/internal/utils"
)

// ── New Brunswick regexps ─────────────────────────────────────────────────────

var newBrunswickBillLinkRe = regexp.MustCompile(`(?i)(legis|bill|projet)`)
var newBrunswickJournalSessionLinkRe = regexp.MustCompile(`(?i)/en/house-business/journals/\d+/\d+/?$`)
var newBrunswickJournalPDFLinkRe = regexp.MustCompile(`(?i)\.pdf(?:\?.*)?$`)
var newBrunswickPDFVoteCountRe = regexp.MustCompile(`(?is)(?:YEAS?|POUR)\s*[:\-]?\s*(\d{1,3}).{0,280}?(?:NAYS?|CONTRE)\s*[:\-]?\s*(\d{1,3})`)
var newBrunswickVoteSectionRe = regexp.MustCompile(`(?is)(?:RECORDED\s+DIVISION\s+)?(YEAS?|POUR)\s*[-:–]\s*\d{1,3}\s+`)
var newBrunswickVoteCountPairRe = regexp.MustCompile(`(?is)(YEAS?|POUR)\s*[-:–]\s*(\d{1,3}).*?(NAYS?|CONTRE)\s*[-:–]\s*(\d{1,3})`)
var newBrunswickVotesLinkRe = regexp.MustCompile(`(?i)(journals?(?:-e\.asp|/)|house-business/journals|votes|legis)`)

// ── New Brunswick bills ───────────────────────────────────────────────────────

func crawlNewBrunswickBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if indexURL == "" {
		indexURL = "https://www.legnb.ca/en/legislation/bills"
	}
	return crawlProvincialBillsFromIndexWithMatcher(indexURL, "nb", legislature, session, "new_brunswick", client, newBrunswickBillLinkRe)
}

// CrawlNewBrunswickBills crawls New Brunswick bills pages.
func CrawlNewBrunswickBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	return crawlNewBrunswickBills(indexURL, legislature, session, client)
}

// ── New Brunswick votes ───────────────────────────────────────────────────────

func crawlNewBrunswickVotesFromPDF(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	indexDoc, err := fetchDoc(indexURL, client)
	if err != nil {
		return nil, err
	}

	sessionLinks := discoverNewBrunswickJournalSessionLinks(indexDoc, indexURL)
	if len(sessionLinks) == 0 {
		sessionLinks = []string{indexURL}
	}

	pdfLinks := make([]string, 0)
	seenPDF := make(map[string]bool)
	for _, sessionURL := range sessionLinks {
		doc, derr := fetchDoc(sessionURL, client)
		if derr != nil {
			log.Printf("[nb-votes] skip session %s: %v", sessionURL, derr)
			continue
		}
		for _, pdfURL := range discoverNewBrunswickJournalPDFLinks(doc, sessionURL) {
			if seenPDF[pdfURL] {
				continue
			}
			seenPDF[pdfURL] = true
			pdfLinks = append(pdfLinks, pdfURL)
		}
	}

	sort.Strings(pdfLinks)
	if len(pdfLinks) > 60 {
		pdfLinks = pdfLinks[len(pdfLinks)-60:]
	}
	if len(pdfLinks) == 0 {
		log.Printf("[nb-votes] no journal PDFs discovered; falling back to generic parser")
		return crawlGenericProvincialVotesWithMatcher(indexURL, "nb", "new_brunswick", legislature, session, client, newBrunswickVotesLinkRe)
	}

	results := make([]ProvincialDivisionResult, 0)
	nextDivNum := 1
	for _, pdfURL := range pdfLinks {
		divs, consumed, derr := crawlNewBrunswickJournalPDF(pdfURL, legislature, session, nextDivNum, client)
		if derr != nil {
			log.Printf("[nb-votes] skip pdf %s: %v", pdfURL, derr)
			continue
		}
		results = append(results, divs...)
		nextDivNum += consumed
	}
	if len(results) == 0 {
		log.Printf("[nb-votes] no divisions parsed from PDFs; falling back to generic parser")
		return crawlGenericProvincialVotesWithMatcher(indexURL, "nb", "new_brunswick", legislature, session, client, newBrunswickVotesLinkRe)
	}

	log.Printf("[nb-votes] parsed %d divisions from %d PDFs", len(results), len(pdfLinks))
	return results, nil
}

func discoverNewBrunswickJournalSessionLinks(doc *goquery.Document, baseURL string) []string {
	seen := make(map[string]bool)
	links := make([]string, 0)
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !newBrunswickJournalSessionLinkRe.MatchString(href) {
			return
		}
		full := resolveRelativeURL(baseURL, href)
		if seen[full] {
			return
		}
		seen[full] = true
		links = append(links, full)
	})
	sort.Strings(links)
	if len(links) > 6 {
		links = links[len(links)-6:]
	}
	return links
}

func discoverNewBrunswickJournalPDFLinks(doc *goquery.Document, baseURL string) []string {
	seen := make(map[string]bool)
	links := make([]string, 0)
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := normalizeHref(a.AttrOr("href", ""))
		if href == "" || !newBrunswickJournalPDFLinkRe.MatchString(href) {
			return
		}
		full := resolveRelativeURL(baseURL, href)
		if seen[full] {
			return
		}
		seen[full] = true
		links = append(links, full)
	})
	sort.Strings(links)
	return links
}

func crawlNewBrunswickJournalPDF(pdfURL string, legislature, session, startDivisionNumber int, client *http.Client) ([]ProvincialDivisionResult, int, error) {
	text, err := downloadAndExtractPDFText(pdfURL, "nb", client)
	if err != nil {
		return nil, 0, err
	}

	date := extractDateFromURL(pdfURL)
	if date == "" {
		date = utils.TodayISO()
	}
	parsed := parseNewBrunswickPDFDivisions(text, pdfURL, legislature, session, startDivisionNumber, date)
	return parsed, len(parsed), nil
}

func parseNewBrunswickPDFDivisions(text, detailURL string, legislature, session, startDivisionNumber int, date string) []ProvincialDivisionResult {
	sections := splitNewBrunswickVoteSections(text)
	blocks := make([][4]string, 0, len(sections))
	for _, section := range sections {
		m := newBrunswickVoteCountPairRe.FindStringSubmatch(section)
		if len(m) != 5 {
			continue
		}
		yeasBlock := section
		naysBlock := ""
		if split := regexp.MustCompile(`(?is)(NAYS?|CONTRE)\s*[-:–]\s*\d{1,3}\s+`).FindStringIndex(section); split != nil {
			yeasBlock = section[:split[0]]
			naysBlock = section[split[0]:]
		}
		blocks = append(blocks, [4]string{m[2], m[4], yeasBlock, naysBlock})
	}
	if len(blocks) == 0 {
		// Fallback to count-only extraction if block extraction misses a layout variant.
		matches := newBrunswickPDFVoteCountRe.FindAllStringSubmatchIndex(text, -1)
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
			divID := ProvincialDivisionID("nb", legislature, session, divNum, date)
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
					ID:          divID,
					Parliament:  legislature,
					Session:     session,
					Number:      divNum,
					Date:        date,
					Description: desc,
					Yeas:        yeas,
					Nays:        nays,
					Result:      result,
					Chamber:     "new_brunswick",
					DetailURL:   detailURL,
					LastScraped: utils.NowISO(),
				},
				Votes: nil,
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
		divID := ProvincialDivisionID("nb", legislature, session, divNum, date)
		desc := newBrunswickDescriptionFromContext(text, strings.Index(text, block[2]))
		if desc == "" {
			desc = "Recorded division"
		}

		result := "Carried"
		if nays > yeas {
			result = "Negatived"
		}

		votes := make([]ProvincialMemberVote, 0, yeas+nays)
		for _, name := range parseNewBrunswickVoteNames(block[2]) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Yea"})
		}
		for _, name := range parseNewBrunswickVoteNames(block[3]) {
			votes = append(votes, ProvincialMemberVote{DivisionID: divID, MemberName: name, Vote: "Nay"})
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
				Chamber:     "new_brunswick",
				DetailURL:   detailURL,
				LastScraped: utils.NowISO(),
			},
			Votes: votes,
		})
	}

	return results
}

func splitNewBrunswickVoteSections(text string) []string {
	idxs := newBrunswickVoteSectionRe.FindAllStringIndex(text, -1)
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

// ParseNewBrunswickPDFDivisionsForTest is test-only access to NB PDF parsing logic.
func ParseNewBrunswickPDFDivisionsForTest(text, detailURL string, legislature, session, startDivisionNumber int, date string) []ProvincialDivisionResult {
	return parseNewBrunswickPDFDivisions(text, detailURL, legislature, session, startDivisionNumber, date)
}

// CrawlNewBrunswickVotes crawls NB journals/votes pages.
func crawlNewBrunswickVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if indexURL == "" {
		indexURL = "https://www.legnb.ca/en/house-business/journals"
	}
	if client == nil {
		client = utils.NewHTTPClient()
	}
	return crawlNewBrunswickVotesFromPDF(indexURL, legislature, session, client)
}

// CrawlNewBrunswickVotes crawls New Brunswick votes/proceedings pages.
func CrawlNewBrunswickVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlNewBrunswickVotes(indexURL, legislature, session, client)
}

// parseNewBrunswickVoteNames extracts names from NB-style vote blocks (Hon. Mr./Ms. prefix format).
var newBrunswickNameTokenRe = regexp.MustCompile(`(?i)(?:Hon\.\s+)?(?:Mr\.|Ms\.)\s+(?:[A-Z]\.\s+)?[A-Z][A-Za-z\.'\-]+(?:\s*\-\s*[A-Z][A-Za-z\.'\-]+)*`)

func parseNewBrunswickVoteNames(blockText string) []string {
	if strings.TrimSpace(blockText) == "" {
		return nil
	}
	clean := strings.Join(strings.Fields(strings.ReplaceAll(blockText, "\u00a0", " ")), " ")
	nameMatches := newBrunswickNameTokenRe.FindAllString(clean, -1)
	if len(nameMatches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	results := make([]string, 0, len(nameMatches))
	for _, raw := range nameMatches {
		name := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
		if name == "" {
			continue
		}
		name = strings.ReplaceAll(name, " - ", "-")
		name = strings.TrimSpace(strings.TrimPrefix(name, "Hon. "))
		name = strings.TrimSpace(strings.TrimPrefix(name, "Mr. "))
		name = strings.TrimSpace(strings.TrimPrefix(name, "Ms. "))
		name = strings.TrimSpace(strings.TrimPrefix(name, "Dr. "))
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, name)
	}
	return results
}
