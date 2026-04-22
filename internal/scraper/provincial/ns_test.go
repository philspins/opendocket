package provincial

import "testing"

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
