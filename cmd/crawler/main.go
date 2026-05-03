// Command crawler is the Open Docket data-crawling CLI.
//
// Usage:
//
//	crawler [flags]
//
// Flags:
//
//	--bills              Crawl bills only (LEGISinfo RSS + detail)
//	--votes              Crawl Commons votes only
//	--senate             Crawl Senate votes only
//	--members            Crawl MP profiles only
//	--calendar           Crawl sitting calendar only
//	--provincial         Crawl all provincial bills and votes
//	--province CODES     Comma-separated province codes to crawl (e.g. pe,on,bc).
//	                     Implies --provincial; ignored when --provincial is not set.
//	--schedule           Run the background scheduler (blocks indefinitely)
//	--db PATH            Path to SQLite database file (default: opendocket.db)
//	--delay MS           Milliseconds between HTTP requests (default: 500)
//	--parallelism N      Max domain crawlers to run concurrently (default: 5, env: CRAWLER_PARALLELISM)
//	--log-level LEVEL    Log verbosity: info (default) or debug
//
// If no specific domain flag is provided, all crawlers run once.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/philspins/opendocket/internal/clog"
	"github.com/philspins/opendocket/internal/db"
	"github.com/philspins/opendocket/internal/scheduler"
	"github.com/philspins/opendocket/internal/scraper"
	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/summarizer"
	"github.com/philspins/opendocket/internal/utils"
)

func main() {
	if err := utils.LoadDotEnv(".env"); err != nil {
		clog.Debugf("warning: could not load .env: %v", err)
	}

	billsFlag := flag.Bool("bills", false, "Crawl bills only")
	votesFlag := flag.Bool("votes", false, "Crawl Commons votes only")
	senateFlag := flag.Bool("senate", false, "Crawl Senate votes only")
	provincialFlag := flag.Bool("provincial", false, "Crawl provincial bills and votes")
	provinceCodes := flag.String("province", "", "Comma-separated province codes to crawl (e.g. pe,on,bc); implies --provincial")
	membersFlag := flag.Bool("members", false, "Crawl MP profiles only")
	calendarFlag := flag.Bool("calendar", false, "Crawl sitting calendar only")
	scheduleFlag := flag.Bool("schedule", false, "Run the background scheduler (blocks indefinitely)")
	dbPath := flag.String("db", db.DefaultPath, "Path to SQLite database file")
	delayMS := flag.Int("delay", 500, "Milliseconds between HTTP requests")
	parallelism := flag.Int("parallelism", scraper.DefaultParallelism(), "Max domain crawlers to run concurrently (env: CRAWLER_PARALLELISM)")
	logLevel := flag.String("log-level", "info", "Log verbosity: info or debug")
	flag.Parse()

	if strings.ToLower(*logLevel) == "debug" {
		clog.SetLevel(clog.LevelDebug)
	}
	log.SetFlags(log.LstdFlags)

	if *provinceCodes != "" {
		*provincialFlag = true
	}
	var provinceFilter []string
	if *provinceCodes != "" {
		for _, c := range strings.Split(*provinceCodes, ",") {
			if c := strings.ToLower(strings.TrimSpace(c)); c != "" {
				provinceFilter = append(provinceFilter, c)
			}
		}
	}

	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	delay := time.Duration(*delayMS) * time.Millisecond
	client := utils.NewHTTPClient()

	// ── Scheduler mode ───────────────────────────────────────────────────────
	if *scheduleFlag {
		p := *parallelism
		scheduler.Start(scheduler.Config{
			DB: conn,
			FullCrawlFn: func(sdb *sql.DB) error {
				return runAll(sdb, client, delay, p)
			},
			FrequentVoteCheck: func(sdb *sql.DB) error {
				return runFrequentVoteCheck(sdb, client, delay, "")
			},
			AISummarizationFn: func(ctx context.Context, sdb *sql.DB) (int, error) {
				return summarizer.SummarizeNewBills(ctx, sdb, true)
			},
		})
		return // never reached
	}

	// ── One-shot mode ────────────────────────────────────────────────────────
	shouldRunAll := !(*billsFlag || *votesFlag || *senateFlag || *provincialFlag || *membersFlag || *calendarFlag)

	// Wire up the same channel-based summarization pipeline used by runAll.
	// The producer (crawlBills) emits requests while crawling; the consumer
	// goroutine calls Claude concurrently.  The channel is only created when
	// bills are included in this run; other crawlers are not affected.
	type summaryRunResult struct {
		processed int
		err       error
	}
	var summaryRequests chan summarizer.BillSummaryRequest
	var summaryResultCh chan summaryRunResult
	if *billsFlag || *provincialFlag || shouldRunAll {
		summaryRequests = make(chan summarizer.BillSummaryRequest, 32)
		summaryResultCh = make(chan summaryRunResult, 1)
		go func() {
			n, err := summarizer.SummarizeBillsFromChannel(context.Background(), conn, summaryRequests)
			summaryResultCh <- summaryRunResult{processed: n, err: err}
		}()
	}

	// Phase 1: crawl members before any bills or votes so that vote-member
	// linkage can resolve against freshly-stored member records.
	if *membersFlag || shouldRunAll {
		if err := scraper.CrawlMembers(conn, client, delay, ""); err != nil {
			clog.Infof("[main] members error: %v", err)
		}
	}

	// Phase 2: crawl bills, calendar, and provincial concurrently.
	// Votes must not start until bills are persisted — divisions.bill_id is a FK
	// into bills, so inserting a vote before its bill exists causes a constraint error.
	type task struct {
		name string
		fn   func() error
	}
	run := func(tasks []task) {
		fns := make([]func(), len(tasks))
		for i, t := range tasks {
			t := t
			fns[i] = func() {
				if err := t.fn(); err != nil {
					clog.Infof("[main] %s error: %v", t.name, err)
				}
			}
		}
		scraper.RunParallel(*parallelism, fns)
	}

	var billTasks []task
	if *calendarFlag || shouldRunAll {
		billTasks = append(billTasks, task{"calendar", func() error { return scraper.CrawlCalendar(conn, client, delay, "") }})
	}
	if *billsFlag || shouldRunAll {
		billTasks = append(billTasks, task{"bills", func() error {
			return scraper.CrawlBills(conn, client, delay, "", func(billID, billTitle, fullTextURL, lastActivityDate string) {
				if summaryRequests == nil || strings.TrimSpace(fullTextURL) == "" {
					return
				}
				summaryRequests <- summarizer.BillSummaryRequest{
					BillID:           billID,
					BillTitle:        billTitle,
					FullTextURL:      fullTextURL,
					LastActivityDate: lastActivityDate,
				}
			})
		}})
	}
	if *provincialFlag || shouldRunAll {
		filter := provinceFilter
		if shouldRunAll {
			filter = nil
		}
		billTasks = append(billTasks, task{"provincial", func() error {
			return scraper.CrawlProvincial(conn, client, delay, *parallelism, filter, func(billID, billTitle, fullTextURL, lastActivityDate string) {
				if summaryRequests == nil || strings.TrimSpace(fullTextURL) == "" {
					return
				}
				summaryRequests <- summarizer.BillSummaryRequest{
					BillID:           billID,
					BillTitle:        billTitle,
					FullTextURL:      fullTextURL,
					LastActivityDate: lastActivityDate,
				}
			})
		}})
	}
	run(billTasks)

	// Phase 3: crawl votes after all bills are in the DB.
	var voteTasks []task
	if *votesFlag || shouldRunAll {
		voteTasks = append(voteTasks, task{"votes", func() error { return scraper.CrawlVotes(conn, client, delay, "") }})
	}
	if *senateFlag || shouldRunAll {
		voteTasks = append(voteTasks, task{"senate", func() error { return scraper.CrawlSenate(conn, client, delay, "") }})
	}
	run(voteTasks)

	// Signal the summarizer worker that all bills have been submitted, then
	// wait for it to finish and log the result.
	if summaryRequests != nil {
		close(summaryRequests)
		res := <-summaryResultCh
		if res.err != nil {
			clog.Infof("[main] ai summarization pipeline error: %v", res.err)
		} else {
			clog.Infof("[main] ai summaries generated: %d", res.processed)
		}
	}

	clog.Infof("[main] done")
}

// ── scheduled helpers ─────────────────────────────────────────────────────────

func runAll(conn *sql.DB, client *http.Client, delay time.Duration, parallelism int) error {
	type summaryRunResult struct {
		processed int
		err       error
	}
	summaryRequests := make(chan summarizer.BillSummaryRequest, 32)
	summaryResultCh := make(chan summaryRunResult, 1)
	go func() {
		n, err := summarizer.SummarizeBillsFromChannel(context.Background(), conn, summaryRequests)
		summaryResultCh <- summaryRunResult{processed: n, err: err}
	}()

	// Phase 1: crawl members before any bills or votes so that vote-member
	// linkage can resolve against freshly-stored member records.
	scraper.CrawlMembers(conn, client, delay, "")

	// Phase 2: crawl bills and calendar concurrently. Votes must not start
	// until bills are persisted, otherwise divisions.bill_id FK constraints fail.
	scraper.RunParallel(parallelism, []func(){
		func() { scraper.CrawlCalendar(conn, client, delay, "") },
		func() {
			scraper.CrawlBills(conn, client, delay, "", func(billID, billTitle, fullTextURL, lastActivityDate string) {
				if strings.TrimSpace(fullTextURL) == "" {
					return
				}
				summaryRequests <- summarizer.BillSummaryRequest{
					BillID:           billID,
					BillTitle:        billTitle,
					FullTextURL:      fullTextURL,
					LastActivityDate: lastActivityDate,
				}
			})
		},
		func() {
			scraper.CrawlProvincial(conn, client, delay, parallelism, nil, func(billID, billTitle, fullTextURL, lastActivityDate string) {
				if strings.TrimSpace(fullTextURL) == "" {
					return
				}
				summaryRequests <- summarizer.BillSummaryRequest{
					BillID:           billID,
					BillTitle:        billTitle,
					FullTextURL:      fullTextURL,
					LastActivityDate: lastActivityDate,
				}
			})
		},
	})

	// Phase 3: crawl votes only after all bills are in the DB.
	scraper.RunParallel(parallelism, []func(){
		func() { scraper.CrawlVotes(conn, client, delay, "") },
		func() { scraper.CrawlSenate(conn, client, delay, "") },
	})
	close(summaryRequests)
	res := <-summaryResultCh
	if res.err != nil {
		return fmt.Errorf("summarization pipeline: %w", res.err)
	}
	clog.Infof("[scheduler] ai summaries generated: %d", res.processed)
	return nil
}

func runFrequentVoteCheck(conn *sql.DB, client *http.Client, delay time.Duration, votesURL string) error {
	dates, err := store.SittingDates(conn, scraper.CurrentParliament, scraper.CurrentSession)
	if err != nil {
		return err
	}
	if !scraper.ParliamentIsSitting(dates, "") {
		clog.Infof("[scheduler] parliament not sitting today — skipping frequent vote check")
		return nil
	}
	return scraper.CrawlVotes(conn, client, delay, votesURL)
}
