package provincial

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// makeWikiServer starts a test server mimicking Wikipedia category and article pages.
// articlesByPath maps article paths (e.g. "/wiki/Blaine_Higgs") to their HTML bodies.
// The category page is served at nbWikipediaCategoryPaths["nb"][0] by default; pass
// a custom categoryPath to test a different province.
func makeWikiServer(t *testing.T, categoryPath string, articlesByPath map[string]string) *provincialWikiLookup {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc(categoryPath, func(w http.ResponseWriter, _ *http.Request) {
		var links strings.Builder
		for path := range articlesByPath {
			name := wikiPathToDisplayName(path)
			links.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a></li>`, path, name))
		}
		fmt.Fprintf(w, `<!DOCTYPE html><html><body><div id="mw-pages"><ul>%s</ul></div></body></html>`, links.String())
	})

	for path, body := range articlesByPath {
		path, body := path, body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, body)
		})
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	lookup := &provincialWikiLookup{
		baseURL:       srv.URL,
		categoryPaths: []string{categoryPath},
		client:        srv.Client(),
		byNormSurname: make(map[string][]wikiEntry),
		articles:      make(map[string]wikiMemberInfo),
	}
	return lookup
}

// wikiPathToDisplayName converts "/wiki/Roland_Hach%C3%A9" → "Roland Haché".
func wikiPathToDisplayName(path string) string {
	if len(path) > 6 {
		path = path[6:] // strip /wiki/
	}
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return path
	}
	return strings.ReplaceAll(decoded, "_", " ")
}

func memberArticleHTML(party, riding string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><body>
<table class="infobox vcard">
<tr><th>Political party</th><td><a href="/wiki/Party">%s</a></td></tr>
<tr><th>New Brunswick Electoral District</th><td>%s</td></tr>
</table>
</body></html>`, party, riding)
}

// memberArticleHTMLWithQualifier mimics the Wikipedia infobox structure for a
// member who recently changed parties: the party link is followed by a plain-text
// qualifier outside the <a> tag, and the riding appears in a colspan-2 office
// header row with no <td> (the {{Infobox officeholder}} template pattern).
func memberArticleHTMLWithQualifier(party, qualifier, riding string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><body>
<table class="infobox vcard">
<tr><th colspan="2" class="infobox-header"><a href="/wiki/MPP">Member of Provincial Parliament</a> <br /> for <a href="/wiki/Riding">%s</a></th></tr>
<tr><th scope="row">Party</th><td><a href="/wiki/Party">%s</a> %s</td></tr>
<tr><th scope="row">Other political affiliations</th><td><a href="/wiki/OldParty">Ontario New Democratic</a> (2018–2026)</td></tr>
</table>
</body></html>`, riding, party, qualifier)
}

const testCategoryPath = "/wiki/Category:21st-century_members_of_the_Legislative_Assembly_of_New_Brunswick"

func TestWikiLookup_BySurname(t *testing.T) {
	lookup := makeWikiServer(t, testCategoryPath, map[string]string{
		"/wiki/Roland_Hach%C3%A9": memberArticleHTML("Liberal Party of New Brunswick", "Tracadie"),
		"/wiki/Blaine_Higgs":      memberArticleHTML("Progressive Conservative Party of New Brunswick", "Quispamsis"),
	})

	party, riding, ok := lookup.lookup("Haché")
	if !ok {
		t.Fatal("expected lookup to succeed for Haché")
	}
	if party != "Liberal Party of New Brunswick" {
		t.Errorf("party=%q, want %q", party, "Liberal Party of New Brunswick")
	}
	if riding != "Tracadie" {
		t.Errorf("riding=%q, want %q", riding, "Tracadie")
	}
}

func TestWikiLookup_AccentFolding(t *testing.T) {
	lookup := makeWikiServer(t, testCategoryPath, map[string]string{
		"/wiki/Roland_Hach%C3%A9": memberArticleHTML("Liberal Party of New Brunswick", "Tracadie"),
	})

	// "Hache" (no accent) should still match "Haché".
	party, _, ok := lookup.lookup("Hache")
	if !ok {
		t.Fatal("expected accent-folded lookup to succeed for Hache → Haché")
	}
	if party != "Liberal Party of New Brunswick" {
		t.Errorf("party=%q, want %q", party, "Liberal Party of New Brunswick")
	}
}

func TestWikiLookup_ByFullName(t *testing.T) {
	lookup := makeWikiServer(t, testCategoryPath, map[string]string{
		"/wiki/Roland_Hach%C3%A9": memberArticleHTML("Liberal Party of New Brunswick", "Tracadie"),
		"/wiki/Blaine_Higgs":      memberArticleHTML("Progressive Conservative Party of New Brunswick", "Quispamsis"),
	})

	party, riding, ok := lookup.lookup("Blaine Higgs")
	if !ok {
		t.Fatal("expected lookup to succeed for Blaine Higgs")
	}
	if party != "Progressive Conservative Party of New Brunswick" {
		t.Errorf("party=%q", party)
	}
	if riding != "Quispamsis" {
		t.Errorf("riding=%q", riding)
	}
}

func TestWikiLookup_Disambiguation(t *testing.T) {
	// Two people share "Oliver" as a surname.
	lookup := makeWikiServer(t, testCategoryPath, map[string]string{
		"/wiki/Bill_Oliver":  memberArticleHTML("Progressive Conservative Party of New Brunswick", "Kings Centre"),
		"/wiki/Carol_Oliver": memberArticleHTML("Liberal Party of New Brunswick", "Fredericton-Grand Lake"),
	})

	party, riding, ok := lookup.lookup("Carol Oliver")
	if !ok {
		t.Fatal("expected lookup to succeed for Carol Oliver")
	}
	if party != "Liberal Party of New Brunswick" {
		t.Errorf("party=%q, want Carol's party", party)
	}
	if riding != "Fredericton-Grand Lake" {
		t.Errorf("riding=%q", riding)
	}
}

func TestWikiLookup_UnknownName(t *testing.T) {
	lookup := makeWikiServer(t, testCategoryPath, map[string]string{
		"/wiki/Blaine_Higgs": memberArticleHTML("Progressive Conservative Party of New Brunswick", "Quispamsis"),
	})

	_, _, ok := lookup.lookup("Someone Nobody")
	if ok {
		t.Fatal("expected lookup to fail for unknown name")
	}
}

func TestWikiLookup_ArticleCached(t *testing.T) {
	fetchCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc(testCategoryPath, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body><div id="mw-pages"><ul><li><a href="/wiki/Blaine_Higgs">Blaine Higgs</a></li></ul></div></body></html>`)
	})
	mux.HandleFunc("/wiki/Blaine_Higgs", func(w http.ResponseWriter, _ *http.Request) {
		fetchCount++
		fmt.Fprint(w, memberArticleHTML("Progressive Conservative Party of New Brunswick", "Quispamsis"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	lookup := &provincialWikiLookup{
		baseURL:       srv.URL,
		categoryPaths: []string{testCategoryPath},
		client:        srv.Client(),
		byNormSurname: make(map[string][]wikiEntry),
		articles:      make(map[string]wikiMemberInfo),
	}

	for i := 0; i < 3; i++ {
		lookup.lookup("Higgs")
	}
	if fetchCount != 1 {
		t.Errorf("article fetched %d times, want 1 (should be cached)", fetchCount)
	}
}

func TestWikiLookup_CategoryFetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	lookup := &provincialWikiLookup{
		baseURL:       srv.URL,
		categoryPaths: []string{testCategoryPath},
		client:        srv.Client(),
		byNormSurname: make(map[string][]wikiEntry),
		articles:      make(map[string]wikiMemberInfo),
	}

	_, _, ok := lookup.lookup("Higgs")
	if ok {
		t.Fatal("expected lookup to fail gracefully when category is unavailable")
	}
}

func TestWikiLookup_MultipleCategoriesMerged(t *testing.T) {
	// 21st-century category has Higgs; 20th-century category has McKenna.
	// Both should be found by the same lookup instance.
	path21 := "/wiki/Category:21st-century_members_test"
	path20 := "/wiki/Category:20th-century_members_test"

	mux := http.NewServeMux()
	mux.HandleFunc(path21, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body><div id="mw-pages"><ul><li><a href="/wiki/Blaine_Higgs">Blaine Higgs</a></li></ul></div></body></html>`)
	})
	mux.HandleFunc(path20, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body><div id="mw-pages"><ul><li><a href="/wiki/Frank_McKenna">Frank McKenna</a></li></ul></div></body></html>`)
	})
	mux.HandleFunc("/wiki/Blaine_Higgs", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, memberArticleHTML("Progressive Conservative Party of New Brunswick", "Quispamsis"))
	})
	mux.HandleFunc("/wiki/Frank_McKenna", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, memberArticleHTML("Liberal Party of New Brunswick", "Chatham"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	lookup := &provincialWikiLookup{
		baseURL:       srv.URL,
		categoryPaths: []string{path21, path20},
		client:        srv.Client(),
		byNormSurname: make(map[string][]wikiEntry),
		articles:      make(map[string]wikiMemberInfo),
	}

	if _, _, ok := lookup.lookup("Higgs"); !ok {
		t.Error("expected Higgs (21st-century category) to be found")
	}
	if _, _, ok := lookup.lookup("McKenna"); !ok {
		t.Error("expected McKenna (20th-century category) to be found")
	}
}

func TestWikiLookup_NewProvincialWikiLookup(t *testing.T) {
	// All 10 province codes should produce a non-nil lookup.
	for _, code := range []string{"ab", "bc", "mb", "nb", "nl", "ns", "on", "pe", "qc", "sk"} {
		if l := newProvincialWikiLookup(code, nil); l == nil {
			t.Errorf("newProvincialWikiLookup(%q) returned nil", code)
		}
	}
	// Unknown codes should return nil.
	if l := newProvincialWikiLookup("xx", nil); l != nil {
		t.Error("newProvincialWikiLookup(xx) should return nil for unknown province")
	}
}

func TestWikiLookup_PartyQualifierStripped(t *testing.T) {
	// Reproduces the "Liberal (since 2026)" bug: the party link text is "Liberal"
	// but the cell also contains plain-text "(since 2026)" outside the <a> tag.
	lookup := makeWikiServer(t, testCategoryPath, map[string]string{
		"/wiki/Doly_Begum": memberArticleHTMLWithQualifier("Liberal", "(since 2026)", "Scarborough Southwest"),
	})

	party, riding, ok := lookup.lookup("Begum")
	if !ok {
		t.Fatal("expected lookup to succeed")
	}
	if party != "Liberal" {
		t.Errorf("party=%q, want %q (qualifier should be stripped)", party, "Liberal")
	}
	if riding != "Scarborough Southwest" {
		t.Errorf("riding=%q, want %q", riding, "Scarborough Southwest")
	}
}

func TestWikiLookup_RidingFromColspan2Header(t *testing.T) {
	// The riding appears in a <th colspan="2"> office-header row with no <td>.
	// The provincial "Member of Provincial Parliament" row must be preferred over
	// the federal "Member of Parliament" row when both are present.
	const html = `<!DOCTYPE html><html><body>
<table class="infobox vcard">
<tr><th colspan="2" class="infobox-header"><a href="/wiki/MP">Member of Parliament</a><br />for <a href="/wiki/FedRiding">Scarborough Southwest (federal)</a></th></tr>
<tr><th colspan="2" class="infobox-header"><a href="/wiki/MPP">Member of Provincial Parliament</a> <br /> for <a href="/wiki/ProvRiding">Scarborough Southwest</a></th></tr>
<tr><th scope="row">Party</th><td><a href="/wiki/Party">Liberal</a></td></tr>
</table>
</body></html>`

	lookup := makeWikiServer(t, testCategoryPath, map[string]string{
		"/wiki/Doly_Begum": html,
	})

	_, riding, ok := lookup.lookup("Begum")
	if !ok {
		t.Fatal("expected lookup to succeed")
	}
	if riding != "Scarborough Southwest" {
		t.Errorf("riding=%q, want provincial %q (federal row must be skipped)", riding, "Scarborough Southwest")
	}
}

func TestWikiLookup_RidingFallsBackToFederalRow(t *testing.T) {
	// When no provincial legislature row is present (e.g. the article was written
	// after the member became a federal MP and the provincial role was removed),
	// the "Member of Parliament for [Riding]" row should still be used.
	const html = `<!DOCTYPE html><html><body>
<table class="infobox vcard">
<tr><th colspan="2" class="infobox-header"><a href="/wiki/MP">Member of Parliament</a><br />for <a href="/wiki/Riding">Scarborough Southwest</a></th></tr>
<tr><th scope="row">Party</th><td><a href="/wiki/Party">Liberal</a></td></tr>
</table>
</body></html>`

	lookup := makeWikiServer(t, testCategoryPath, map[string]string{
		"/wiki/Doly_Begum": html,
	})

	_, riding, ok := lookup.lookup("Begum")
	if !ok {
		t.Fatal("expected lookup to succeed")
	}
	if riding != "Scarborough Southwest" {
		t.Errorf("riding=%q, want %q (should fall back to federal row)", riding, "Scarborough Southwest")
	}
}
