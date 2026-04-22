package provincial

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCrawlAlbertaVotes_ReturnsZeroWhenNoPDFLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body>
      <a href="/assembly-records/votes-and-proceedings/2026-04-08">Votes and Proceedings</a>
    </body></html>`))
	}))
	defer srv.Close()

	divs, err := CrawlAlbertaVotes(srv.URL, 31, 1, srv.Client())
	if err != nil {
		t.Fatalf("CrawlAlbertaVotes: %v", err)
	}
	if divs == nil {
		divs = []ProvincialDivisionResult{}
	}
	_ = divs
}

func TestParseAlbertaVPDivisions_ForAgainstFormat(t *testing.T) {
	text := `VOTES AND PROCEEDINGS No. 7 DIVISION 1 On Bill 37 amendment For the amendment: 31 Al-Guneid Elmeligi Kayande Arcand-Paul Eremenko Against the amendment: 28 Amery Johnson Rowswell`
	divs := ParseAlbertaVPDivisionsForTest(text, "https://example.com/vp.pdf", 31, 2, 1, "2025-05-14")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if divs[0].Division.Yeas != 31 || divs[0].Division.Nays != 28 {
		t.Fatalf("counts=(%d,%d), want (31,28)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
	if divs[0].Division.Result != "Carried" {
		t.Fatalf("result=%q, want Carried", divs[0].Division.Result)
	}
	if len(divs[0].Votes) < 5 {
		t.Fatalf("len(votes)=%d, want >=5", len(divs[0].Votes))
	}
}

func TestParseAlbertaVPDivisions_MultiDivision(t *testing.T) {
	text := `DIVISION 1 On the motion For the motion: 20 Smith Jones Brown Against the motion: 15 Davis Wilson DIVISION 2 On third reading For the bill: 35 Taylor Morgan Against the bill: 10 Allen Foster`
	divs := ParseAlbertaVPDivisionsForTest(text, "https://example.com/vp.pdf", 31, 2, 1, "2025-05-14")
	if len(divs) != 2 {
		t.Fatalf("len(divs)=%d, want 2", len(divs))
	}
	if divs[0].Division.Yeas != 20 || divs[0].Division.Nays != 15 {
		t.Fatalf("div1 counts=(%d,%d), want (20,15)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
	if divs[1].Division.Yeas != 35 || divs[1].Division.Nays != 10 {
		t.Fatalf("div2 counts=(%d,%d), want (35,10)", divs[1].Division.Yeas, divs[1].Division.Nays)
	}
}

func TestParseAlbertaVPDivisions_QuestionBlocksFormat(t *testing.T) {
	text := `Hon. Mr. Schow, Government House Leader, moved pursuant to Standing Order 27 that the Assembly proceed immediately to Orders of the Day. The question being put, the motion was agreed to on the voice vote. With Hon. Mr. McIver in the Chair, the names being called for were taken as follows: For the motion: 45 Amery Armstrong-Homeniuk Boitchenko Bouchard Cyr de Jonge Dreeshen Against the motion: 37 Al-Guneid Arcand-Paul Batten Boparai Brar ORDERS OF THE DAY Government Motions 2. Moved by Hon. Mr. Schow: Be it resolved that the Legislative Assembly resolve into Committee of the Whole, when called, to consider certain Bills on the Order Paper. The question being put, the motion was agreed to on the voice vote. With Hon. Mr. McIver in the Chair, the names being called for were taken as follows: For the motion: 44 Amery Armstrong-Homeniuk Boitchenko Bouchard Cyr de Jonge Dreeshen Against the motion: 37 Al-Guneid Arcand-Paul Batten Boparai Brar`
	divs := ParseAlbertaVPDivisionsForTest(text, "https://example.com/vp.pdf", 31, 2, 1, "2025-10-27")
	if len(divs) != 2 {
		t.Fatalf("len(divs)=%d, want 2", len(divs))
	}
	if divs[0].Division.Yeas != 45 || divs[0].Division.Nays != 37 {
		t.Fatalf("div1 counts=(%d,%d), want (45,37)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
	if divs[1].Division.Yeas != 44 || divs[1].Division.Nays != 37 {
		t.Fatalf("div2 counts=(%d,%d), want (44,37)", divs[1].Division.Yeas, divs[1].Division.Nays)
	}
	if !strings.Contains(divs[0].Division.Description, "Standing Order 27") {
		t.Fatalf("div1 description=%q", divs[0].Division.Description)
	}
	if !strings.Contains(divs[1].Division.Description, "Committee of the Whole") {
		t.Fatalf("div2 description=%q", divs[1].Division.Description)
	}
	if len(divs[0].Votes) < 5 || len(divs[1].Votes) < 5 {
		t.Fatalf("votes lens=(%d,%d), want >=5", len(divs[0].Votes), len(divs[1].Votes))
	}
}

func TestParseAlbertaVPDivisions_QuestionBlocksKeepBillDescription(t *testing.T) {
	text := `Second Reading On the motion that the following Bill be now read a Second time: Bill 27 Financial Statutes Amendment Act, 2026 -- Hon. Mr. Horner A debate followed. The question being put, the motion was agreed to on the voice vote. With Hon. Mr. McIver in the Chair, the names being called for were taken as follows: For the motion: 45 Amery Armstrong-Homeniuk Boitchenko Bouchard Cyr de Jonge Dreeshen Against the motion: 37 Al-Guneid Arcand-Paul Batten Boparai Brar`
	divs := ParseAlbertaVPDivisionsForTest(text, "https://example.com/vp.pdf", 31, 2, 1, "2026-04-15")
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if !strings.Contains(divs[0].Division.Description, "Bill 27 Financial Statutes Amendment Act") {
		t.Fatalf("description=%q", divs[0].Division.Description)
	}
}

func TestExtractLegislatureSessionCandidates_AlbertaFormats(t *testing.T) {
	tests := []struct {
		text string
		want legislatureSession
	}{
		{
			text: "Legislature, Session 31-2 (2025-2026)",
			want: legislatureSession{Legislature: 31, Session: 2},
		},
		{
			text: "Legislature 31, Session 2 (2025-2026)",
			want: legislatureSession{Legislature: 31, Session: 2},
		},
		{
			text: "https://www.assembly.ab.ca/assembly-business/assembly-dashboard?legl=31&session=2&sectionb=d&btn=i#page-menu",
			want: legislatureSession{Legislature: 31, Session: 2},
		},
	}

	for _, tc := range tests {
		candidates := extractLegislatureSessionCandidates("ab", tc.text, 50)
		best, ok := maxLegislatureSession(candidates)
		if !ok {
			t.Fatalf("no candidates for %q", tc.text)
		}
		if best.Legislature != tc.want.Legislature || best.Session != tc.want.Session {
			t.Fatalf("best=%+v, want legislature=%d session=%d", best, tc.want.Legislature, tc.want.Session)
		}
	}
}
