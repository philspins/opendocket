package scraper

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/philspins/opendocket/internal/clog"
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
			clog.Debugf("[bills] detail error for %s: %v", stub.ID, err)
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
			clog.Debugf("[bills] upsert %s: %v", stub.ID, err)
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
	_ = delay // Member crawls are OpenNorth/API-backed; avoid per-row crawl delay for performance.

	profiles, err := CrawlMembersFromAPI(apiURL, client)
	if err != nil {
		return err
	}
	store.UpsertProfiles(conn, toDBMembers(profiles), 0)

	type provincialMembersResult struct {
		setSlug  string
		profiles []MemberProfile
	}

	results := make(chan provincialMembersResult, len(ProvincialLegislatureAPIs))
	g := new(errgroup.Group)
	for setSlug := range ProvincialLegislatureAPIs {
		setSlug := setSlug
		g.Go(func() error {
			provProfiles, perr := CrawlProvincialMembersFromAPI(setSlug, "", client)
			if perr != nil {
				clog.Debugf("[members] provincial set %s: %v", setSlug, perr)
				return nil
			}
			// The Represent API for nb-legislature currently returns 0 members.
			// Fall back to scraping the NB legislature website directly.
			if len(provProfiles) == 0 && setSlug == "nb-legislature" {
				clog.Debugf("[members] nb-legislature: Represent API returned 0 members; falling back to NB website scraper")
				nbProfiles, nberr := CrawlNewBrunswickMembersFromWebsite("", client)
				if nberr != nil {
					clog.Debugf("[members] nb-legislature website fallback: %v", nberr)
				} else {
					provProfiles = nbProfiles
				}
			}
			results <- provincialMembersResult{setSlug: setSlug, profiles: provProfiles}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	close(results)
	allProvincialProfiles := make([]MemberProfile, 0)

	for result := range results {
		if len(result.profiles) == 0 {
			continue
		}
		allProvincialProfiles = append(allProvincialProfiles, result.profiles...)
		store.UpsertProfiles(conn, toDBMembers(result.profiles), 0)
	}
	clog.Infof("[members] summary by province (federal/provincial): %s", federalProvincialMembersByProvinceSummary(profiles, allProvincialProfiles))
	return nil
}

func federalProvincialMembersByProvinceSummary(federalProfiles, provincialProfiles []MemberProfile) string {
	if len(federalProfiles) == 0 && len(provincialProfiles) == 0 {
		return "none"
	}
	type counts struct {
		federal    int
		provincial int
	}
	byProvince := make(map[string]counts)
	for _, profile := range federalProfiles {
		province := strings.TrimSpace(profile.Province)
		if province == "" {
			province = "Unknown"
		}
		entry := byProvince[province]
		entry.federal++
		byProvince[province] = entry
	}
	for _, profile := range provincialProfiles {
		province := strings.TrimSpace(profile.Province)
		if province == "" {
			province = "Unknown"
		}
		entry := byProvince[province]
		entry.provincial++
		byProvince[province] = entry
	}
	keys := make([]string, 0, len(byProvince))
	for province := range byProvince {
		keys = append(keys, province)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, province := range keys {
		entry := byProvince[province]
		parts = append(parts, fmt.Sprintf("%s federal=%d provincial=%d", province, entry.federal, entry.provincial))
	}
	return strings.Join(parts, "; ")
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
			clog.Debugf("[calendar] upsert %s: %v", d, err)
		}
	}
	if err := CrawlAndPersistLegislatureCalendars(conn, client); err != nil {
		clog.Infof("[calendar] legislature schedule crawl warning: %v", err)
	}
	return nil
}

// CrawlVotes indexes commons votes and fills detail rows when needed.
func CrawlVotes(conn *sql.DB, client *http.Client, delay time.Duration, indexURL string) error {
	divs, err := CrawlVotesIndex(indexURL, CurrentParliament, CurrentSession, client)
	if err != nil {
		clog.Infof("[votes] crawl failed: %v", err)
		return err
	}

	processed := 0
	detailScraped := 0
	for _, div := range divs {
		processed++
		existed, err := store.DivisionExists(conn, div.ID)
		if err != nil {
			clog.Debugf("[votes] exists check %s: %v", div.ID, err)
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
			clog.Debugf("[votes] upsert %s: %v", div.ID, err)
		}

		needsDetail := !existed
		if existed && div.DetailURL != "" {
			hasVotes, err := store.DivisionHasVotes(conn, div.ID)
			if err != nil {
				clog.Debugf("[votes] has-votes check %s: %v", div.ID, err)
			}
			needsDetail = !hasVotes
		}
		if needsDetail && div.DetailURL != "" {
			detailScraped++
			votes, err := CrawlDivisionDetail(div.ID, div.DetailURL, client)
			if err != nil {
				clog.Debugf("[votes] detail error %s: %v", div.ID, err)
			}
			for _, v := range votes {
				store.UpsertMemberVote(conn, v.DivisionID, v.MemberID, v.Vote)
			}
			time.Sleep(delay)
		}
	}

	clog.Infof("[votes] crawl complete: divisions=%d detailed=%d", processed, detailScraped)
	return nil
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
			clog.Debugf("[senate] exists check %s: %v", div.ID, err)
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
			clog.Debugf("[senate] upsert %s: %v", div.ID, err)
		}

		needsDetail := !existed
		if existed && div.DetailURL != "" {
			hasVotes, err := store.DivisionHasVotes(conn, div.ID)
			if err != nil {
				clog.Debugf("[senate] has-votes check %s: %v", div.ID, err)
			}
			needsDetail = !hasVotes
		}
		if needsDetail && div.DetailURL != "" {
			votes, err := CrawlSenateDivisionDetail(div.ID, div.DetailURL, client)
			if err != nil {
				clog.Debugf("[senate] detail error %s: %v", div.ID, err)
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
// ProvincialCrawlOpts holds optional overrides for CrawlProvincial.
type ProvincialCrawlOpts struct {
	Codes       []string // province codes to crawl; nil = all
	Legislature int      // force legislature number (requires Session > 0)
	Session     int      // force session number (requires Legislature > 0)
	AllSittings bool     // bypass per-province recent-PDF window limits
}

func CrawlProvincial(conn *sql.DB, client *http.Client, delay time.Duration, parallelism int, codes []string, enqueueSummary BillSummaryEnqueue) error {
	return CrawlProvincialWithOpts(conn, client, delay, parallelism, ProvincialCrawlOpts{Codes: codes}, enqueueSummary)
}

func CrawlProvincialWithOpts(conn *sql.DB, client *http.Client, delay time.Duration, parallelism int, opts ProvincialCrawlOpts, enqueueSummary BillSummaryEnqueue) error {
	clog.Infof("[provincial] crawling provincial sources")
	sources := provincial.ProvincialSources
	if len(opts.Codes) > 0 {
		set := make(map[string]bool, len(opts.Codes))
		for _, c := range opts.Codes {
			set[strings.ToLower(strings.TrimSpace(c))] = true
		}
		filtered := make([]ProvincialSource, 0, len(opts.Codes))
		for _, src := range provincial.ProvincialSources {
			if set[src.Code] {
				filtered = append(filtered, src)
			}
		}
		sources = filtered
	}
	// Apply CLI overrides to every source being crawled.
	if opts.Legislature > 0 && opts.Session > 0 || opts.AllSittings {
		patched := make([]ProvincialSource, len(sources))
		for i, src := range sources {
			src.ForcedLegislature = opts.Legislature
			src.ForcedSession = opts.Session
			src.AllSittings = opts.AllSittings
			patched[i] = src
		}
		sources = patched
	}
	// One goroutine per province: each independently fetches+writes bills, then
	// fetches+writes votes. No inter-province barrier — a province moves from bills
	// to votes as soon as its own bills are done, regardless of other provinces.
	var (
		g     errgroup.Group
		errMu sync.Mutex
		errs  []error
	)
	for _, src := range sources {
		src := src
		g.Go(func() error {
			seeder := func(conn *sql.DB, code, province string, c *http.Client, d time.Duration) {
				_ = ensureProvincialMembersForSource(conn, c, d, ProvincialSource{Code: code, Province: province})
			}
			if err := provincial.CrawlProvinceSource(conn, client, delay, src, enqueueSummary, seeder); err != nil {
				clog.Infof("[provincial] %s: %v", src.Code, err)
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
	store.UpsertProfiles(conn, toDBMembers(profiles), delay)
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
