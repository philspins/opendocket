package provincial

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/utils"
	"golang.org/x/sync/errgroup"
)

var nonAlnumForIDRe = regexp.MustCompile(`[^a-z0-9]+`)

// ProvincialSource defines one province-specific crawl configuration.
type ProvincialSource struct {
	Code     string
	Province string
	Chamber  string
	BillsURL string
	VotesURL string
	Special  string // "on" | "sk" | ""
}

// BillSummaryEnqueue is an optional callback used to feed crawled provincial
// bills into the AI summarization pipeline without creating package cycles.
type BillSummaryEnqueue func(billID, billTitle, fullTextURL, lastActivityDate string)

// MemberSeeder is an optional callback that seeds provincial members into the
// DB when fewer than 10 are present. Pass nil to skip seeding (e.g. in tests).
type MemberSeeder func(conn *sql.DB, code, province string, client *http.Client, delay time.Duration)

// ProvincialSources is the default source matrix used by CrawlProvincial.
var ProvincialSources = []ProvincialSource{
	{Code: "ab", Province: "Alberta", Chamber: "alberta", BillsURL: "https://www.assembly.ab.ca/assembly-business/bills/bill-status", VotesURL: "https://www.assembly.ab.ca/assembly-business/assembly-records/votes-and-proceedings"},
	{Code: "bc", Province: "British Columbia", Chamber: "british_columbia", BillsURL: "https://www.leg.bc.ca/parliamentary-business/bills-and-legislation", VotesURL: ""},
	{Code: "mb", Province: "Manitoba", Chamber: "manitoba", BillsURL: "https://web2.gov.mb.ca/bills/sess/index.php", VotesURL: "https://www.gov.mb.ca/legislature/business/votes_proceedings.html"},
	{Code: "nb", Province: "New Brunswick", Chamber: "new_brunswick", BillsURL: "https://www.legnb.ca/en/legislation/bills", VotesURL: "https://www.legnb.ca/en/house-business/journals"},
	{Code: "nl", Province: "Newfoundland and Labrador", Chamber: "newfoundland_labrador", BillsURL: "https://www.assembly.nl.ca/HouseBusiness/Bills/", VotesURL: "https://www.assembly.nl.ca/HouseBusiness/Journals/"},
	{Code: "ns", Province: "Nova Scotia", Chamber: "nova_scotia", BillsURL: "https://nslegislature.ca/legislative-business/bills-statutes/bills", VotesURL: "https://nslegislature.ca/legislative-business/journals"},
	{Code: "on", Province: "Ontario", Chamber: "ontario", BillsURL: "https://www.ola.org/en/legislative-business/bills/current", VotesURL: OntarioVPIndexURL, Special: "on"},
	{Code: "pe", Province: "Prince Edward Island", Chamber: "pei", BillsURL: "https://www.assembly.pe.ca/legislative-business/house-records/bills", VotesURL: "https://www.assembly.pe.ca/legislative-business/house-records/journals"},
	{Code: "qc", Province: "Quebec", Chamber: "quebec", BillsURL: "https://www.assnat.qc.ca/en/travaux-parlementaires/projets-loi/index.html", VotesURL: "https://www.assnat.qc.ca/en/travaux-parlementaires/registre-des-votes/index.html"},
	{Code: "sk", Province: "Saskatchewan", Chamber: "saskatchewan", BillsURL: "https://www.legassembly.sk.ca/legislative-business/bills/", VotesURL: SaskatchewanArchiveURL, Special: "sk"},
}

type provincialCrawlStats struct {
	BillsSeen            int
	BillsUpserted        int
	DivisionsSeen        int
	DivisionsUpserted    int
	MemberVotesSeen      int
	MemberVotesUpserted  int
	MemberVotesUnmatched int
	Errors               int
}

const provinceSubcrawlParallelism = 3

// ProvinceCrawler defines a province-specific bills/votes crawler pair.
type ProvinceCrawler interface {
	CrawlBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error)
	CrawlVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error)
}

type provinceCrawlerFuncs struct {
	bills func(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error)
	votes func(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error)
}

func (c provinceCrawlerFuncs) CrawlBills(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if c.bills == nil {
		return nil, nil
	}
	return c.bills(indexURL, legislature, session, client)
}

func (c provinceCrawlerFuncs) CrawlVotes(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if c.votes == nil {
		return nil, nil
	}
	return c.votes(indexURL, legislature, session, client)
}

var provinceCrawlers = map[string]ProvinceCrawler{
	"ab": provinceCrawlerFuncs{bills: CrawlAlbertaBills, votes: CrawlAlbertaVotes},
	"bc": provinceCrawlerFuncs{bills: CrawlBritishColumbiaBills, votes: CrawlBritishColumbiaVotes},
	"mb": provinceCrawlerFuncs{bills: CrawlManitobaBills, votes: CrawlManitobaVotes},
	"nb": provinceCrawlerFuncs{bills: CrawlNewBrunswickBills, votes: CrawlNewBrunswickVotes},
	"nl": provinceCrawlerFuncs{bills: CrawlNewfoundlandAndLabradorBills, votes: CrawlNewfoundlandAndLabradorVotes},
	"ns": provinceCrawlerFuncs{bills: CrawlNovaScotiaBills, votes: CrawlNovaScotiaVotes},
	"on": provinceCrawlerFuncs{bills: CrawlOntarioBills},
	"pe": provinceCrawlerFuncs{
		bills: func(indexURL string, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
			return CrawlPrinceEdwardIslandBills(indexURL, legislature, session, peiSourceClient(indexURL, client))
		},
		votes: func(indexURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
			return CrawlPrinceEdwardIslandVotes(indexURL, legislature, session, peiSourceClient(indexURL, client))
		},
	},
	"qc": provinceCrawlerFuncs{bills: CrawlQuebecBills, votes: CrawlQuebecVotes},
	"sk": provinceCrawlerFuncs{bills: CrawlSaskatchewanBills},
}

// SessionPlan holds the raw network output for one legislature/session crawl.
type SessionPlan struct {
	Legislature int
	Session     int
	Bills       []ProvincialBillStub
	Divisions   []ProvincialDivisionResult
}

// CrawlPlan is the output of the network discovery phase for one province source.
type CrawlPlan struct {
	Source   ProvincialSource
	Sessions []SessionPlan
}

// BuildCrawlPlan performs the network discovery phase for src: it resolves the
// current legislature/session, crawls bills and votes for each session, and
// returns a plan ready to pass to ExecuteCrawlPlan.
// conn is used read-only to guide fallback decisions (e.g. seeding previous session).
func BuildCrawlPlan(conn *sql.DB, client *http.Client, delay time.Duration, src ProvincialSource) (CrawlPlan, error) {
	legislature, currentSession := resolveProvincialLegislatureSession(conn, src, client)
	sessions := sessionsToCrawlForSource(src, currentSession)
	if len(sessions) == 1 {
		clog.Debugf("[provincial][%s] detected legislature/session: %d/%d", src.Code, legislature, currentSession)
	} else {
		clog.Debugf("[provincial][%s] detected legislature/current session: %d/%d; crawling sessions %v", src.Code, legislature, currentSession, sessions)
	}

	plan := CrawlPlan{Source: src}
	allowPreviousSessionFallback := len(sessions) == 1
	for _, session := range sessions {
		clog.Debugf("[provincial][%s] crawling legislature/session: %d/%d", src.Code, legislature, session)
		sp, err := buildSessionPlan(conn, client, delay, src, legislature, session, allowPreviousSessionFallback)
		if err != nil {
			return plan, err
		}
		plan.Sessions = append(plan.Sessions, sp)
	}
	return plan, nil
}

// buildSessionPlan performs the network discovery for a single legislature/session.
// conn is used read-only for previous-session fallback checks.
func buildSessionPlan(conn *sql.DB, client *http.Client, delay time.Duration, src ProvincialSource, legislature, session int, allowPreviousSessionFallback bool) (SessionPlan, error) {
	sp := SessionPlan{Legislature: legislature, Session: session}

	bills, berr := crawlBillsForSource(src, legislature, session, client)
	if allowPreviousSessionFallback && berr == nil && len(bills) == 0 && session > 1 && provinceBillCountInDB(conn, src.Code) == 0 {
		clog.Infof("[provincial][%s] 0 bills for session %d; retrying with previous session %d to seed DB", src.Code, session, session-1)
		prevBills, prevErr := crawlBillsForSource(src, legislature, session-1, client)
		if prevErr == nil && len(prevBills) > 0 {
			bills = prevBills
			session--
			sp.Session = session
		}
	}
	if berr != nil {
		clog.Infof("[provincial] %s bills error: %v", src.Code, berr)
	}
	sp.Bills = bills

	var divs []ProvincialDivisionResult
	switch src.Special {
	case "on":
		dates, err := CrawlOntarioVPSittingDates(src.VotesURL, legislature, session, client)
		if err != nil {
			return sp, fmt.Errorf("ontario dates: %w", err)
		}
		ontarioDivs, _ := crawlOntarioDaysConcurrently(dates, legislature, session, client, delay)
		divs = ontarioDivs
	case "sk":
		links, err := CrawlSaskatchewanMinutesLinks(src.VotesURL, client)
		if err != nil {
			clog.Infof("[provincial] sk: cannot discover minutes links (archive URL may have changed): %v", err)
		} else {
			skDivs, _ := crawlSaskatchewanMinutesConcurrently(links, legislature, session, client, delay)
			divs = skDivs
		}
	default:
		parsed, err := crawlDivisionsForSource(src, legislature, session, client)
		if err != nil {
			return sp, err
		}
		if allowPreviousSessionFallback && len(parsed) == 0 && session > 1 && provinceDivisionCountInDB(conn, src.Code) == 0 {
			clog.Infof("[provincial][%s] 0 divisions for session %d; retrying with previous session %d to seed DB", src.Code, session, session-1)
			prevParsed, prevErr := crawlDivisionsForSource(src, legislature, session-1, client)
			if prevErr == nil && len(prevParsed) > 0 {
				parsed = prevParsed
			}
		}
		divs = parsed
	}
	sp.Divisions = divs

	return sp, nil
}

// ExecuteCrawlPlan writes a CrawlPlan to the database.
// client and delay are only used when seedMembers is non-nil and fewer than
// 10 provincial members exist for the province (lazy member seeding).
func ExecuteCrawlPlan(conn *sql.DB, client *http.Client, delay time.Duration, plan CrawlPlan, enqueueSummary BillSummaryEnqueue, seedMembers MemberSeeder) error {
	src := plan.Source
	stats := provincialCrawlStats{}
	defer func() {
		if stats.MemberVotesSeen > 0 && stats.MemberVotesUpserted == 0 && stats.MemberVotesUnmatched == stats.MemberVotesSeen {
			var memberCount int
			_ = conn.QueryRow(
				`SELECT COUNT(1) FROM members WHERE government_level='provincial' AND lower(province)=lower(?)`,
				src.Province).Scan(&memberCount)
			if memberCount == 0 {
				clog.Infof("[provincial][%s] hint: 0 provincial members in DB for %q — run --members first to enable vote matching", src.Code, src.Province)
			}
		}
		clog.Infof("[provincial][%s] summary bills=%d/%d divisions=%d/%d votes=%d/%d unmatched=%d errors=%d",
			src.Code,
			stats.BillsUpserted, stats.BillsSeen,
			stats.DivisionsUpserted, stats.DivisionsSeen,
			stats.MemberVotesUpserted, stats.MemberVotesSeen,
			stats.MemberVotesUnmatched,
			stats.Errors,
		)
	}()

	for _, sp := range plan.Sessions {
		if err := executeSessionPlan(conn, client, delay, src, sp, enqueueSummary, seedMembers, &stats); err != nil {
			return err
		}
	}
	return nil
}

// executeSessionPlan writes one session's bills and divisions to the database.
func executeSessionPlan(conn *sql.DB, client *http.Client, delay time.Duration, src ProvincialSource, sp SessionPlan, enqueueSummary BillSummaryEnqueue, seedMembers MemberSeeder, stats *provincialCrawlStats) error {
	stats.BillsSeen += len(sp.Bills)
	for _, b := range sp.Bills {
		if err := store.UpsertBill(conn, store.BillRecord{
			ID:               b.ID,
			Parliament:       b.Parliament,
			Session:          b.Session,
			Number:           b.Number,
			Title:            b.Title,
			Chamber:          b.Chamber,
			LegisInfoURL:     b.DetailURL,
			FullTextURL:      b.DetailURL,
			LastActivityDate: b.LastActivityDate,
			LastScraped:      b.LastScraped,
		}); err != nil {
			stats.Errors++
			clog.Debugf("[provincial] %s bill upsert %s: %v", src.Code, b.ID, err)
		} else {
			stats.BillsUpserted++
			if enqueueSummary != nil && strings.TrimSpace(b.DetailURL) != "" {
				enqueueSummary(b.ID, b.Title, b.DetailURL, b.LastActivityDate)
			}
		}
		time.Sleep(delay)
	}

	stats.DivisionsSeen += len(sp.Divisions)
	if count := provinceMemberCountInDB(conn, src.Province); len(sp.Divisions) > 0 && count < 10 && seedMembers != nil {
		seedMembers(conn, src.Code, src.Province, client, delay)
	}
	memberCandidates, err := loadProvincialMemberCandidates(conn, src.Province)
	if err != nil {
		stats.Errors++
		return fmt.Errorf("load provincial members for %s: %w", src.Province, err)
	}

	wiki := newProvincialWikiLookup(src.Code, client)

	for _, res := range sp.Divisions {
		billID := provincialBillIDFromDescription(conn, src.Code, sp.Legislature, sp.Session, res.Division.Description)
		if err := store.UpsertDivision(conn, store.DivisionRecord{
			ID:          res.Division.ID,
			Parliament:  res.Division.Parliament,
			Session:     res.Division.Session,
			Number:      res.Division.Number,
			Date:        res.Division.Date,
			BillID:      billID,
			Description: strings.TrimSpace(res.Division.Description),
			Yeas:        res.Division.Yeas,
			Nays:        res.Division.Nays,
			Paired:      res.Division.Paired,
			Result:      res.Division.Result,
			Chamber:     res.Division.Chamber,
			SittingURL:  res.Division.DetailURL,
			LastScraped: res.Division.LastScraped,
		}); err != nil {
			stats.Errors++
			clog.Debugf("[provincial] %s division upsert %s: %v", src.Code, res.Division.ID, err)
		} else {
			stats.DivisionsUpserted++
		}

		stats.MemberVotesSeen += len(res.Votes)
		for _, mv := range res.Votes {
			memberID := resolveProvincialMemberIDFromCandidatesAtDate(memberCandidates, mv.MemberName, res.Division.Date)
			if memberID == "" {
				// Create a provisional member record for unmatched names so votes are not
				// lost. This handles former members not in the Represent API and new
				// members that haven't been crawled yet.
				// AB is excluded: its plain-token PDF parser still emits enough non-name
				// tokens that auto-creating records would pollute the members table.
				if src.Code != "ab" {
					fallbackID := provisionalProvincialMemberID(src.Code, mv.MemberName)
					if fallbackID != "" {
						rec := store.MemberRecord{
							ID:              fallbackID,
							Name:            mv.MemberName,
							Province:        src.Province,
							Chamber:         res.Division.Chamber,
							Active:          false,
							LastScraped:     utils.NowISO(),
							GovernmentLevel: "provincial",
						}
						if wiki != nil {
							if info, ok := wiki.lookupInfo(mv.MemberName); ok {
								rec.Party = info.party
								rec.Riding = info.riding
								rec.TermStart = info.termStart
								rec.TermEnd = info.termEnd
								clog.Debugf("[wiki] enriched provisional member %q: party=%q riding=%q term=%s..%s", mv.MemberName, info.party, info.riding, info.termStart, info.termEnd)
							}
						}
						if err := store.UpsertMember(conn, rec); err == nil {
							memberID = fallbackID
						}
					}
				}
				if memberID == "" {
					stats.MemberVotesUnmatched++
					clog.Debugf("[provincial][%s] unmatched vote name: %q", src.Code, mv.MemberName)
					continue
				}
			}
			if err := store.UpsertMemberVote(conn, res.Division.ID, memberID, mv.Vote); err != nil {
				stats.Errors++
				clog.Debugf("[provincial] %s member vote upsert: %v", src.Code, err)
			} else {
				stats.MemberVotesUpserted++
			}
		}
		time.Sleep(delay)
	}

	return nil
}

// looksLikePersonName rejects tokens that are clearly not person names:
// anything starting with a digit (timestamps, numbers) or containing no
// lowercase letters (all-caps headings like "AN ACT TO").
func looksLikePersonName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	runes := []rune(s)
	if !unicode.IsLetter(runes[0]) {
		return false
	}
	for _, r := range runes {
		if unicode.IsLower(r) {
			return true
		}
	}
	return false
}

func provisionalProvincialMemberID(provinceCode, sourceName string) string {
	if !looksLikePersonName(sourceName) {
		return ""
	}
	base := strings.ToLower(strings.TrimSpace(sourceName))
	if base == "" {
		return ""
	}
	base = strings.ReplaceAll(base, "'", "")
	base = strings.ReplaceAll(base, ".", "")
	base = nonAlnumForIDRe.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s-historical-%s", provinceCode, base)
}

func billTitleForDivisionDescription(conn *sql.DB, billID string) string {
	if conn == nil || strings.TrimSpace(billID) == "" {
		return ""
	}
	var title string
	if err := conn.QueryRow(`
		SELECT COALESCE(NULLIF(short_title,''), title, '')
		FROM bills
		WHERE id = ?`, billID).Scan(&title); err != nil {
		return ""
	}
	return strings.TrimSpace(title)
}

// CrawlProvinceSource crawls bills and votes for one province source and upserts
// normalized records into bills/divisions/member_votes tables.
// seedMembers is called when fewer than 10 provincial members exist in the DB;
// pass nil to skip member seeding (e.g. in tests).
func CrawlProvinceSource(conn *sql.DB, client *http.Client, delay time.Duration, src ProvincialSource, enqueueSummary BillSummaryEnqueue, seedMembers MemberSeeder) error {
	clog.Debugf("[provincial] crawling %s", src.Province)
	legislature, currentSession := resolveProvincialLegislatureSession(conn, src, client)
	sessions := sessionsToCrawlForSource(src, currentSession)
	if len(sessions) == 1 {
		clog.Debugf("[provincial][%s] detected legislature/session: %d/%d", src.Code, legislature, currentSession)
	} else {
		clog.Debugf("[provincial][%s] detected legislature/current session: %d/%d; crawling sessions %v", src.Code, legislature, currentSession, sessions)
	}

	stats := provincialCrawlStats{}
	defer func() {
		if stats.MemberVotesSeen > 0 && stats.MemberVotesUpserted == 0 && stats.MemberVotesUnmatched == stats.MemberVotesSeen {
			var memberCount int
			_ = conn.QueryRow(
				`SELECT COUNT(1) FROM members WHERE government_level='provincial' AND lower(province)=lower(?)`,
				src.Province).Scan(&memberCount)
			if memberCount == 0 {
				clog.Infof("[provincial][%s] hint: 0 provincial members in DB for %q — run --members first to enable vote matching", src.Code, src.Province)
			}
		}
		clog.Infof("[provincial][%s] summary bills=%d/%d divisions=%d/%d votes=%d/%d unmatched=%d errors=%d",
			src.Code,
			stats.BillsUpserted, stats.BillsSeen,
			stats.DivisionsUpserted, stats.DivisionsSeen,
			stats.MemberVotesUpserted, stats.MemberVotesSeen,
			stats.MemberVotesUnmatched,
			stats.Errors,
		)
	}()

	allowPreviousSessionFallback := len(sessions) == 1
	for _, session := range sessions {
		clog.Debugf("[provincial][%s] crawling legislature/session: %d/%d", src.Code, legislature, session)

		effectiveSession := session
		bills, berr := crawlBillsForSource(src, legislature, session, client)
		if allowPreviousSessionFallback && berr == nil && len(bills) == 0 && session > 1 && provinceBillCountInDB(conn, src.Code) == 0 {
			clog.Infof("[provincial][%s] 0 bills for session %d; retrying with previous session %d to seed DB", src.Code, session, session-1)
			prevBills, prevErr := crawlBillsForSource(src, legislature, session-1, client)
			if prevErr == nil && len(prevBills) > 0 {
				bills = prevBills
				effectiveSession = session - 1
			}
		}
		if berr != nil {
			clog.Infof("[provincial] %s bills error: %v", src.Code, berr)
		}

		if len(bills) > 0 {
			if err := executeSessionPlan(conn, client, delay, src, SessionPlan{
				Legislature: legislature,
				Session:     effectiveSession,
				Bills:       bills,
			}, enqueueSummary, seedMembers, &stats); err != nil {
				return err
			}
		}

		var divs []ProvincialDivisionResult
		switch src.Special {
		case "on":
			dates, err := CrawlOntarioVPSittingDates(src.VotesURL, legislature, effectiveSession, client)
			if err != nil {
				return fmt.Errorf("ontario dates: %w", err)
			}
			ontarioDivs, _ := crawlOntarioDaysConcurrently(dates, legislature, effectiveSession, client, delay)
			divs = ontarioDivs
		case "sk":
			links, err := CrawlSaskatchewanMinutesLinks(src.VotesURL, client)
			if err != nil {
				clog.Infof("[provincial] sk: cannot discover minutes links (archive URL may have changed): %v", err)
			} else {
				skDivs, _ := crawlSaskatchewanMinutesConcurrently(links, legislature, effectiveSession, client, delay)
				divs = skDivs
			}
		default:
			parsed, err := crawlDivisionsForSource(src, legislature, effectiveSession, client)
			if err != nil {
				return err
			}
			if allowPreviousSessionFallback && len(parsed) == 0 && effectiveSession > 1 && provinceDivisionCountInDB(conn, src.Code) == 0 {
				clog.Infof("[provincial][%s] 0 divisions for session %d; retrying with previous session %d to seed DB", src.Code, effectiveSession, effectiveSession-1)
				prevParsed, prevErr := crawlDivisionsForSource(src, legislature, effectiveSession-1, client)
				if prevErr == nil && len(prevParsed) > 0 {
					parsed = prevParsed
				}
			}
			divs = parsed
		}

		if len(divs) > 0 {
			if err := executeSessionPlan(conn, client, delay, src, SessionPlan{
				Legislature: legislature,
				Session:     effectiveSession,
				Divisions:   divs,
			}, enqueueSummary, seedMembers, &stats); err != nil {
				return err
			}
		}

		// Ensure 0-result sessions still contribute to seen counters and summary shape.
		if len(bills) == 0 && len(divs) == 0 {
			if err := executeSessionPlan(conn, client, delay, src, SessionPlan{Legislature: legislature, Session: effectiveSession}, enqueueSummary, seedMembers, &stats); err != nil {
				return err
			}
		}
	}

	return nil
}

func sessionsToCrawlForSource(src ProvincialSource, currentSession int) []int {
	if src.Code != "pe" || currentSession <= 1 {
		return []int{currentSession}
	}
	sessions := make([]int, 0, currentSession)
	for session := currentSession; session >= 1; session-- {
		sessions = append(sessions, session)
	}
	return sessions
}

func crawlOntarioDaysConcurrently(dates []string, legislature, session int, client *http.Client, delay time.Duration) ([]ProvincialDivisionResult, int) {
	if len(dates) == 0 {
		return nil, 0
	}
	limit := minInt(provinceSubcrawlParallelism, len(dates))
	g := new(errgroup.Group)
	g.SetLimit(limit)

	divs := make([]ProvincialDivisionResult, 0, len(dates))
	errCount := 0
	var mu sync.Mutex

	for _, date := range dates {
		date := date
		g.Go(func() error {
			dayURL := OntarioVPDayURL(legislature, session, date)
			dayDivs, err := CrawlOntarioVPDay(dayURL, legislature, session, date, client)
			if err != nil {
				if strings.Contains(err.Error(), "status 404") {
					clog.Debugf("[provincial] on day %s: no votes-proceedings page; skipping", date)
					return nil
				}
				mu.Lock()
				errCount++
				mu.Unlock()
				clog.Debugf("[provincial] on day %s: %v", date, err)
				return nil
			}
			mu.Lock()
			divs = append(divs, dayDivs...)
			mu.Unlock()
			time.Sleep(delay)
			return nil
		})
	}
	_ = g.Wait()
	return divs, errCount
}

func crawlSaskatchewanMinutesConcurrently(links []string, legislature, session int, client *http.Client, delay time.Duration) ([]ProvincialDivisionResult, int) {
	if len(links) == 0 {
		return nil, 0
	}
	limit := minInt(provinceSubcrawlParallelism, len(links))
	g := new(errgroup.Group)
	g.SetLimit(limit)

	divs := make([]ProvincialDivisionResult, 0, len(links))
	errCount := 0
	var mu sync.Mutex

	for _, link := range links {
		link := link
		g.Go(func() error {
			dayDivs, err := CrawlSaskatchewanMinutes(link, legislature, session, client)
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
				clog.Debugf("[provincial] sk minutes %s: %v", link, err)
				return nil
			}
			mu.Lock()
			divs = append(divs, dayDivs...)
			mu.Unlock()
			time.Sleep(delay)
			return nil
		})
	}
	_ = g.Wait()
	return divs, errCount
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// crawlBillsForSource dispatches to the correct province-specific bill crawler.
func crawlBillsForSource(src ProvincialSource, legislature, session int, client *http.Client) ([]ProvincialBillStub, error) {
	if crawler, ok := provinceCrawlers[src.Code]; ok {
		return crawler.CrawlBills(src.BillsURL, legislature, session, client)
	}
	return CrawlProvincialBillsFromIndex(src.BillsURL, src.Code, legislature, session, src.Chamber, client)
}

// crawlDivisionsForSource dispatches to the correct province-specific votes crawler
// for all non-special provinces (i.e. excluding ON and SK which use their own multi-step
// logic in CrawlProvinceSource).
func crawlDivisionsForSource(src ProvincialSource, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	if crawler, ok := provinceCrawlers[src.Code]; ok && crawler != nil {
		if votes, err := crawler.CrawlVotes(src.VotesURL, legislature, session, client); err != nil || votes != nil {
			return votes, err
		}
	}
	return CrawlGenericProvincialVotes(src.VotesURL, src.Code, src.Chamber, legislature, session, client)
}

func resolveProvincialLegislatureSession(conn *sql.DB, src ProvincialSource, client *http.Client) (int, int) {
	if client == nil {
		client = http.DefaultClient
	}

	candidates := make([]legislatureSession, 0)
	for _, u := range []string{src.BillsURL, src.VotesURL} {
		if u == "" {
			continue
		}
		candidates = append(candidates, extractLegislatureSessionCandidates(src.Code, u, 70)...)
		doc, err := fetchDoc(u, client)
		if err != nil {
			continue
		}
		doc.Find("title, h1, h2, h3, h4, h5").Each(func(_ int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if text == "" {
				return
			}
			candidates = append(candidates, extractLegislatureSessionCandidates(src.Code, text, 90)...)
		})
		doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			text := strings.TrimSpace(s.Text())
			snippet := strings.TrimSpace(text + " " + href)
			linkCandidates := extractLegislatureSessionCandidates(src.Code, snippet, 55)
			lower := strings.ToLower(snippet)
			if len(linkCandidates) == 0 && !strings.Contains(lower, "parliament") && !strings.Contains(lower, "legislature") && !strings.Contains(lower, "session") && !strings.Contains(lower, "assembly") {
				return
			}
			candidates = append(candidates, linkCandidates...)
		})
	}

	if best, ok := maxLegislatureSession(candidates); ok {
		return best.Legislature, best.Session
	}

	if l, s, ok := latestLegislatureSessionFromDB(conn, src.Code); ok {
		return l, s
	}

	if src.Code == "pe" {
		if l, s, ok := FetchPEICurrentAssemblySession(); ok {
			clog.Infof("[pe] auto-detected assembly=%d session=%d from WDF API", l, s)
			return l, s
		}
		return PEIFallbackAssembly(), PEIFallbackSession()
	}
	switch src.Special {
	case "on":
		return OntarioParliament, OntarioSession
	case "sk":
		return SaskatchewanLegislature, SaskatchewanSession
	default:
		return 1, 1
	}
}

func latestLegislatureSessionFromDB(conn *sql.DB, provinceCode string) (int, int, bool) {
	if conn == nil {
		return 0, 0, false
	}
	var legislature, session int
	q := `SELECT COALESCE(MAX(parliament),0), COALESCE(MAX(session),0) FROM bills WHERE id LIKE ?`
	if err := conn.QueryRow(q, provinceCode+"-%").Scan(&legislature, &session); err != nil {
		return 0, 0, false
	}
	if legislature <= 0 || session <= 0 {
		return 0, 0, false
	}
	return legislature, session, true
}

func provinceBillCountInDB(conn *sql.DB, provinceCode string) int {
	if conn == nil {
		return 0
	}
	var n int
	_ = conn.QueryRow(`SELECT COUNT(1) FROM bills WHERE id LIKE ?`, provinceCode+"-%").Scan(&n)
	return n
}

func provinceDivisionCountInDB(conn *sql.DB, provinceCode string) int {
	if conn == nil {
		return 0
	}
	var n int
	_ = conn.QueryRow(`SELECT COUNT(1) FROM divisions WHERE id LIKE ?`, provinceCode+"-%").Scan(&n)
	return n
}

func provinceMemberCountInDB(conn *sql.DB, province string) int {
	if conn == nil {
		return 0
	}
	var n int
	_ = conn.QueryRow(
		`SELECT COUNT(1) FROM members WHERE government_level='provincial' AND lower(province)=lower(?)`,
		province,
	).Scan(&n)
	return n
}

func provincialSetSlugForCode(code string) string {
	switch code {
	case "ab":
		return "alberta-legislature"
	case "bc":
		return "bc-legislature"
	case "mb":
		return "manitoba-legislature"
	case "nb":
		return "nb-legislature"
	case "nl":
		return "newfoundland-labrador-legislature"
	case "ns":
		return "nova-scotia-legislature"
	case "on":
		return "ontario-legislature"
	case "pe":
		return "pei-legislature"
	case "qc":
		return "quebec-assemblee-nationale"
	case "sk":
		return "saskatchewan-legislature"
	default:
		return ""
	}
}

// ProvincialSetSlugForCode returns the Represent API set slug for a province code.
func ProvincialSetSlugForCode(code string) string {
	return provincialSetSlugForCode(code)
}

func cleanupManitobaStaleSessionDivisions(conn *sql.DB, legislature, session int) (int64, error) {
	if conn == nil {
		return 0, nil
	}
	idLike := fmt.Sprintf("mb-%d-%d-%%", legislature, session)
	wantPath := fmt.Sprintf("%%%s/%s/%%", parliamentOrdinal(legislature), parliamentOrdinal(session))

	res, err := conn.Exec(`
			DELETE FROM member_votes
			WHERE division_id IN (
				SELECT id FROM divisions
				WHERE id LIKE ? AND sitting_url IS NOT NULL AND sitting_url NOT LIKE ?
			)`, idLike, wantPath)
	if err != nil {
		return 0, err
	}
	_, _ = res.RowsAffected()

	divRes, err := conn.Exec(`
			DELETE FROM divisions
			WHERE id LIKE ? AND sitting_url IS NOT NULL AND sitting_url NOT LIKE ?`, idLike, wantPath)
	if err != nil {
		return 0, err
	}
	return divRes.RowsAffected()
}

// CleanupManitobaStaleSessionDivisionsForTest exposes cleanupManitobaStaleSessionDivisions for tests.
func CleanupManitobaStaleSessionDivisionsForTest(conn *sql.DB, legislature, session int) (int64, error) {
	return cleanupManitobaStaleSessionDivisions(conn, legislature, session)
}

func provincialBillIDFromDescription(conn *sql.DB, provinceCode string, legislature, session int, description string) string {
	billNumber := ExtractProvincialBillNumber(description)
	if billNumber == "" {
		return ""
	}
	billID := ProvincialBillID(provinceCode, legislature, session, billNumber)
	if billID == "" {
		return ""
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(1) FROM bills WHERE id = ?`, billID).Scan(&count); err != nil {
		return ""
	}
	if count == 0 {
		return ""
	}
	return billID
}
