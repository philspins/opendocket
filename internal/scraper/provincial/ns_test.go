package provincial

import (
	"testing"
)

func TestExtractLegislatureSessionCandidates_NovaScotiaHansardURL(t *testing.T) {
	candidates := extractLegislatureSessionCandidates("ns", "https://nslegislature.ca/legislative-business/hansard-debates/assembly-65-session-1", 50)
	best, ok := maxLegislatureSession(candidates)
	if !ok {
		t.Fatal("no candidates for Nova Scotia Hansard session URL")
	}
	if best.Legislature != 65 || best.Session != 1 {
		t.Fatalf("best=%+v, want legislature=65 session=1", best)
	}
}

func TestNovaScotiaHansardSessionURL(t *testing.T) {
	got := novaScotiaHansardSessionURL("https://nslegislature.ca/legislative-business/journals", 64, 1)
	want := "https://nslegislature.ca/legislative-business/hansard-debates/assembly-64-session-1"
	if got != want {
		t.Fatalf("novaScotiaHansardSessionURL()=%q, want %q", got, want)
	}

	if got := novaScotiaHansardSessionURL("https://nslegislature.ca/legislative-business/journals", 1, 1); got != "https://nslegislature.ca/legislative-business/journals" {
		t.Fatalf("novaScotiaHansardSessionURL(unresolved)=%q, want journals URL", got)
	}

	alreadySession := "https://nslegislature.ca/legislative-business/hansard-debates/assembly-65-session-1"
	if got := novaScotiaHansardSessionURL(alreadySession, 65, 1); got != alreadySession {
		t.Fatalf("novaScotiaHansardSessionURL(sessionURL)=%q, want %q", got, alreadySession)
	}
}

func TestNSDateFromHansardURL(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://nslegislature.ca/.../house_26apr08", "2026-04-08"},
		{"https://nslegislature.ca/.../house_26apr09", "2026-04-09"},
		{"https://nslegislature.ca/.../house_25mar03", "2025-03-03"},
		{"https://nslegislature.ca/.../house_25nov21", "2025-11-21"},
		{"https://nslegislature.ca/.../house_26jan01", "2026-01-01"},
	}
	for _, c := range cases {
		if got := nsDateFromHansardURL(c.url); got != c.want {
			t.Errorf("nsDateFromHansardURL(%q)=%q want %q", c.url, got, c.want)
		}
	}
}

func TestParseNSHansardHTMLPage_VoteTable(t *testing.T) {
	const hansardHTML = `<html><body>
<p>There has been a request for a recorded vote.</p>
<p>[The Clerk called the roll.]</p>
<table>
  <thead><tr><th>YEAS</th><th>NAYS</th></tr></thead>
  <tbody>
    <tr><td>Hon. Alice Smith</td><td>Bob Jones</td></tr>
    <tr><td>Hon. Carol Brown</td><td>Dave White</td></tr>
    <tr><td>Eve Taylor</td><td></td></tr>
  </tbody>
</table>
<p><b>Bill No. 42 - An Act to Do Something.</b></p>
<p>THE CLERK: For, 3. Against, 2</p>
</body></html>`

	doc := mustDocFromHTML(t, hansardHTML)
	// The URL is only used to parse the date from the slug; no HTTP request is made.
	results := parseNSHansardHTMLPage(doc, "https://nslegislature.ca/house_26apr08", 65, 1, 1)

	if len(results) != 1 {
		t.Fatalf("len(results)=%d want 1", len(results))
	}
	div := results[0].Division
	if div.Yeas != 3 {
		t.Errorf("Yeas=%d want 3", div.Yeas)
	}
	if div.Nays != 2 {
		t.Errorf("Nays=%d want 2", div.Nays)
	}
	if div.Date != "2026-04-08" {
		t.Errorf("Date=%q want 2026-04-08", div.Date)
	}

	byName := map[string]string{}
	for _, v := range results[0].Votes {
		byName[v.MemberName] = v.Vote
	}
	for name, wantVote := range map[string]string{
		"Hon. Alice Smith": "Yea",
		"Hon. Carol Brown": "Yea",
		"Eve Taylor":       "Yea",
		"Bob Jones":        "Nay",
		"Dave White":       "Nay",
	} {
		if got := byName[name]; got != wantVote {
			t.Errorf("%s: vote=%q want %q", name, got, wantVote)
		}
	}
}

func TestParseNSHansardHTMLPage_SkipsNonVoteTables(t *testing.T) {
	const html = `<html><body>
<table><thead><tr><th>Name</th><th>Party</th></tr></thead><tbody>
  <tr><td>Alice</td><td>Liberal</td></tr>
</tbody></table>
</body></html>`
	doc := mustDocFromHTML(t, html)
	results := parseNSHansardHTMLPage(doc, "/house_26apr01", 65, 1, 1)
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-vote table, got %d", len(results))
	}
}

func TestDiscoverNovaScotiaVotePDFLinks(t *testing.T) {
	doc := mustDocFromHTML(t, `<html><body>
		<a href="/sites/default/files/pdfs/proceedings/hansard/64-1/h111apr04.pdf?4058">Hansard PDF</a>
		<a href="/sites/default/files/pdfs/proceedings/hansard/64-1/h111apr04.pdf?4058">Hansard PDF duplicate</a>
		<a href="/sites/default/files/pdfs/proceedings/journals/63-3/020%202021Apr19.pdf">Journal PDF</a>
		<a href="/sites/default/files/pdfs/proceedings/journals/63-3/Appendix%20C%20Bills.pdf">Appendix</a>
		<a href="/sites/default/files/pdfs/proceedings/journals/61-1/04%20Cab%20list%20June19.09.pdf">Cabinet list</a>
		<a href="/sites/default/files/pdfs/proceedings/journals/62-1/001%202013oct24.pdf">Wrong session journal PDF</a>
		<a href="/legislative-business/hansard-debates/assembly-64-session-1/house_24apr04">Detail page</a>
	</body></html>`)

	got := discoverNovaScotiaVotePDFLinks(doc, "https://nslegislature.ca/legislative-business/hansard-debates/assembly-64-session-1", 63, 3)
	want := []string{
		"https://nslegislature.ca/sites/default/files/pdfs/proceedings/hansard/64-1/h111apr04.pdf?4058",
		"https://nslegislature.ca/sites/default/files/pdfs/proceedings/journals/63-3/020%202021Apr19.pdf",
	}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}
