package scraper

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/philspins/opendocket/internal/scraper/provincial"
	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/utils"
	"golang.org/x/sync/errgroup"
)

// ProvincialSource is an alias for provincial.ProvincialSource.
type ProvincialSource = provincial.ProvincialSource

// BillSummaryEnqueue is an alias for provincial.BillSummaryEnqueue.
type BillSummaryEnqueue = provincial.BillSummaryEnqueue

// CrawlBills runs federal bill crawl + detail enrichment + optional summary enqueue.
func CrawlBills(conn *sql.DB, client *http.Client, delay time.Duration, rssURL string, enqueueSummary BillSummaryEnqueue) error {
	stubs, err := CrawlBillsRSS(rssURL, client)
	if err != nil {
		return err
	}
	for _, stub := range stubs {
		detail, err := CrawlBillDetail(stub.ID, stub.LegisInfoURL, client)
		if err != nil {
			log.Printf("[bills] detail error for %s: %v", stub.ID, err)
		}
		time.Sleep(delay)

		parl, sess, ok := utils.ParliamentSessionFromBillID(stub.ID)
		bill := store.BillRecord{
			ID:               stub.ID,
			Parliament:       parl,
			Session:          sess,
			Number:           utils.BillNumberFromID(stub.ID),
			Title:            stub.Title,
			Chamber:          utils.BillChamber(utils.BillNumberFromID(stub.ID)),
			LegisInfoURL:     stub.LegisInfoURL,
			LastActivityDate: stub.LastActivityDate,
			CurrentStage:     detail.CurrentStage,
			CurrentStatus:    detail.CurrentStatus,
			SponsorID:        detail.SponsorID,
			BillType:         detail.BillType,
			FullTextURL:      detail.FullTextURL,
			IntroducedDate:   detail.IntroducedDate,
			LastScraped:      utils.NowISO(),
		}
		if !ok {
			bill.Parliament = 0
			bill.Session = 0
		}
		if err := store.UpsertBill(conn, bill); err != nil {
			log.Printf("[bills] upsert %s: %v", stub.ID, err)
		}
		for _, stage := range detail.Stages {
			store.UpsertBillStage(conn, store.BillStageRecord{
				BillID:  stub.ID,
				Stage:   stage.Stage,
				Chamber: stage.Chamber,
				Date:    stage.Date,
			})
		}

		if enqueueSummary != nil && strings.TrimSpace(bill.FullTextURL) != "" {
			enqueueSummary(bill.ID, bill.Title, bill.FullTextURL, bill.LastActivityDate)
		}
	}
	return nil
}

// CrawlMembers fetches federal + provincial members and upserts to DB.
func CrawlMembers(conn *sql.DB, client *http.Client, delay time.Duration, apiURL string) error {
	profiles, err := CrawlMembersFromAPI(apiURL, client)
	if err != nil {
		return err
	}
	store.UpsertProfiles(conn, toDBMembers(profiles))

	slugs := make([]string, 0, len(ProvincialLegislatureAPIs))
	for slug := range ProvincialLegislatureAPIs {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	type result struct {
		slug     string
		profiles []MemberProfile
	}
	results := make([]result, len(slugs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // fetch up to 5 provincial APIs concurrently
	for i, setSlug := range slugs {
		wg.Add(1)
		go func(i int, setSlug string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			time.Sleep(delay) // polite delay per request to represent.opennorth.ca
			provProfiles, perr := CrawlProvincialMembersFromAPI(setSlug, "", client)
			if perr != nil {
				log.Printf("[members] provincial set %s: %v", setSlug, perr)
				return
			}
			if len(provProfiles) == 0 && setSlug == "nb-legislature" {
				log.Printf("[members] nb-legislature: Represent API returned 0 members; falling back to NB website scraper")
				nbProfiles, nberr := CrawlNewBrunswickMembersFromWebsite("", client)
				if nberr != nil {
					log.Printf("[members] nb-legislature website fallback: %v", nberr)
				} else {
					provProfiles = nbProfiles
				}
			}
			results[i] = result{slug: setSlug, profiles: provProfiles}
		}(i, setSlug)
	}
	wg.Wait()

	for _, r := range results {
		if len(r.profiles) > 0 {
			store.UpsertProfiles(conn, toDBMembers(r.profiles))
		}
	}
	return nil
}

func toDBMembers(profiles []MemberProfile) []store.MemberRecord {
	out := make([]store.MemberRecord, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, store.MemberRecord{
			ID:              profile.ID,
			Name:            profile.Name,
			Party:           profile.Party,
			Riding:          profile.Riding,
			Province:        profile.Province,
			Role:            profile.Role,
			PhotoURL:        profile.PhotoURL,
			Email:           profile.Email,
			Website:         profile.Website,
			Chamber:         profile.Chamber,
			Active:          profile.Active,
			LastScraped:     profile.LastScraped,
			GovernmentLevel: profile.GovernmentLevel,
		})
	}
	return out
}

// CrawlCalendar fetches sitting dates and upserts them for the current parliament/session.
func CrawlCalendar(conn *sql.DB, client *http.Client, _ time.Duration, sourceURL string) error {
	dates, err := CrawlSittingCalendar(sourceURL, client)
	if err != nil {
		return err
	}
	for _, d := range dates {
		if err := store.UpsertSittingDate(conn, CurrentParliament, CurrentSession, d); err != nil {
			log.Printf("[calendar] upsert %s: %v", d, err)
		}
	}
	if err := CrawlAndPersistLegislatureCalendars(conn, client); err != nil {
		log.Printf("[calendar] legislature schedule crawl warning: %v", err)
	}
	return nil
}

// CrawlVotes indexes commons votes and fills detail rows when needed.
func CrawlVotes(conn *sql.DB, client *http.Client, delay time.Duration, indexURL string) error {
	divs, err := CrawlVotesIndex(indexURL, CurrentParliament, CurrentSession, client)
	if err != nil {
		return err
	}
	federalCandidates, err := loadFederalMemberCandidates(conn)
	if err != nil {
		log.Printf("[votes] load federal member candidates: %v", err)
	}
	for _, div := range divs {
		existed, err := store.DivisionExists(conn, div.ID)
		if err != nil {
			log.Printf("[votes] exists check %s: %v", div.ID, err)
		}

		if err := store.UpsertDivision(conn, store.DivisionRecord{
			ID:          div.ID,
			Parliament:  div.Parliament,
			Session:     div.Session,
			Number:      div.Number,
			Date:        div.Date,
			BillID:      utils.BillIDFromParts(div.Parliament, div.Session, div.BillNumber),
			Description: div.Description,
			Yeas:        div.Yeas,
			Nays:        div.Nays,
			Paired:      div.Paired,
			Result:      div.Result,
			Chamber:     div.Chamber,
			LastScraped: div.LastScraped,
		}); err != nil {
			log.Printf("[votes] upsert %s: %v", div.ID, err)
		}

		needsDetail := !existed
		if existed && div.DetailURL != "" {
			hasVotes, err := store.DivisionHasVotes(conn, div.ID)
			if err != nil {
				log.Printf("[votes] has-votes check %s: %v", div.ID, err)
			}
			needsDetail = !hasVotes
		}
		if needsDetail && div.DetailURL != "" {
			votes, err := CrawlDivisionDetail(div.ID, div.DetailURL, client)
			if err != nil {
				log.Printf("[votes] detail error %s: %v", div.ID, err)
			}
			for _, v := range votes {
				memberID := strings.TrimSpace(v.MemberID)
				if memberID == "" && strings.TrimSpace(v.MemberName) != "" {
					memberID = resolveFederalMemberIDFromCandidates(federalCandidates, v.MemberName)
				}
				if memberID == "" {
					continue
				}
				store.UpsertMemberVote(conn, v.DivisionID, memberID, v.Vote)
			}
			time.Sleep(delay)
		}
	}
	return nil
}

type federalMemberCandidate struct {
	ID     string
	Name   string
	Riding string
	Active bool
}

func loadFederalMemberCandidates(conn *sql.DB) ([]federalMemberCandidate, error) {
	if conn == nil {
		return nil, nil
	}
	rows, err := conn.Query(`
		SELECT id, name, COALESCE(riding,''), COALESCE(active, 1)
		FROM members
		WHERE lower(government_level) = 'federal' AND lower(chamber) = 'commons'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]federalMemberCandidate, 0)
	for rows.Next() {
		var c federalMemberCandidate
		if err := rows.Scan(&c.ID, &c.Name, &c.Riding, &c.Active); err != nil {
			continue
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

var federalNameParenRe = regexp.MustCompile(`\s*\([^)]*\)`)

func normalizeFederalMemberName(s string) string {
	s = strings.TrimSpace(s)
	s = federalNameParenRe.ReplaceAllString(s, "")
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func normalizeFederalText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "(", " ")
	s = strings.ReplaceAll(s, ")", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func federalNameQualifier(sourceName string) string {
	m := regexp.MustCompile(`\(([^)]*)\)`).FindStringSubmatch(sourceName)
	if len(m) < 2 {
		return ""
	}
	return normalizeFederalText(m[1])
}

func federalTokenOverlap(a, b string) int {
	if a == "" || b == "" {
		return 0
	}
	set := map[string]struct{}{}
	for _, token := range strings.Fields(a) {
		set[token] = struct{}{}
	}
	score := 0
	for _, token := range strings.Fields(b) {
		if _, ok := set[token]; ok {
			score++
		}
	}
	return score
}

func pickBestFederalCandidate(candidates []federalMemberCandidate, qualifier string) string {
	if len(candidates) == 0 {
		return ""
	}
	bestID := ""
	bestScore := -1
	bestActive := false
	for _, c := range candidates {
		score := 0
		if qualifier != "" {
			score = federalTokenOverlap(normalizeFederalText(c.Riding), qualifier)
		}
		if bestID == "" || score > bestScore ||
			(score == bestScore && c.Active && !bestActive) ||
			(score == bestScore && c.Active == bestActive && c.ID < bestID) {
			bestID = c.ID
			bestScore = score
			bestActive = c.Active
		}
	}
	return bestID
}

func hasTokenSuffix(haystack, suffix []string) bool {
	if len(suffix) == 0 || len(haystack) < len(suffix) {
		return false
	}
	start := len(haystack) - len(suffix)
	for i := range suffix {
		if haystack[start+i] != suffix[i] {
			return false
		}
	}
	return true
}

func resolveFederalMemberIDFromCandidates(list []federalMemberCandidate, sourceName string) string {
	want := normalizeFederalMemberName(sourceName)
	if want == "" {
		return ""
	}
	qualifier := federalNameQualifier(sourceName)

	exactMatches := make([]federalMemberCandidate, 0, 1)
	for _, c := range list {
		if normalizeFederalMemberName(c.Name) == want {
			exactMatches = append(exactMatches, c)
		}
	}
	if len(exactMatches) > 0 {
		return pickBestFederalCandidate(exactMatches, qualifier)
	}

	wantParts := strings.Fields(want)
	if len(wantParts) == 0 {
		return ""
	}

	suffixMatches := make([]federalMemberCandidate, 0, 2)
	for _, c := range list {
		candidateParts := strings.Fields(normalizeFederalMemberName(c.Name))
		if !hasTokenSuffix(candidateParts, wantParts) {
			continue
		}
		suffixMatches = append(suffixMatches, c)
	}
	if len(suffixMatches) > 0 {
		return pickBestFederalCandidate(suffixMatches, qualifier)
	}

	// Journal pages often list only surnames. Best-effort fallback:
	// match last-name token to Commons members and pick the best candidate
	// by riding qualifier overlap, active status, then deterministic ID order.
	if len(wantParts) == 1 {
		surname := wantParts[0]
		surnameMatches := make([]federalMemberCandidate, 0, 2)
		for _, c := range list {
			candidateParts := strings.Fields(normalizeFederalMemberName(c.Name))
			if len(candidateParts) == 0 {
				continue
			}
			if candidateParts[len(candidateParts)-1] == surname {
				surnameMatches = append(surnameMatches, c)
			}
		}
		if len(surnameMatches) > 0 {
			return pickBestFederalCandidate(surnameMatches, qualifier)
		}
	}

	return ""
}

// CrawlSenate indexes senate votes and fills detail rows when needed.
func CrawlSenate(conn *sql.DB, client *http.Client, delay time.Duration, indexURL string) error {
	divs, err := CrawlSenateVotesIndex(indexURL, CurrentParliament, CurrentSession, client)
	if err != nil {
		return err
	}
	for _, div := range divs {
		existed, err := store.DivisionExists(conn, div.ID)
		if err != nil {
			log.Printf("[senate] exists check %s: %v", div.ID, err)
		}

		if err := store.UpsertDivision(conn, store.DivisionRecord{
			ID:          div.ID,
			Parliament:  div.Parliament,
			Session:     div.Session,
			Number:      div.Number,
			Date:        div.Date,
			BillID:      utils.BillIDFromParts(div.Parliament, div.Session, div.BillNumber),
			Description: div.Description,
			Yeas:        div.Yeas,
			Nays:        div.Nays,
			Paired:      div.Paired,
			Result:      div.Result,
			Chamber:     div.Chamber,
			LastScraped: div.LastScraped,
		}); err != nil {
			log.Printf("[senate] upsert %s: %v", div.ID, err)
		}

		needsDetail := !existed
		if existed && div.DetailURL != "" {
			hasVotes, err := store.DivisionHasVotes(conn, div.ID)
			if err != nil {
				log.Printf("[senate] has-votes check %s: %v", div.ID, err)
			}
			needsDetail = !hasVotes
		}
		if needsDetail && div.DetailURL != "" {
			votes, err := CrawlSenateDivisionDetail(div.ID, div.DetailURL, client)
			if err != nil {
				log.Printf("[senate] detail error %s: %v", div.ID, err)
			}
			for _, v := range votes {
				store.UpsertMemberVote(conn, v.DivisionID, v.MemberID, v.Vote)
			}
			time.Sleep(delay)
		}
	}
	return nil
}

// CrawlProvincial runs configured provincial crawlers with bounded concurrency.
// If codes is non-empty only the named province codes (e.g. "pe", "on") are
// crawled; otherwise all sources in ProvincialSources run.
func CrawlProvincial(conn *sql.DB, client *http.Client, delay time.Duration, parallelism int, codes []string, enqueueSummary BillSummaryEnqueue) error {
	sources := provincial.ProvincialSources
	if len(codes) > 0 {
		set := make(map[string]bool, len(codes))
		for _, c := range codes {
			set[strings.ToLower(strings.TrimSpace(c))] = true
		}
		filtered := make([]ProvincialSource, 0, len(codes))
		for _, src := range provincial.ProvincialSources {
			if set[src.Code] {
				filtered = append(filtered, src)
			}
		}
		sources = filtered
	}
	if parallelism < 1 {
		parallelism = 1
	}
	g := new(errgroup.Group)
	g.SetLimit(parallelism)
	errMu := sync.Mutex{}
	errs := make([]error, 0)
	for _, src := range sources {
		src := src
		g.Go(func() error {
			seeder := func(conn *sql.DB, code, province string, c *http.Client, d time.Duration) {
				_ = ensureProvincialMembersForSource(conn, c, d, ProvincialSource{Code: code, Province: province})
			}
			if err := provincial.CrawlProvinceSource(conn, client, delay, src, enqueueSummary, seeder); err != nil {
				log.Printf("[provincial] %s: %v", src.Code, err)
				errMu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", src.Code, err))
				errMu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func ensureProvincialMembersForSource(conn *sql.DB, client *http.Client, delay time.Duration, src ProvincialSource) error {
	if conn == nil {
		return nil
	}
	setSlug := provincial.ProvincialSetSlugForCode(src.Code)
	if setSlug == "" {
		return nil
	}
	profiles, err := CrawlProvincialMembersFromAPI(setSlug, "", client)
	if err != nil {
		return err
	}
	if len(profiles) == 0 && setSlug == "nb-legislature" {
		profiles, err = CrawlNewBrunswickMembersFromWebsite("", client)
		if err != nil {
			return err
		}
	}
	if len(profiles) == 0 {
		return nil
	}
	store.UpsertProfiles(conn, toDBMembers(profiles))
	return nil
}

// DefaultParallelism reads CRAWLER_PARALLELISM and falls back to 5 when unset or invalid.
func DefaultParallelism() int {
	if v := os.Getenv("CRAWLER_PARALLELISM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 5
}

// RunParallel executes each function in fns in its own goroutine, bounded by parallelism.
func RunParallel(parallelism int, fns []func()) {
	if parallelism < 1 {
		parallelism = 1
	}
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for _, fn := range fns {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			fn()
		}()
	}
	wg.Wait()
}
