package scraper_test

import (
	"testing"

	"github.com/philspins/opendocket/internal/scraper"
)

// ── CrawlSenateVotesIndex ──────────────────────────────────────────────────────

// sampleSenateVotesHTML mirrors the actual sencanada.ca votes table structure:
// Col 0: date (ISO), data-order ends with sequential vote number
// Col 1: description link + "Yeas: N | Nays: N | Abstentions: N | Total: N"
// Col 2: bill number (optional link)
// Col 3: result ("Defeated" / "Adopted" / "Agreed to")
const sampleSenateVotesHTML = `<html><body>
  <table>
    <thead><tr>
      <th>Date</th><th>Description</th><th>Bill</th><th>Result</th>
    </tr></thead>
    <tbody>
      <tr>
        <td class="vote-centered" data-order="2024-04-04 13:30:00 42">
          <a href="/en/content/sen/chamber/451/journals/j-e">2024-04-04</a>
        </td>
        <td>
          <a class="vote-web-title-link" href="/en/in-the-chamber/votes/details/12345/45-1">Motion on S-209</a>
          <br />
          Yeas: 58 | Nays: 22 | Abstentions: 2 | Total: 82
        </td>
        <td class="vote-centered">
          <a href="http://www.parl.ca/LEGISInfo/BillDetails.aspx?Language=en&amp;billId=999">S-209</a>
        </td>
        <td class="vote-centered">
          Agreed to
        </td>
      </tr>
      <tr>
        <td class="vote-centered" data-order="2024-04-03 13:30:00 41">
          <a href="/en/content/sen/chamber/451/journals/j2-e">2024-04-03</a>
        </td>
        <td>
          <a class="vote-web-title-link" href="/en/in-the-chamber/votes/details/12344/45-1">Third reading of S-5</a>
          <br />
          Yeas: 50 | Nays: 30 | Abstentions: 0 | Total: 80
        </td>
        <td class="vote-centered">
          <a href="http://www.parl.ca/LEGISInfo/BillDetails.aspx?Language=en&amp;billId=888">S-5</a>
        </td>
        <td class="vote-centered">
          Adopted
        </td>
      </tr>
    </tbody>
  </table>
</body></html>`

func TestCrawlSenateVotesIndex_ParsesRows(t *testing.T) {
	srv := newTestServer(sampleSenateVotesHTML)
	defer srv.Close()

	divs, err := scraper.CrawlSenateVotesIndex(srv.URL, 45, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlSenateVotesIndex: %v", err)
	}
	if len(divs) != 2 {
		t.Errorf("len=%d, want 2", len(divs))
	}
}

func TestCrawlSenateVotesIndex_ParsesFirstDivision(t *testing.T) {
	srv := newTestServer(sampleSenateVotesHTML)
	defer srv.Close()

	divs, _ := scraper.CrawlSenateVotesIndex(srv.URL, 45, 1, srv.Client())
	d := divs[0]

	if d.ID != "senate-45-1-42" {
		t.Errorf("ID=%q want senate-45-1-42", d.ID)
	}
	if d.Number != 42 {
		t.Errorf("Number=%d want 42", d.Number)
	}
	if d.Yeas != 58 || d.Nays != 22 {
		t.Errorf("Yeas=%d Nays=%d want 58/22", d.Yeas, d.Nays)
	}
	if d.Result != "Agreed to" {
		t.Errorf("Result=%q want Agreed to", d.Result)
	}
	if d.Date != "2024-04-04" {
		t.Errorf("Date=%q want 2024-04-04", d.Date)
	}
}

func TestCrawlSenateVotesIndex_ExtractsBillNumber(t *testing.T) {
	srv := newTestServer(sampleSenateVotesHTML)
	defer srv.Close()

	divs, _ := scraper.CrawlSenateVotesIndex(srv.URL, 45, 1, srv.Client())
	// First row: col 2 link text "S-209"
	if divs[0].BillNumber != "S-209" {
		t.Errorf("divs[0].BillNumber=%q want S-209", divs[0].BillNumber)
	}
	// Second row: col 2 link text "S-5"
	if divs[1].BillNumber != "S-5" {
		t.Errorf("divs[1].BillNumber=%q want S-5", divs[1].BillNumber)
	}
}

func TestCrawlSenateVotesIndex_ChamberIsSenate(t *testing.T) {
	srv := newTestServer(sampleSenateVotesHTML)
	defer srv.Close()

	divs, _ := scraper.CrawlSenateVotesIndex(srv.URL, 45, 1, srv.Client())
	for _, d := range divs {
		if d.Chamber != "senate" {
			t.Errorf("Chamber=%q want senate for division %s", d.Chamber, d.ID)
		}
	}
}

func TestCrawlSenateVotesIndex_IDHasSenatePrefix(t *testing.T) {
	srv := newTestServer(sampleSenateVotesHTML)
	defer srv.Close()

	divs, _ := scraper.CrawlSenateVotesIndex(srv.URL, 45, 1, srv.Client())
	for _, d := range divs {
		if len(d.ID) < 7 || d.ID[:7] != "senate-" {
			t.Errorf("ID=%q does not start with senate-", d.ID)
		}
	}
}

func TestCrawlSenateVotesIndex_BuildsDetailURL(t *testing.T) {
	srv := newTestServer(sampleSenateVotesHTML)
	defer srv.Close()

	divs, _ := scraper.CrawlSenateVotesIndex(srv.URL, 45, 1, srv.Client())
	// Relative href "/en/in-the-chamber/votes/42" should become an absolute URL
	if divs[0].DetailURL == "" {
		t.Error("DetailURL should not be empty")
	}
}

func TestCrawlSenateVotesIndex_ErrorOnBadServer(t *testing.T) {
	_, err := scraper.CrawlSenateVotesIndex("http://localhost:0/no-server", 45, 1, nil)
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestCrawlSenateVotesIndex_ErrorWhenNoTable(t *testing.T) {
	srv := newTestServer("<html><body><p>No table</p></body></html>")
	defer srv.Close()

	_, err := scraper.CrawlSenateVotesIndex(srv.URL, 45, 1, srv.Client())
	if err == nil {
		t.Error("expected error when no table found")
	}
}

func TestCrawlSenateVotesIndex_SkipsRowsWithNoNumber(t *testing.T) {
	// A row with 4 columns but no valid sequential vote number in data-order
	html := `<html><body><table><tbody>
      <tr>
        <td data-order="not-a-valid-order"><a href="#">2024-04-03</a></td>
        <td><a class="vote-web-title-link" href="/en/votes/1">Some motion</a><br/>Yeas: 10 | Nays: 5</td>
        <td></td>
        <td>Adopted</td>
      </tr>
    </tbody></table></body></html>`
	srv := newTestServer(html)
	defer srv.Close()

	divs, err := scraper.CrawlSenateVotesIndex(srv.URL, 45, 1, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(divs) != 0 {
		t.Errorf("expected 0 rows skipped, got %d", len(divs))
	}
}

// ── CrawlSenateDivisionDetail ─────────────────────────────────────────────────

const sampleSenateDivisionHTML = `<html><body>
  <div class="vote-yea">
    <ul>
      <li><a href="/Members/en/111">Senator Alice</a></li>
      <li><a href="/Members/en/222">Senator Bob</a></li>
    </ul>
  </div>
  <div class="vote-nay">
    <ul>
      <li><a href="/Members/en/333">Senator Carol</a></li>
    </ul>
  </div>
  <div class="vote-abstain">
    <ul>
      <li><a href="/Members/en/444">Senator Dave</a></li>
    </ul>
  </div>
</body></html>`

const sampleSenateDivisionModernHTML = `<html><body>
	<section class="container sc-vote-details votes-module">
		<div class="table-responsive">
			<table class="table sc-table" id="sc-vote-details-table">
				<thead>
					<tr>
						<th class="vote-senator">Senator</th>
						<th class="vote-affiliation">Affiliation</th>
						<th class="vote-province">Province/Territory</th>
						<th class="vote-yea min-desktop">Yea</th>
						<th class="vote-nay min-desktop">Nay</th>
						<th class="vote-abstention min-desktop">Abstention</th>
					</tr>
				</thead>
				<tbody>
					<tr>
						<td data-order="Ringuette, Pierrette"><a href="/en/in-the-chamber/votes/senator/2753/45-1">Ringuette, Pierrette</a></td>
						<td>ISG</td><td>New Brunswick</td>
						<td data-order="aaa"><i class="fa-solid fa-times"></i></td>
						<td data-order="zzz"></td>
						<td data-order="zzz"></td>
					</tr>
					<tr>
						<td data-order="Anderson, Dawn"><a href="/en/in-the-chamber/votes/senator/3001/45-1">Anderson, Dawn</a></td>
						<td>PSG</td><td>Northwest Territories</td>
						<td data-order="zzz"></td>
						<td data-order="aaa"><i class="fa-solid fa-times"></i></td>
						<td data-order="zzz"></td>
					</tr>
					<tr>
						<td data-order="Wallin, Pamela"><a href="/en/in-the-chamber/votes/senator/4123/45-1">Wallin, Pamela</a></td>
						<td>C</td><td>Saskatchewan</td>
						<td data-order="zzz"></td>
						<td data-order="zzz"></td>
						<td data-order="aaa"><i class="fa-solid fa-times"></i></td>
					</tr>
					<tr>
						<td data-order="No Vote, Senator"><a href="/en/in-the-chamber/votes/senator/4999/45-1">No Vote, Senator</a></td>
						<td>C</td><td>Ontario</td>
						<td data-order="zzz"></td>
						<td data-order="zzz"></td>
						<td data-order="zzz"></td>
					</tr>
				</tbody>
			</table>
		</div>
	</section>
</body></html>`

func TestCrawlSenateDivisionDetail_ParsesYeaVotes(t *testing.T) {
	srv := newTestServer(sampleSenateDivisionHTML)
	defer srv.Close()

	votes, err := scraper.CrawlSenateDivisionDetail("senate-45-1-42", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("CrawlSenateDivisionDetail: %v", err)
	}
	var yeas []scraper.MemberVote
	for _, v := range votes {
		if v.Vote == "Yea" {
			yeas = append(yeas, v)
		}
	}
	if len(yeas) != 2 {
		t.Errorf("len(yeas)=%d want 2", len(yeas))
	}
}

func TestCrawlSenateDivisionDetail_ParsesNayVotes(t *testing.T) {
	srv := newTestServer(sampleSenateDivisionHTML)
	defer srv.Close()

	votes, _ := scraper.CrawlSenateDivisionDetail("senate-45-1-42", srv.URL, srv.Client())
	var nays []scraper.MemberVote
	for _, v := range votes {
		if v.Vote == "Nay" {
			nays = append(nays, v)
		}
	}
	if len(nays) != 1 || nays[0].MemberID != "333" {
		t.Errorf("nay votes mismatch: %+v", nays)
	}
}

func TestCrawlSenateDivisionDetail_ParsesAbstainVotes(t *testing.T) {
	srv := newTestServer(sampleSenateDivisionHTML)
	defer srv.Close()

	votes, _ := scraper.CrawlSenateDivisionDetail("senate-45-1-42", srv.URL, srv.Client())
	var abstains []scraper.MemberVote
	for _, v := range votes {
		if v.Vote == "Abstain" {
			abstains = append(abstains, v)
		}
	}
	if len(abstains) != 1 || abstains[0].MemberID != "444" {
		t.Errorf("abstain votes mismatch: %+v", abstains)
	}
}

func TestCrawlSenateDivisionDetail_ParsesModernTable(t *testing.T) {
	srv := newTestServer(sampleSenateDivisionModernHTML)
	defer srv.Close()

	votes, err := scraper.CrawlSenateDivisionDetail("senate-45-1-1", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("CrawlSenateDivisionDetail: %v", err)
	}
	if len(votes) != 3 {
		t.Fatalf("len(votes)=%d, want 3", len(votes))
	}

	got := map[string]string{}
	for _, v := range votes {
		got[v.MemberID] = v.Vote
	}
	if got["2753"] != "Yea" {
		t.Errorf("member 2753 vote=%q, want Yea", got["2753"])
	}
	if got["3001"] != "Nay" {
		t.Errorf("member 3001 vote=%q, want Nay", got["3001"])
	}
	if got["4123"] != "Abstain" {
		t.Errorf("member 4123 vote=%q, want Abstain", got["4123"])
	}
	if _, exists := got["4999"]; exists {
		t.Errorf("member 4999 should not be present; got vote=%q", got["4999"])
	}
}

func TestCrawlSenateDivisionDetail_AllHaveDivisionID(t *testing.T) {
	srv := newTestServer(sampleSenateDivisionHTML)
	defer srv.Close()

	votes, _ := scraper.CrawlSenateDivisionDetail("senate-45-1-42", srv.URL, srv.Client())
	for _, v := range votes {
		if v.DivisionID != "senate-45-1-42" {
			t.Errorf("DivisionID=%q want senate-45-1-42", v.DivisionID)
		}
	}
}

func TestCrawlSenateDivisionDetail_ErrorOnBadURL(t *testing.T) {
	_, err := scraper.CrawlSenateDivisionDetail("senate-45-1-99", "http://localhost:0/no-server", nil)
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestCrawlSenateDivisionDetail_EmptyWhenNoMembers(t *testing.T) {
	srv := newTestServer("<html><body><p>No votes here</p></body></html>")
	defer srv.Close()

	votes, err := scraper.CrawlSenateDivisionDetail("senate-45-1-42", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(votes) != 0 {
		t.Errorf("expected 0 votes, got %d", len(votes))
	}
}
