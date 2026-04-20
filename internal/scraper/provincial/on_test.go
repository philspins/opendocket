package provincial

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCrawlOntarioVPSittingDates_ParsesVotesProceedingsLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body>
		<a href="/en/legislative-business/house-documents/parliament-44/session-1/2025-04-16/votes-proceedings">Votes and Proceedings</a>
		<a href="/en/legislative-business/house-documents/parliament-44/session-1/2025-04-15/votes-proceedings">Votes and Proceedings</a>
	</body></html>`))
	}))
	defer srv.Close()

	dates, err := CrawlOntarioVPSittingDates(srv.URL, 44, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlOntarioVPSittingDates: %v", err)
	}
	if len(dates) != 2 {
		t.Fatalf("len(dates)=%d, want 2", len(dates))
	}
	if dates[0] != "2025-04-15" || dates[1] != "2025-04-16" {
		t.Fatalf("dates=%v, want [2025-04-15 2025-04-16]", dates)
	}
}

func TestCrawlOntarioVPSittingDates_ParsesHansardLinksAndIgnoresOrdersNotices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body>
		<a href="/en/legislative-business/house-documents/parliament-44/session-1/2025-04-16/orders-notices">Orders and Notices</a>
		<a href="/en/legislative-business/house-documents/parliament-44/session-1/2025-04-15/hansard">Hansard</a>
		<a href="/en/legislative-business/house-documents/parliament-44/session-1/2025-04-15/hansard">Hansard duplicate</a>
	</body></html>`))
	}))
	defer srv.Close()

	dates, err := CrawlOntarioVPSittingDates(srv.URL, 44, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlOntarioVPSittingDates: %v", err)
	}
	if len(dates) != 1 {
		t.Fatalf("len(dates)=%d, want 1", len(dates))
	}
	if dates[0] != "2025-04-15" {
		t.Fatalf("dates=%v, want [2025-04-15]", dates)
	}
}

func TestCrawlOntarioVPDay_ParsesCurrentMarkupAndSkipsNonDivisionDataWrappers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body>
	<table><tbody><tr><td colspan="2"><div class="datawrapper">Yesterday, the Leader of the Official Opposition filed a notice of motion.</div></td></tr></tbody></table>
	<table><tbody><tr>
		<td class="votesProceedingsDoc2col" lang="en"><p class="no-indent">Second Reading of <strong>Bill 23</strong>, An Act to amend the Residential Tenancies Act, 2006 and the Retirement Homes Act, 2010 respecting tenancies in care homes.</p></td>
		<td class="votesProceedingsDoc2col" lang="fr"><p class="no-indent">Deuxieme lecture du projet de loi 23.</p></td>
	</tr></tbody></table>
	<table><tbody><tr>
		<td class="votesProceedingsDoc2col" lang="en"><p class="no-indent">Lost on the following division:</p></td>
		<td class="votesProceedingsDoc2col" lang="fr"><p class="no-indent">Rejetee par le vote suivant :</p></td>
	</tr></tbody></table>
	<table><tbody><tr><td colspan="2">
		<div class="datawrapper">
			<h5 class="divisionHeader"><span lang="en">Ayes</span><span class="sl-hide">/</span><span lang="fr">pour</span> (2)</h5>
			<table class="votesList"><tbody><tr>
				<td><div lang="en">Armstrong</div><div class="docHide" lang="fr">Armstrong</div></td>
				<td><div lang="en">Jones</div><div class="docHide" lang="fr">Jones</div></td>
			</tr></tbody></table>
			<h5 class="divisionHeader"><span lang="en">Nays</span><span class="sl-hide">/</span><span lang="fr">contre</span> (3)</h5>
			<table class="votesList"><tbody><tr>
				<td><div lang="en">Brown</div><div class="docHide" lang="fr">Brown</div></td>
				<td><div lang="en">Taylor</div><div class="docHide" lang="fr">Taylor</div></td>
				<td><div lang="en">Wilson</div><div class="docHide" lang="fr">Wilson</div></td>
			</tr></tbody></table>
		</div>
	</td></tr></tbody></table>
	</body></html>`))
	}))
	defer srv.Close()

	divs, err := CrawlOntarioVPDay(srv.URL, 44, 1, "2026-04-16", srv.Client())
	if err != nil {
		t.Fatalf("CrawlOntarioVPDay: %v", err)
	}
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if divs[0].Division.ID != "on-44-1-2026-04-16-1" {
		t.Fatalf("division ID=%q, want on-44-1-2026-04-16-1", divs[0].Division.ID)
	}
	if divs[0].Division.Description != "Second Reading of Bill 23, An Act to amend the Residential Tenancies Act, 2006 and the Retirement Homes Act, 2010 respecting tenancies in care homes." {
		t.Fatalf("description=%q", divs[0].Division.Description)
	}
	if divs[0].Division.Yeas != 2 || divs[0].Division.Nays != 3 {
		t.Fatalf("counts=(%d,%d), want (2,3)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
	if divs[0].Division.Result != "Negatived" {
		t.Fatalf("result=%q, want Negatived", divs[0].Division.Result)
	}
	if len(divs[0].Votes) != 5 {
		t.Fatalf("len(votes)=%d, want 5", len(divs[0].Votes))
	}
}
