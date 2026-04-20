package scraper

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/philspins/open-democracy/internal/db"
	"github.com/philspins/open-democracy/internal/scraper/provincial"
	"github.com/philspins/open-democracy/internal/utils"
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
		var lopSummary string
		if ok {
			lopSummary = CrawlLibraryOfParliamentSummary(
				utils.BillNumberFromID(stub.ID), parl, sess, client,
			)
			time.Sleep(delay)
		}

		bill := db.Bill{
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
			SummaryLoP:       lopSummary,
			LastScraped:      utils.NowISO(),
		}
		if !ok {
			bill.Parliament = 0
			bill.Session = 0
		}
		if err := db.UpsertBill(conn, bill); err != nil {
			log.Printf("[bills] upsert %s: %v", stub.ID, err)
		}
		for _, stage := range detail.Stages {
			db.UpsertBillStage(conn, db.BillStage{
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
	db.UpsertProfiles(conn, toDBMembers(profiles), delay)

	setSlugsSorted := make([]string, 0, len(ProvincialLegislatureAPIs))
	for slug := range ProvincialLegislatureAPIs {
		setSlugsSorted = append(setSlugsSorted, slug)
	}
	sort.Strings(setSlugsSorted)
	for _, setSlug := range setSlugsSorted {
		provProfiles, perr := CrawlProvincialMembersFromAPI(setSlug, "", client)
		if perr != nil {
			log.Printf("[members] provincial set %s: %v", setSlug, perr)
			continue
		}
		// The Represent API for nb-legislature currently returns 0 members.
		// Fall back to scraping the NB legislature website directly.
		if len(provProfiles) == 0 && setSlug == "nb-legislature" {
			log.Printf("[members] nb-legislature: Represent API returned 0 members; falling back to NB website scraper")
			nbProfiles, nberr := CrawlNewBrunswickMembersFromWebsite("", client)
			if nberr != nil {
				log.Printf("[members] nb-legislature website fallback: %v", nberr)
			} else {
				provProfiles = nbProfiles
			}
		}
		db.UpsertProfiles(conn, toDBMembers(provProfiles), delay)
	}
	return nil
}

func toDBMembers(profiles []MemberProfile) []db.Member {
	out := make([]db.Member, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, db.Member{
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
		if err := db.UpsertSittingDate(conn, CurrentParliament, CurrentSession, d); err != nil {
			log.Printf("[calendar] upsert %s: %v", d, err)
		}
	}
	return nil
}

// CrawlVotes indexes commons votes and fills detail rows when needed.
func CrawlVotes(conn *sql.DB, client *http.Client, delay time.Duration, indexURL string) error {
	divs, err := CrawlVotesIndex(indexURL, CurrentParliament, CurrentSession, client)
	if err != nil {
		return err
	}
	for _, div := range divs {
		existed, err := db.DivisionExists(conn, div.ID)
		if err != nil {
			log.Printf("[votes] exists check %s: %v", div.ID, err)
		}

		if err := db.UpsertDivision(conn, db.Division{
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
			hasVotes, err := db.DivisionHasVotes(conn, div.ID)
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
				db.UpsertMemberVote(conn, v.DivisionID, v.MemberID, v.Vote)
			}
			time.Sleep(delay)
		}
	}
	return nil
}

// CrawlSenate indexes senate votes and fills detail rows when needed.
func CrawlSenate(conn *sql.DB, client *http.Client, delay time.Duration, indexURL string) error {
	divs, err := CrawlSenateVotesIndex(indexURL, CurrentParliament, CurrentSession, client)
	if err != nil {
		return err
	}
	for _, div := range divs {
		existed, err := db.DivisionExists(conn, div.ID)
		if err != nil {
			log.Printf("[senate] exists check %s: %v", div.ID, err)
		}

		if err := db.UpsertDivision(conn, db.Division{
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
			hasVotes, err := db.DivisionHasVotes(conn, div.ID)
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
				db.UpsertMemberVote(conn, v.DivisionID, v.MemberID, v.Vote)
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
	db.UpsertProfiles(conn, toDBMembers(profiles), delay)
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
