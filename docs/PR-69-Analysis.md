# PR 69 Analysis

## Overview

When building PR-69 I did not follow best practices for commit cleanliness. I started on an out-of-date main commit, and after pushing a few commits I noticed a message at the bottom of the page saying my branch was out of date, so I clicked the "Update with rebase" button.

In addition to this, the scope creeped far beyong the initial goal of solving [issue 64](https://github.com/philspins/opendocket/issues/64). I attempted to make many unrelated crawler improvements, and also included fixes for [issue 40](https://github.com/philspins/opendocket/issues/40) and [issue 38](https://github.com/philspins/opendocket/issues/38).

After pushing a large number of local commits that had built up, I received a notification that the PR check had failed due to broken tests. This is when I realized how bad the situation was, and when I started looking into the test failures I discovered that many changes merged into main from the [previous PR](https://github.com/philspins/opendocket/pull/66) had been reverted due to the rebase.  Attempting to find all the regressions proved very difficult due to the amount of scope creep, so I decided to create this document to try and collect the list of changes and document some of the problems identified.

## Summary of changes

## Fix more regressions (88eb3ceaa9f8a62961d0032b574615fb242f5e52)

Replace GET/POST /feedback handlers, use authentication check via IsAuthenticated() when rendering home, and use the selected jurisdiction level when listing members for comparison. Update layout CSS to include an alignment column in member vote tables and regenerate the compiled layout template source.

## Logging updates (4fcc09822d99af43684d3b7159bded30510c8e4e)

Changed some log.Printf calls to use clog.Infof

## Fix some reverted changes (32bd127f700c97336b197399449ee84bef45b00e)

Attempted to replace reverted code related to bill filtering, footer, and compare page (which all should have remained unchanged)

## Fix failing unit tests (4b598e42ec958778cff28dcab03150a819898ece)

After pushing the previous 4 commits to Github I discovered that we had failing tests and many regression errors. Claude fixed these to the best of its ability, but things were still not perfect after this.

### Crawler improvements; Add clog logger and --log-level flag (b45baae058dbedca99db867cbc8782de17139558)

Introduce internal/clog logging package and switch existing logging to use it across the crawler and scraper code. Replace the old verbose flag with a --log-level option (info|debug), call clog.SetLevel when debug is selected, and update log calls (Printf/Println) to clog.Infof/Debugf where appropriate. Also add the new logger implementation and remove the checked-in crawler.log file.

## Fix NS vote crawling (62b93b0dedcdb190a97cbb521d669c01d7b4d6f2)

Nova Scotia votes (primary fix)

- Added an HTML-first crawl path for NS Hansard votes: discovers individual day-page links (e.g. /hansard-debates/assembly-65-session-1/house_26apr09), parses each page's YEAS/NAYS tables directly, extracts dates from URL slugs.
- PDF parsing remains as a fallback for older sessions.
- Added nsDescriptionForVoteTable to extract bill/motion context from preceding `<b>` siblings.

Vote description extraction (provincial/votes.go)

- Rewrote the description extraction logic to be smarter: looks back up to 700 characters, strips procedural boilerplate ("the debate being ended", "question being put", etc.), and prioritises substantive clauses mentioning bill/motion/amendment/resolution via a series of regex passes.

Senate vote parsing (senate.go)

- Added a modern sencanada.ca layout parser for #sc-vote-details-table with columns [Senator, Affiliation, Province, Yea, Nay, Abstention], using fa-times icon or data-order="aaa" to detect the marked vote
- Legacy layout kept as a fallback
- Added senateMemberIDFromHref to extract senator IDs from URLs.

Supporting changes

- votes.go: added DownloadAndExtractPDFTextForTest export, seeListUnderRe for cross-reference handling
- server.go, templates: layout/compare template cleanup (mostly deletions, likely extracting helpers)
- Tests added/updated for all the above in ns_test.go, senate_test.go, votes_test.go, nb_test.go

### Crawler improvements (2bbecd6e9ef0ddb63a6f00aae8e1882c47e8ed62)

Added retry logic to summarizer when fetching bill text. Added de-duplication for bill detail division votes, updated crawler to save the bill title for the division description. Added fallback for finding votes from division journal URL when vote table on bill page is empty. Removed delays from PE crawler. Updated PE crawler (postPEIWorkflow) to paginate over API results so that it retrieves all bills in a session. Removed delays and improved performance for members crawler.

### Change crawler order to force bills before votes (ce2f6f624d4317a314be0fc9e76f4a4a1cad6769)

Splits the crawler's Phase 2 into two sequential phases to fix a foreign-key constraint problem:

- Before: bills, calendar, provincial, votes, and senate were all run concurrently in one batch — meaning CrawlVotes/CrawlSenate could insert divisions rows referencing a bill_id FK before the bill existed in the DB, causing constraint errors.

- After:

  - Phase 2 runs bills, calendar, and provincial concurrently (all tasks that produce bills)
  - Phase 3 runs votes and senate concurrently only after Phase 2 completes (all tasks that reference bills via FK)

The same two-phase ordering is applied to both the main() flag-driven path and the runAll() helper. A small run(tasks) closure is also extracted to avoid repeating the parallel-dispatch boilerplate.

### Removed LOP summarization code & update layout of bill detail page (bec2cce6e3da43e8e6edec15eca04a70635a847c, de7b3c9f86a742d949635acaa9cc684a3ad2c372)

Change order of elements on bill detail page: move PlainSummary up to beneath the one-line summar, remove the LoP summary.

Completely removed all LOP summarization code.

### Add rate limiter and retries for Claude API (81b422ce3f2da11a8291fcc99dfa99d12300a214)

Introduce a package-level token bucket (claudeRateLimiter) to limit Anthropic requests per minute (default 15, overridable via ANTHROPIC_REQUESTS_PER_MINUTE). Implement tokenBucket with a pre-filled channel and wait(ctx) to block until a token is available. Wrap Claude API calls in a retry loop (max 5 attempts) with exponential backoff starting at 5s, handle 429 responses by honoring Retry-After (seconds or HTTP date), log rate-limited waits, and improve response reading/unmarshalling and error messages. Also set an HTTP client timeout for requests.

### Improve vote parsing, titles, and summarization (9f26f51613bcb50fddc669fa1b53c2df3fffeb94s)

Fix PDF date extraction and vote parsing for provincial journals: prefer dates found in PDF text for NB (avoid misreading opaque IDs), validate implausible years from URLs, and expand PEI division context extraction (strip boilerplate, widen context window) with new unit tests. Add BillTitle to store models and include COALESCE(b.short_title, b.title) in queries so UI can show bill short titles; update compare template to prefer BillTitle over generic description. Update summarizer pipeline prompt to produce simpler, 13-year-old-friendly summaries and adjust summarization logic/order (download LoP summaries earlier and remove LoP-based skip). Minor docs whitespace tweaks and regenerated template code (line-ending/templ runtime changes) from templ codegen.

## Issues to Investigate

### Bill text unavailable

The actual bill text was not available in the provided content, so specific changes cannot be listed accurately.
http://localhost:8080/bills/bc-43-2-5

### Recent Votes / Recorded Votes Descriptions

We still have many votes showing up with poor descriptions that say nothing about what was being voted on, and are not linked to a bill. See these pages for example:
- http://localhost:8080/members/ontario-legislature-adil-shamji
- http://localhost:8080/members/newfoundland-labrador-legislature-elvis-loveless

| Date | Bill | Description | Vote | Alignment | Result |
| :--- | :---: | :---: | :---: | :---: | ---: |
| Mar 13, 2024 | | The Speaker put the question and declared the resolution carried | Yea | ✓ party | Carried |
| Nov 15, 2023 | | Recorded division | Nay | ✓ party | Carried |

### Feedback form and new footer changes were removed

Fixed in commits 32bd127f700c97336b197399449ee84bef45b00e and 88eb3ceaa9f8a62961d0032b574615fb242f5e52

### Large number of unmatched votes

2026/05/03 16:33:59 [members] ontario-legislature: 123 members
2026/05/03 16:33:59 [members] pei-legislature: 26 members
2026/05/03 16:33:59 [members] quebec-assemblee-nationale: 124 members
2026/05/03 16:33:59 [members] saskatchewan-legislature: 61 members
2026/05/03 16:33:59 [members] yukon-legislature: 19 members
2026/05/03 16:33:59 [votes] fetching sitting calendar: https://www.ourcommons.ca/en/sitting-calendar
2026/05/03 16:33:59 [provincial] crawling Saskatchewan
2026/05/03 16:33:59 [provincial] crawling New Brunswick
2026/05/03 16:33:59 [provincial] crawling British Columbia
2026/05/03 16:33:59 [provincial] crawling Alberta
2026/05/03 16:33:59 [provincial] crawling Manitoba
2026/05/03 16:33:59 [provincial] crawling Quebec
2026/05/03 16:33:59 [bills] fetching RSS: https://www.parl.ca/legisinfo/en/bills/rss
2026/05/03 16:33:59 [provincial] crawling Newfoundland and Labrador
2026/05/03 16:33:59 [provincial] crawling Nova Scotia
2026/05/03 16:33:59 [provincial] crawling Prince Edward Island
2026/05/03 16:33:59 [provincial] crawling Ontario
2026/05/03 16:33:59 [provincial][nb] detected legislature/session: 61/2
2026/05/03 16:33:59 [provincial][nb] crawling legislature/session: 61/2
2026/05/03 16:33:59 [votes] found 117 sitting dates
2026/05/03 16:33:59 [provincial][bc] detected legislature/session: 43/2
2026/05/03 16:33:59 [provincial][bc] crawling legislature/session: 43/2
2026/05/03 16:33:59 [provincial][on] detected legislature/session: 44/1
2026/05/03 16:33:59 [provincial][on] crawling legislature/session: 44/1
2026/05/03 16:33:59 [ontario-votes] fetching session index: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1
2026/05/03 16:33:59 [provincial][nl] detected legislature/session: 1/1
2026/05/03 16:33:59 [provincial][nl] crawling legislature/session: 1/1
2026/05/03 16:33:59 [ontario-votes] found 67 sitting dates with V&P
2026/05/03 16:33:59 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-04-16/votes-proceedings
2026/05/03 16:33:59 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-04-14/votes-proceedings
2026/05/03 16:33:59 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-04-15/votes-proceedings
2026/05/03 16:34:00 [calendar] detected 119 dates for federal-commons
2026/05/03 16:34:00 [provincial][sk] detected legislature/session: 30/2
2026/05/03 16:34:00 [provincial][sk] crawling legislature/session: 30/2
2026/05/03 16:34:00 [ontario-votes] 2025-04-14: parsed 0 divisions
2026/05/03 16:34:00 [ontario-votes] 2025-04-16: parsed 0 divisions
2026/05/03 16:34:00 [ontario-votes] 2025-04-15: parsed 0 divisions
2026/05/03 16:34:00 [bills] RSS contained 158 bills
2026/05/03 16:34:00 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-1
2026/05/03 16:34:00 [nl-votes] fetching journals index: https://www.assembly.nl.ca/HouseBusiness/Journals/
2026/05/03 16:34:00 [sk-votes] fetching archive: https://www.legassembly.sk.ca/legislative-business/archive/?Start=&End=&Type=Assembly
2026/05/03 16:34:00 [provincial][pe] detected legislature/current session: 67/3; crawling sessions [3 2 1]
2026/05/03 16:34:00 [provincial][pe] crawling legislature/session: 67/3
2026/05/03 16:34:00 [sk-votes] found 12 Assembly Minutes HTML links
2026/05/03 16:34:00 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260428Minutes-HTML.htm
2026/05/03 16:34:00 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260430Minutes-HTML.htm
2026/05/03 16:34:00 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260429Minutes-HTML.htm
2026/05/03 16:34:00 [provincial][mb] detected legislature/session: 43/3
2026/05/03 16:34:00 [provincial][mb] crawling legislature/session: 43/3
2026/05/03 16:34:00 [mb-votes] fetching index: https://www.gov.mb.ca/legislature/business/votes_proceedings.html
2026/05/03 16:34:00 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-04-17/votes-proceedings
2026/05/03 16:34:00 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-04-29/votes-proceedings
2026/05/03 16:34:00 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-04-30/votes-proceedings
2026/05/03 16:34:00 [ontario-votes] 2025-04-17: parsed 0 divisions
2026/05/03 16:34:00 [ontario-votes] 2025-04-30: parsed 0 divisions
2026/05/03 16:34:00 [ontario-votes] 2025-04-29: parsed 0 divisions
2026/05/03 16:34:00 [nl-votes] 2023-10-25: parsed 2 divisions (outcome-only)
2026/05/03 16:34:00 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-2
2026/05/03 16:34:00 [sk-votes] 2026-04-30: parsed 1 divisions
2026/05/03 16:34:00 [nl-votes] 2023-10-26: parsed 1 divisions (outcome-only)
2026/05/03 16:34:00 [sk-votes] 2026-04-29: parsed 2 divisions
2026/05/03 16:34:00 [sk-votes] 2026-04-28: parsed 0 divisions
2026/05/03 16:34:00 [nl-votes] 2023-10-30: parsed 1 divisions (outcome-only)
2026/05/03 16:34:00 [pe-bills] page 1: 20 rows
2026/05/03 16:34:00 [nl-votes] 2023-10-31: parsed 2 divisions (outcome-only)
2026/05/03 16:34:00 [summarizer] bill text unavailable (404) for "45-1-s-1" (https://www.parl.ca/DocumentViewer/en/45-1/bill/S-1/first-reading); clearing full_text_url
2026/05/03 16:34:00 [nl-votes] 2023-11-01: parsed 1 divisions (outcome-only)
2026/05/03 16:34:00 [bc-votes] fetching LIMS index: https://lims.leg.bc.ca/pdms/votes-and-proceedings/43rd2nd
2026/05/03 16:34:01 [nl-votes] 2023-11-02: parsed 1 divisions (outcome-only)
2026/05/03 16:34:01 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-01/votes-proceedings
2026/05/03 16:34:01 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-05/votes-proceedings
2026/05/03 16:34:01 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-06/votes-proceedings
2026/05/03 16:34:01 [ontario-votes] 2025-05-01: parsed 0 divisions
2026/05/03 16:34:01 [ontario-votes] 2025-05-06: parsed 2 divisions
2026/05/03 16:34:01 [ontario-votes] 2025-05-05: parsed 1 divisions
2026/05/03 16:34:01 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260427Minutes-HTML.htm
2026/05/03 16:34:01 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260423Minutes-HTML.htm
2026/05/03 16:34:01 [nl-votes] 2023-11-16: parsed 0 divisions (outcome-only)
2026/05/03 16:34:01 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260422Minutes-HTML.htm
2026/05/03 16:34:01 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-3
2026/05/03 16:34:01 [nl-votes] 2024-02-21: parsed 0 divisions (outcome-only)
2026/05/03 16:34:01 [sk-votes] 2026-04-27: parsed 0 divisions
2026/05/03 16:34:01 [bc-votes] LIMS index: 32 VP files for 43rd2nd
2026/05/03 16:34:01 [nl-votes] 2024-03-04: parsed 1 divisions (outcome-only)
2026/05/03 16:34:01 [summarizer] skip unchanged bill "45-1-s-2"
2026/05/03 16:34:01 [sk-votes] 2026-04-23: parsed 0 divisions
2026/05/03 16:34:01 [sk-votes] 2026-04-22: parsed 0 divisions
2026/05/03 16:34:01 [nl-votes] 2024-03-05: parsed 2 divisions (outcome-only)
2026/05/03 16:34:01 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-07/votes-proceedings
2026/05/03 16:34:01 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-08/votes-proceedings
2026/05/03 16:34:01 [nl-votes] 2024-03-06: parsed 2 divisions (outcome-only)
2026/05/03 16:34:01 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-12/votes-proceedings
2026/05/03 16:34:01 [ontario-votes] 2025-05-08: parsed 0 divisions
2026/05/03 16:34:01 [ontario-votes] 2025-05-07: parsed 0 divisions
2026/05/03 16:34:01 [ontario-votes] 2025-05-12: parsed 2 divisions
2026/05/03 16:34:01 [nl-votes] 2024-03-07: parsed 1 divisions (outcome-only)
2026/05/03 16:34:01 [nl-votes] 2024-03-11: parsed 1 divisions (outcome-only)
2026/05/03 16:34:01 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260421Minutes-HTML.htm
2026/05/03 16:34:01 [nl-votes] 2024-03-12: parsed 2 divisions (outcome-only)
2026/05/03 16:34:01 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-4
2026/05/03 16:34:01 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260420Minutes-HTML.htm
2026/05/03 16:34:01 [sk-votes] 2026-04-21: parsed 1 divisions
2026/05/03 16:34:02 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260416Minutes-HTML.htm
2026/05/03 16:34:02 [sk-votes] 2026-04-20: parsed 0 divisions
2026/05/03 16:34:02 [nl-votes] 2024-03-20: parsed 0 divisions (outcome-only)
2026/05/03 16:34:02 [summarizer] skip unchanged bill "45-1-s-3"
2026/05/03 16:34:02 [sk-votes] 2026-04-16: parsed 1 divisions
2026/05/03 16:34:02 [provincial][ab] detected legislature/session: 31/2
2026/05/03 16:34:02 [provincial][ab] crawling legislature/session: 31/2
2026/05/03 16:34:02 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-13/votes-proceedings
2026/05/03 16:34:02 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-14/votes-proceedings
2026/05/03 16:34:02 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-15/votes-proceedings
2026/05/03 16:34:02 [ontario-votes] 2025-05-14: parsed 0 divisions
2026/05/03 16:34:02 [ontario-votes] 2025-05-13: parsed 0 divisions
2026/05/03 16:34:02 [ontario-votes] 2025-05-15: parsed 0 divisions
2026/05/03 16:34:02 [nl-votes] 2024-04-15: parsed 0 divisions (outcome-only)
2026/05/03 16:34:02 [nl-votes] 2024-04-16: parsed 1 divisions (outcome-only)
2026/05/03 16:34:02 [ab-votes] fetching index: https://www.assembly.ab.ca/assembly-business/assembly-records/votes-and-proceedings
2026/05/03 16:34:02 [nl-votes] 2024-04-17: parsed 0 divisions (outcome-only)
2026/05/03 16:34:02 [bc-votes] 2026-02-19: parsed 1 divisions
2026/05/03 16:34:02 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260415Minutes-HTML.htm
2026/05/03 16:34:02 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-5
2026/05/03 16:34:02 [nl-votes] 2024-04-18: parsed 0 divisions (outcome-only)
2026/05/03 16:34:02 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260414Minutes-HTML.htm
2026/05/03 16:34:02 [sk-votes] scraping Minutes: https://docs.legassembly.sk.ca/legdocs/Assembly/Minutes/30L2S/20260413Minutes-HTML.htm
2026/05/03 16:34:02 [sk-votes] 2026-04-15: parsed 1 divisions
2026/05/03 16:34:02 [nl-votes] 2024-04-23: parsed 2 divisions (outcome-only)
2026/05/03 16:34:02 [summarizer] skip unchanged bill "45-1-s-4"
2026/05/03 16:34:02 [sk-votes] 2026-04-13: parsed 0 divisions
2026/05/03 16:34:02 [bc-votes] 2026-02-23: parsed 1 divisions
2026/05/03 16:34:02 [nl-votes] 2024-04-24: parsed 5 divisions (outcome-only)
2026/05/03 16:34:02 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-26/votes-proceedings
2026/05/03 16:34:02 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-27/votes-proceedings
2026/05/03 16:34:02 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-28/votes-proceedings
2026/05/03 16:34:02 [sk-votes] 2026-04-14: parsed 1 divisions
2026/05/03 16:34:02 [ontario-votes] 2025-05-26: parsed 0 divisions
2026/05/03 16:34:02 [ontario-votes] 2025-05-27: parsed 0 divisions
2026/05/03 16:34:02 [ontario-votes] 2025-05-28: parsed 2 divisions
2026/05/03 16:34:02 [nl-votes] 2024-04-25: parsed 0 divisions (outcome-only)
2026/05/03 16:34:02 [nl-votes] 2024-04-29: parsed 1 divisions (outcome-only)
2026/05/03 16:34:02 [nl-votes] 2024-04-30: parsed 3 divisions (outcome-only)
2026/05/03 16:34:03 [nl-votes] 2024-05-01: parsed 1 divisions (outcome-only)
2026/05/03 16:34:03 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-05-29/votes-proceedings
2026/05/03 16:34:03 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-06-02/votes-proceedings
2026/05/03 16:34:03 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-6
2026/05/03 16:34:03 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-06-03/votes-proceedings
2026/05/03 16:34:03 [ontario-votes] 2025-06-03: parsed 6 divisions
2026/05/03 16:34:03 [ontario-votes] 2025-06-02: parsed 7 divisions
2026/05/03 16:34:03 [summarizer] skip unchanged bill "sk-30-2-24"
2026/05/03 16:34:03 [provincial][ns] detected legislature/session: 65/1
2026/05/03 16:34:03 [provincial][ns] crawling legislature/session: 65/1
2026/05/03 16:34:03 [ontario-votes] 2025-05-29: parsed 2 divisions
2026/05/03 16:34:03 [bc-votes] 2026-02-26: parsed 3 divisions
2026/05/03 16:34:03 [summarizer] skip unchanged bill "45-1-s-5"
2026/05/03 16:34:03 [nl-votes] 2024-05-13: parsed 0 divisions (outcome-only)
2026/05/03 16:34:03 [nl-votes] 2024-05-14: parsed 1 divisions (outcome-only)
2026/05/03 16:34:03 [calendar] detected 74 dates for federal-senate
2026/05/03 16:34:03 [nl-votes] 2024-05-16: parsed 1 divisions (outcome-only)
2026/05/03 16:34:03 [nb-votes] parsed 47 divisions from 60 PDFs
2026/05/03 16:34:03 [summarizer] skip unchanged bill "nb-61-2-14"
2026/05/03 16:34:03 [nl-votes] 2024-05-21: parsed 1 divisions (outcome-only)
2026/05/03 16:34:03 [summarizer] skip unchanged bill "sk-30-2-25"
2026/05/03 16:34:03 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-06-04/votes-proceedings
2026/05/03 16:34:03 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-06-05/votes-proceedings
2026/05/03 16:34:03 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-10-20/votes-proceedings
2026/05/03 16:34:03 [nl-votes] 2024-05-22: parsed 0 divisions (outcome-only)
2026/05/03 16:34:03 [calendar] detected 81 dates for provincial-AB
2026/05/03 16:34:03 [ontario-votes] 2025-06-04: parsed 4 divisions
2026/05/03 16:34:03 [ontario-votes] 2025-06-05: parsed 2 divisions
2026/05/03 16:34:03 [ontario-votes] 2025-10-20: parsed 1 divisions
2026/05/03 16:34:03 [nl-votes] 2024-05-23: parsed 1 divisions (outcome-only)
2026/05/03 16:34:03 [calendar] detected 365 dates for provincial-BC
2026/05/03 16:34:04 [nl-votes] 2024-05-27: parsed 1 divisions (outcome-only)
2026/05/03 16:34:04 [ns-votes] fetching hansard session index for html: https://nslegislature.ca/legislative-business/hansard-debates/assembly-65-session-1
2026/05/03 16:34:04 [ab-votes] parsed 5 divisions
2026/05/03 16:34:04 [nl-votes] 2024-05-28: parsed 1 divisions (outcome-only)
2026/05/03 16:34:04 [summarizer] skip unchanged bill "nb-61-2-15"
2026/05/03 16:34:04 [nl-votes] 2024-05-29: parsed 1 divisions (outcome-only)
2026/05/03 16:34:04 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-201
2026/05/03 16:34:04 [bc-votes] 2026-03-05: parsed 5 divisions
2026/05/03 16:34:04 [summarizer] skip unchanged bill "sk-30-2-26"
2026/05/03 16:34:04 [nl-votes] 2024-06-26: parsed 0 divisions (outcome-only)
2026/05/03 16:34:04 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-10-21/votes-proceedings
2026/05/03 16:34:04 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-10-22/votes-proceedings
2026/05/03 16:34:04 [nl-votes] 2024-09-13: parsed 0 divisions (outcome-only)
2026/05/03 16:34:04 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-10-23/votes-proceedings
2026/05/03 16:34:04 [ontario-votes] 2025-10-21: parsed 0 divisions
2026/05/03 16:34:04 [ontario-votes] 2025-10-22: parsed 0 divisions
2026/05/03 16:34:04 [bc-votes] 2026-03-09: parsed 1 divisions
2026/05/03 16:34:04 [ontario-votes] 2025-10-23: parsed 1 divisions
2026/05/03 16:34:04 [nl-votes] 2024-11-04: parsed 1 divisions (outcome-only)
2026/05/03 16:34:04 [summarizer] skip unchanged bill "45-1-s-6"
2026/05/03 16:34:04 [nl-votes] 2024-11-05: parsed 0 divisions (outcome-only)
2026/05/03 16:34:04 [nl-votes] 2024-11-06: parsed 1 divisions (outcome-only)
2026/05/03 16:34:04 [summarizer] skip unchanged bill "nb-61-2-16"
2026/05/03 16:34:04 [nl-votes] 2024-11-07: parsed 0 divisions (outcome-only)
2026/05/03 16:34:04 [summarizer] skip unchanged bill "sk-30-2-27"
2026/05/03 16:34:04 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-202
2026/05/03 16:34:04 [nl-votes] 2024-11-12: parsed 0 divisions (outcome-only)
2026/05/03 16:34:04 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-10-27/votes-proceedings
2026/05/03 16:34:04 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-10-28/votes-proceedings
2026/05/03 16:34:04 [nl-votes] 2024-11-13: parsed 0 divisions (outcome-only)
2026/05/03 16:34:04 [ontario-votes] 2025-10-28: parsed 0 divisions
2026/05/03 16:34:04 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-10-29/votes-proceedings
2026/05/03 16:34:04 [ontario-votes] 2025-10-27: parsed 0 divisions
2026/05/03 16:34:04 [summarizer] skip unchanged bill "45-1-s-201"
2026/05/03 16:34:05 [ontario-votes] 2025-10-29: parsed 3 divisions
2026/05/03 16:34:05 [nl-votes] 2024-11-14: parsed 0 divisions (outcome-only)
2026/05/03 16:34:05 [bc-votes] 2026-03-12: parsed 3 divisions
2026/05/03 16:34:05 [nl-votes] 2024-11-18: parsed 0 divisions (outcome-only)
2026/05/03 16:34:05 [calendar] detected 195 dates for provincial-MB
2026/05/03 16:34:05 [nl-votes] 2024-11-19: parsed 3 divisions (outcome-only)
2026/05/03 16:34:05 [summarizer] skip unchanged bill "nb-61-2-17"
2026/05/03 16:34:05 [summarizer] skip unchanged bill "sk-30-2-28"
2026/05/03 16:34:05 [ab-votes] parsed 1 divisions
2026/05/03 16:34:05 [nl-votes] 2024-11-20: parsed 2 divisions (outcome-only)
2026/05/03 16:34:05 [bc-votes] 2026-03-30: parsed 1 divisions
2026/05/03 16:34:05 [nl-votes] 2024-11-21: parsed 0 divisions (outcome-only)
2026/05/03 16:34:05 [ab-votes] parsed 3 divisions
2026/05/03 16:34:05 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-10-30/votes-proceedings
2026/05/03 16:34:05 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-03/votes-proceedings
2026/05/03 16:34:05 [calendar] detected 32 dates for provincial-NB
2026/05/03 16:34:05 [ontario-votes] 2025-10-30: parsed 2 divisions
2026/05/03 16:34:05 [ontario-votes] 2025-11-03: parsed 2 divisions
2026/05/03 16:34:05 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-04/votes-proceedings
2026/05/03 16:34:05 [nl-votes] 2024-12-02: parsed 1 divisions (outcome-only)
2026/05/03 16:34:05 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-203
2026/05/03 16:34:05 [ontario-votes] 2025-11-04: parsed 0 divisions
2026/05/03 16:34:05 [bc-votes] 2026-03-31: parsed 1 divisions
2026/05/03 16:34:05 [summarizer] skip unchanged bill "45-1-s-202"
2026/05/03 16:34:05 [summarizer] skip unchanged bill "nb-61-2-18"
2026/05/03 16:34:05 [nl-votes] 2024-12-04: parsed 2 divisions (outcome-only)
2026/05/03 16:34:05 [summarizer] skip unchanged bill "sk-30-2-29"
2026/05/03 16:34:05 [bc-votes] 2026-04-01: parsed 4 divisions
2026/05/03 16:34:05 [nl-votes] 2024-12-17: parsed 0 divisions (outcome-only)
2026/05/03 16:34:05 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-05/votes-proceedings
2026/05/03 16:34:06 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-06/votes-proceedings
2026/05/03 16:34:06 [ontario-votes] 2025-11-05: parsed 0 divisions
2026/05/03 16:34:06 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-17/votes-proceedings
2026/05/03 16:34:06 [ontario-votes] 2025-11-06: parsed 1 divisions
2026/05/03 16:34:06 [nl-votes] 2025-01-03: parsed 0 divisions (outcome-only)
2026/05/03 16:34:06 [calendar] detected 68 dates for provincial-NL
2026/05/03 16:34:06 [ontario-votes] 2025-11-17: parsed 6 divisions
2026/05/03 16:34:06 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-204
2026/05/03 16:34:06 [nl-votes] 2025-01-06: parsed 1 divisions (outcome-only)
2026/05/03 16:34:06 [summarizer] skip unchanged bill "nb-61-2-19"
2026/05/03 16:34:06 [nl-votes] 2025-01-07: parsed 0 divisions (outcome-only)
2026/05/03 16:34:06 [bc-votes] 2026-04-13: parsed 3 divisions
2026/05/03 16:34:06 [summarizer] skip unchanged bill "45-1-s-203"
2026/05/03 16:34:06 [summarizer] skip unchanged bill "sk-30-2-30"
2026/05/03 16:34:06 [nl-votes] 2025-01-08: parsed 0 divisions (outcome-only)
2026/05/03 16:34:06 [nl-votes] 2025-03-03: parsed 0 divisions (outcome-only)
2026/05/03 16:34:06 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-18/votes-proceedings
2026/05/03 16:34:06 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-19/votes-proceedings
2026/05/03 16:34:06 [ab-votes] parsed 1 divisions
2026/05/03 16:34:06 [nl-votes] 2025-03-04: parsed 1 divisions (outcome-only)
2026/05/03 16:34:06 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-20/votes-proceedings
2026/05/03 16:34:06 [ontario-votes] 2025-11-18: parsed 0 divisions
2026/05/03 16:34:06 [ontario-votes] 2025-11-19: parsed 2 divisions
2026/05/03 16:34:06 [ontario-votes] 2025-11-20: parsed 0 divisions
2026/05/03 16:34:06 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-205
2026/05/03 16:34:06 [nl-votes] 2025-03-05: parsed 2 divisions (outcome-only)
2026/05/03 16:34:06 [summarizer] skip unchanged bill "nb-61-2-20"
2026/05/03 16:34:06 [summarizer] skip unchanged bill "sk-30-2-31"
2026/05/03 16:34:06 [ab-votes] parsed 2 divisions
2026/05/03 16:34:06 [nl-votes] 2025-03-06: parsed 0 divisions (outcome-only)
2026/05/03 16:34:06 [summarizer] skip unchanged bill "45-1-s-204"
2026/05/03 16:34:06 [nl-votes] 2025-03-10: parsed 1 divisions (outcome-only)
2026/05/03 16:34:06 [ab-votes] parsed 2 divisions
2026/05/03 16:34:07 [bc-votes] 2026-04-20: parsed 1 divisions
2026/05/03 16:34:07 [nl-votes] 2025-03-12: parsed 1 divisions (outcome-only)
2026/05/03 16:34:07 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-24/votes-proceedings
2026/05/03 16:34:07 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-25/votes-proceedings
2026/05/03 16:34:07 [ab-votes] parsed 1 divisions
2026/05/03 16:34:07 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-26/votes-proceedings
2026/05/03 16:34:07 [nl-votes] 2025-04-09: parsed 4 divisions (outcome-only)
2026/05/03 16:34:07 [ontario-votes] 2025-11-24: parsed 6 divisions
2026/05/03 16:34:07 [ontario-votes] 2025-11-25: parsed 1 divisions
2026/05/03 16:34:07 [ontario-votes] 2025-11-26: parsed 1 divisions
2026/05/03 16:34:07 [summarizer] skip unchanged bill "nb-61-2-21"
2026/05/03 16:34:07 [bc-votes] 2026-04-21: parsed 1 divisions
2026/05/03 16:34:07 [nl-votes] 2025-04-10: parsed 0 divisions (outcome-only)
2026/05/03 16:34:07 [summarizer] skip unchanged bill "sk-30-2-32"
2026/05/03 16:34:07 [ab-votes] parsed 7 divisions
2026/05/03 16:34:07 [nl-votes] 2025-04-14: parsed 1 divisions (outcome-only)
2026/05/03 16:34:07 [nl-votes] 2025-04-15: parsed 0 divisions (outcome-only)
2026/05/03 16:34:07 [bc-votes] 2026-04-22: parsed 2 divisions
2026/05/03 16:34:07 [ab-votes] parsed 2 divisions
2026/05/03 16:34:07 [nl-votes] 2025-04-16: parsed 0 divisions (outcome-only)
2026/05/03 16:34:07 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-206
2026/05/03 16:34:07 [nl-votes] 2025-05-12: parsed 3 divisions (outcome-only)
2026/05/03 16:34:07 [ab-votes] parsed 1 divisions
2026/05/03 16:34:07 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-11-27/votes-proceedings
2026/05/03 16:34:07 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-12-01/votes-proceedings
2026/05/03 16:34:07 [nl-votes] 2025-05-13: parsed 1 divisions (outcome-only)
2026/05/03 16:34:07 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-12-02/votes-proceedings
2026/05/03 16:34:07 [summarizer] skip unchanged bill "45-1-s-205"
2026/05/03 16:34:07 [summarizer] skip unchanged bill "nb-61-2-22"
2026/05/03 16:34:07 [ontario-votes] 2025-11-27: parsed 1 divisions
2026/05/03 16:34:07 [ontario-votes] 2025-12-01: parsed 1 divisions
2026/05/03 16:34:07 [summarizer] skip unchanged bill "sk-30-2-33"
2026/05/03 16:34:07 [ontario-votes] 2025-12-02: parsed 2 divisions
2026/05/03 16:34:07 [bc-votes] 2026-04-27: parsed 1 divisions
2026/05/03 16:34:07 [nl-votes] 2025-05-15: parsed 2 divisions (outcome-only)
2026/05/03 16:34:07 [nl-votes] 2025-05-20: parsed 0 divisions (outcome-only)
2026/05/03 16:34:08 [bc-votes] 2026-04-28: parsed 2 divisions
2026/05/03 16:34:08 [nl-votes] 2025-05-22: parsed 0 divisions (outcome-only)
2026/05/03 16:34:08 [nl-votes] parsed 74 divisions from 80 PDFs
2026/05/03 16:34:08 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-207
2026/05/03 16:34:08 [summarizer] skip unchanged bill "nl-1-1-1"
2026/05/03 16:34:08 [summarizer] skip unchanged bill "nb-61-2-23"
2026/05/03 16:34:08 [summarizer] skip unchanged bill "45-1-s-206"
2026/05/03 16:34:08 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-12-03/votes-proceedings
2026/05/03 16:34:08 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-12-04/votes-proceedings
2026/05/03 16:34:08 [summarizer] skip unchanged bill "sk-30-2-34"
2026/05/03 16:34:08 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-12-08/votes-proceedings
2026/05/03 16:34:08 [ontario-votes] 2025-12-03: parsed 1 divisions
2026/05/03 16:34:08 [ontario-votes] 2025-12-04: parsed 0 divisions
2026/05/03 16:34:08 [ontario-votes] 2025-12-08: parsed 3 divisions
2026/05/03 16:34:08 [bc-votes] parsed 30 divisions from 32 files
2026/05/03 16:34:08 [summarizer] skip unchanged bill "bc-43-2-1"
2026/05/03 16:34:08 [summarizer] skip unchanged bill "nl-1-1-10"
2026/05/03 16:34:08 [summarizer] skip unchanged bill "nb-61-2-24"
2026/05/03 16:34:08 [summarizer] skip unchanged bill "sk-30-2-35"
2026/05/03 16:34:08 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-12-09/votes-proceedings
2026/05/03 16:34:08 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-12-10/votes-proceedings
2026/05/03 16:34:08 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-208
2026/05/03 16:34:08 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2025-12-11/votes-proceedings
2026/05/03 16:34:08 [ontario-votes] 2025-12-09: parsed 1 divisions
2026/05/03 16:34:08 [ontario-votes] 2025-12-10: parsed 1 divisions
2026/05/03 16:34:08 [ontario-votes] 2025-12-11: parsed 3 divisions
2026/05/03 16:34:08 [calendar] detected 11 dates for provincial-NS
2026/05/03 16:34:08 [summarizer] skip unchanged bill "45-1-s-207"
2026/05/03 16:34:09 [calendar] detected 76 dates for provincial-ON
2026/05/03 16:34:09 [summarizer] skip unchanged bill "bc-43-2-2"
2026/05/03 16:34:09 [ab-votes] parsed 1 divisions
2026/05/03 16:34:09 [summarizer] skip unchanged bill "nl-1-1-11"
2026/05/03 16:34:09 [summarizer] skip unchanged bill "nb-61-2-25"
2026/05/03 16:34:09 [ab-votes] parsed 1 divisions
2026/05/03 16:34:09 [summarizer] skip unchanged bill "sk-30-2-36"
2026/05/03 16:34:09 [ab-votes] parsed 1 divisions
2026/05/03 16:34:09 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-03-23/votes-proceedings
2026/05/03 16:34:09 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-03-24/votes-proceedings
2026/05/03 16:34:09 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-03-25/votes-proceedings
2026/05/03 16:34:09 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-209
2026/05/03 16:34:09 [ontario-votes] 2026-03-24: parsed 0 divisions
2026/05/03 16:34:09 [ontario-votes] 2026-03-23: parsed 0 divisions
2026/05/03 16:34:09 [summarizer] skip unchanged bill "bc-43-2-3"
2026/05/03 16:34:09 [ontario-votes] 2026-03-25: parsed 1 divisions
2026/05/03 16:34:09 [summarizer] skip unchanged bill "45-1-s-208"
2026/05/03 16:34:09 [summarizer] skip unchanged bill "nb-61-2-26"
2026/05/03 16:34:09 [summarizer] skip unchanged bill "sk-30-2-37"
2026/05/03 16:34:09 [summarizer] skip unchanged bill "nl-1-1-12"
2026/05/03 16:34:10 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-210
2026/05/03 16:34:10 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-03-26/votes-proceedings
2026/05/03 16:34:10 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-03-30/votes-proceedings
2026/05/03 16:34:10 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-03-31/votes-proceedings
2026/05/03 16:34:10 [summarizer] skip unchanged bill "bc-43-2-4"
2026/05/03 16:34:10 [ontario-votes] 2026-03-26: parsed 1 divisions
2026/05/03 16:34:10 [ontario-votes] 2026-03-30: parsed 0 divisions
2026/05/03 16:34:10 [ontario-votes] 2026-03-31: parsed 1 divisions
2026/05/03 16:34:10 [summarizer] skip unchanged bill "45-1-s-209"
2026/05/03 16:34:10 [summarizer] skip unchanged bill "nb-61-2-27"
2026/05/03 16:34:10 [summarizer] skip unchanged bill "nl-1-1-13"
2026/05/03 16:34:10 [summarizer] skip unchanged bill "sk-30-2-38"
2026/05/03 16:34:10 [ab-votes] parsed 1 divisions
2026/05/03 16:34:10 [summarizer] skip unchanged bill "bc-43-2-5"
2026/05/03 16:34:10 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-01/votes-proceedings
2026/05/03 16:34:10 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-02/votes-proceedings
2026/05/03 16:34:10 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-211
2026/05/03 16:34:10 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-13/votes-proceedings
2026/05/03 16:34:10 [ontario-votes] 2026-04-01: parsed 0 divisions
2026/05/03 16:34:10 [ontario-votes] 2026-04-02: parsed 3 divisions
2026/05/03 16:34:10 [ontario-votes] 2026-04-13: parsed 1 divisions
2026/05/03 16:34:10 [summarizer] skip unchanged bill "nl-1-1-14"
2026/05/03 16:34:10 [summarizer] skip unchanged bill "45-1-s-210"
2026/05/03 16:34:10 [summarizer] skip unchanged bill "nb-61-2-28"
2026/05/03 16:34:10 [summarizer] skip unchanged bill "sk-30-2-39"
2026/05/03 16:34:10 [ab-votes] parsed 29 divisions from 51 PDFs
2026/05/03 16:34:11 [summarizer] skip unchanged bill "ab-31-2-1"
2026/05/03 16:34:11 [summarizer] skip unchanged bill "bc-43-2-6"
2026/05/03 16:34:11 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-14/votes-proceedings
2026/05/03 16:34:11 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-15/votes-proceedings
2026/05/03 16:34:11 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-212
2026/05/03 16:34:11 [summarizer] skip unchanged bill "nl-1-1-2"
2026/05/03 16:34:11 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-16/votes-proceedings
2026/05/03 16:34:11 [summarizer] skip unchanged bill "nb-61-2-29"
2026/05/03 16:34:11 [ontario-votes] 2026-04-14: parsed 2 divisions
2026/05/03 16:34:11 [ontario-votes] 2026-04-15: parsed 2 divisions
2026/05/03 16:34:11 [summarizer] skip unchanged bill "sk-30-2-40"
2026/05/03 16:34:11 [ontario-votes] 2026-04-16: parsed 3 divisions
2026/05/03 16:34:11 [summarizer] skip unchanged bill "45-1-s-211"
2026/05/03 16:34:11 [mb-votes] skip pdf https://www.gov.mb.ca/legislature/business/43rd/3rd/votes_013.pdf: Read: xRefTable failed: pdfcpu: no header version available
2026/05/03 16:34:11 [summarizer] skip unchanged bill "bc-43-2-7"
2026/05/03 16:34:11 [summarizer] skip unchanged bill "ab-31-2-2"
2026/05/03 16:34:11 [summarizer] skip unchanged bill "nb-61-2-30"
2026/05/03 16:34:11 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-20/votes-proceedings
2026/05/03 16:34:11 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-21/votes-proceedings
2026/05/03 16:34:11 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-22/votes-proceedings
2026/05/03 16:34:11 [summarizer] skip unchanged bill "sk-30-2-41"
2026/05/03 16:34:11 [ontario-votes] 2026-04-20: parsed 2 divisions
2026/05/03 16:34:11 [ontario-votes] 2026-04-21: parsed 1 divisions
2026/05/03 16:34:11 [summarizer] skip unchanged bill "nl-1-1-3"
2026/05/03 16:34:11 [ontario-votes] 2026-04-22: parsed 0 divisions
2026/05/03 16:34:12 [summarizer] skip unchanged bill "bc-43-2-8"
2026/05/03 16:34:12 [summarizer] skip unchanged bill "nb-61-2-31"
2026/05/03 16:34:12 [summarizer] skip unchanged bill "nl-1-1-4"
2026/05/03 16:34:12 [summarizer] skip unchanged bill "sk-30-2-42"
2026/05/03 16:34:12 [ontario-votes] scraping V&P: https://www.ola.org/en/legislative-business/house-documents/parliament-44/session-1/2026-04-23/votes-proceedings
2026/05/03 16:34:12 [ontario-votes] 2026-04-23: parsed 3 divisions
2026/05/03 16:34:12 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-213
2026/05/03 16:34:12 [calendar] detected 19 dates for provincial-PE
2026/05/03 16:34:12 [summarizer] skip unchanged bill "bc-43-2-9"
2026/05/03 16:34:12 [summarizer] skip unchanged bill "45-1-s-212"
2026/05/03 16:34:12 [summarizer] skip unchanged bill "nb-61-2-32"
2026/05/03 16:34:12 [pe-bills] page 2: 15 rows
2026/05/03 16:34:12 [summarizer] skip unchanged bill "nl-1-1-5"
2026/05/03 16:34:12 [summarizer] skip unchanged bill "sk-30-2-43"
2026/05/03 16:34:12 [summarizer] skip unchanged bill "on-44-1-1"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "bc-43-2-10"
2026/05/03 16:34:13 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-214
2026/05/03 16:34:13 [summarizer] skip unchanged bill "45-1-s-213"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "nl-1-1-6"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "nb-61-2-33"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "sk-30-2-44"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "on-44-1-10"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "bc-43-2-11"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "ab-31-2-4"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "nb-61-2-61"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "nl-1-1-7"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "sk-30-2-45"
2026/05/03 16:34:13 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-215
2026/05/03 16:34:13 [summarizer] skip unchanged bill "on-44-1-100"
2026/05/03 16:34:13 [summarizer] skip unchanged bill "45-1-s-214"
2026/05/03 16:34:14 [summarizer] skip unchanged bill "bc-43-2-12"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Allain"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Holder"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Crossman"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Shephard"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Holland"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Speaker"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "ms. On"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "ms. Concerning"
2026/05/03 16:34:14 [summarizer] skip unchanged bill "nl-1-1-8"
2026/05/03 16:34:14 [summarizer] skip unchanged bill "sk-30-2-46"
2026/05/03 16:34:14 [summarizer] skip unchanged bill "ab-31-2-5"
2026/05/03 16:34:14 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-216
2026/05/03 16:34:14 [summarizer] skip unchanged bill "on-44-1-101"
2026/05/03 16:34:14 [summarizer] skip unchanged bill "bc-43-2-13"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Ho"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Holder"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Higgs"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Crossman"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Shephard"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Anderson-Mason"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Holland"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:14 [provincial][nb] unmatched vote name: "Speaker"
2026/05/03 16:34:14 [summarizer] skip unchanged bill "45-1-s-215"
2026/05/03 16:34:14 [summarizer] skip unchanged bill "nl-1-1-9"
2026/05/03 16:34:14 [summarizer] skip unchanged bill "sk-30-2-47"
2026/05/03 16:34:14 [summarizer] skip unchanged bill "on-44-1-102"
2026/05/03 16:34:15 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-217
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Allain"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Higgs"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Shephard"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Green"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Anderson-Mason"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Holland"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Th"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Cardy"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Speaker"
2026/05/03 16:34:15 [summarizer] skip unchanged bill "ab-31-2-6"
2026/05/03 16:34:15 [summarizer] skip unchanged bill "bc-43-2-14"
2026/05/03 16:34:15 [summarizer] skip unchanged bill "45-1-s-216"
2026/05/03 16:34:15 [summarizer] skip unchanged bill "sk-30-2-48"
2026/05/03 16:34:15 [summarizer] skip unchanged bill "on-44-1-103"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Allain"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Higgs"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Shephard"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Green"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Anderson-Mason"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Holland"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Th"
2026/05/03 16:34:15 [provincial][nb] unmatched vote name: "Cardy"
2026/05/03 16:34:15 [summarizer] skip unchanged bill "ab-31-2-7"
2026/05/03 16:34:15 [summarizer] skip unchanged bill "bc-43-2-15"
2026/05/03 16:34:15 [summarizer] skip unchanged bill "sk-30-2-49"
2026/05/03 16:34:15 [summarizer] skip unchanged bill "on-44-1-104"
2026/05/03 16:34:15 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-218
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Th"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Allain"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Higgs"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Shephard"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Green"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Anderson-Mason"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Holland"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Th"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Allain"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Green"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Shephard"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Anderson-Mason"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Holland"
2026/05/03 16:34:16 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:16 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-219
2026/05/03 16:34:17 [ns-votes] parsed 11 divisions from 45 html day pages
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Allain"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Shephard"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Green"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Holland"
2026/05/03 16:34:17 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-220
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Allain"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Shephard"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Green"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Holland"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:17 [provincial][nb] unmatched vote name: "Mr."
2026/05/03 16:34:18 [calendar] detected 135 dates for provincial-SK
2026/05/03 16:34:18 [calendar] legislature schedule crawl warning: Get "https://www.assnat.qc.ca/en/document/211091.html": EOF
2026/05/03 16:34:18 [summarizer] skip unchanged bill "ab-31-2-3"
2026/05/03 16:34:18 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/S-221
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Green"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Allain"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Shephard"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Holland"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:18 [summarizer] skip unchanged bill "bc-43-2-16"
2026/05/03 16:34:18 [summarizer] skip unchanged bill "45-1-s-217"
2026/05/03 16:34:18 [summarizer] skip unchanged bill "sk-30-2-50"
2026/05/03 16:34:18 [summarizer] skip unchanged bill "on-44-1-105"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Losier"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Th"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Allain"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Higgs"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Wetmore"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Steeves"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Dawson"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Green"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Flemming"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Carr"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Fitch"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Turner"
2026/05/03 16:34:18 [provincial][nb] unmatched vote name: "Holland"

2026/05/03 16:35:23 [provincial][on] unmatched vote name: "Wong-Tam"
2026/05/03 16:35:23 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-250
2026/05/03 16:35:23 [summarizer] skip unchanged bill "ns-65-1-220"
2026/05/03 16:35:23 [summarizer] skip unchanged bill "45-1-c-249"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Begum"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Wong-Tam"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Jones (Chatham-Kent—Leamington)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Jones (Dufferin—Caledon)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:24 [summarizer] skip unchanged bill "ns-65-1-221"
2026/05/03 16:35:24 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-251
2026/05/03 16:35:24 [summarizer] skip unchanged bill "45-1-c-250"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Begum"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Wong-Tam"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Jones (Chatham-Kent—Leamington)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:24 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:24 [summarizer] skip unchanged bill "ns-65-1-222"
2026/05/03 16:35:25 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-252
2026/05/03 16:35:25 [summarizer] skip unchanged bill "45-1-c-251"
2026/05/03 16:35:25 [summarizer] skip unchanged bill "ns-65-1-223"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Jones (Chatham-Kent—Leamington)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Jones (Dufferin—Caledon)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Wong-Tam"
2026/05/03 16:35:25 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-253
2026/05/03 16:35:25 [summarizer] skip unchanged bill "ns-65-1-224"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:25 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:25 [summarizer] skip unchanged bill "45-1-c-252"
2026/05/03 16:35:26 [summarizer] skip unchanged bill "ns-65-1-225"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:26 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-254
2026/05/03 16:35:26 [summarizer] skip unchanged bill "45-1-c-253"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:26 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:26 [summarizer] skip unchanged bill "ns-65-1-226"
2026/05/03 16:35:26 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-255
2026/05/03 16:35:27 [summarizer] skip unchanged bill "45-1-c-254"
2026/05/03 16:35:27 [summarizer] skip unchanged bill "ns-65-1-227"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:27 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-256
2026/05/03 16:35:27 [summarizer] skip unchanged bill "45-1-c-255"
2026/05/03 16:35:27 [summarizer] skip unchanged bill "ns-65-1-228"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:27 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:28 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-257
2026/05/03 16:35:28 [summarizer] skip unchanged bill "45-1-c-256"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:28 [summarizer] skip unchanged bill "ns-65-1-229"
2026/05/03 16:35:28 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-258
2026/05/03 16:35:28 [summarizer] skip unchanged bill "45-1-c-257"
2026/05/03 16:35:28 [summarizer] skip unchanged bill "ns-65-1-23"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Jones (Chatham-Kent—Leamington)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Jones (Dufferin—Caledon)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:28 [provincial][on] unmatched vote name: "Begum"
2026/05/03 16:35:29 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-259
2026/05/03 16:35:29 [summarizer] skip unchanged bill "45-1-c-258"
2026/05/03 16:35:29 [summarizer] skip unchanged bill "ns-65-1-230"
2026/05/03 16:35:29 [provincial][on] unmatched vote name: "Begum"
2026/05/03 16:35:29 [provincial][on] unmatched vote name: "Cho (Scarborough North)"
2026/05/03 16:35:29 [provincial][on] unmatched vote name: "Jones (Chatham-Kent—Leamington)"
2026/05/03 16:35:29 [provincial][on] unmatched vote name: "Jones (Dufferin—Caledon)"
2026/05/03 16:35:29 [provincial][on] unmatched vote name: "Scott (Haliburton—Kawartha Lakes—Brock)"
2026/05/03 16:35:29 [provincial][on] unmatched vote name: "Smith (Parry Sound—Muskoka)"
2026/05/03 16:35:29 [provincial][on] unmatched vote name: "Smith (Peterborough—Kawartha)"
2026/05/03 16:35:29 [provincial][on] unmatched vote name: "Smith (Scarborough Centre)"
2026/05/03 16:35:29 [provincial][on] unmatched vote name: "Smith (Thornhill)"
2026/05/03 16:35:29 [bills] scraping detail: https://www.parl.ca/legisinfo/en/bill/45-1/C-260
2026/05/03 16:35:29 [summarizer] skip unchanged bill "ns-65-1-231"
2026/05/03 16:35:29 [summarizer] skip unchanged bill "45-1-c-259"

