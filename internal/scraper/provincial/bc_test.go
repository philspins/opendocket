package provincial

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCrawlBritishColumbiaVotes_UsesLIMSAPI(t *testing.T) {
	vpHTML := `<!DOCTYPE html><html><body>
<p>Motion agreed to on the following division:</p>
<table class="division">
<tr><td class="head" colspan="4">Yeas &#8212; 11</td></tr>
<tr><td>Eby <br>Farnworth <br>Sharma <br></td><td>Dix <br>Beare <br>Boyle <br></td><td>Kahlon <br>Bailey <br>Gibson <br></td><td>Glumac <br>Arora <br></td></tr>
<tr><td class="head" colspan="4">Nays &#8212; 4</td></tr>
<tr><td>Rustad <br></td><td>Milobar <br></td><td>Halford <br></td><td>Dew <br></td></tr>
</table>
</body></html>`

	limsJSON := `{"allParliamentaryFileAttributes":{"nodes":[{"fileName":"v260407.htm","filePath":"/ldp/43rd2nd/votes/","published":true,"date":"2026-04-07T00:00:00","votesAttributesByFileId":{"nodes":[{"voteNumbers":"38"}]}}]}}`

	mux := http.NewServeMux()
	mux.HandleFunc("/pdms/votes-and-proceedings/43rd2nd", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(limsJSON))
	})
	mux.HandleFunc("/pdms/ldp/43rd2nd/votes/v260407.htm", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(vpHTML))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	divs, err := CrawlBritishColumbiaVotes(srv.URL, 43, 2, srv.Client())
	if err != nil {
		t.Fatalf("CrawlBritishColumbiaVotes: %v", err)
	}
	if len(divs) == 0 {
		t.Fatal("expected at least one parsed bc division")
	}
	if divs[0].Division.Yeas != 11 || divs[0].Division.Nays != 4 {
		t.Fatalf("counts=(%d,%d), want (11,4)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
}

func TestParseBCVotesDivisions_ParsesDivisionTableYeasNays(t *testing.T) {
	html := `<html><body>
<p>Motion agreed to on the following division:</p>
<table width="600" cellpadding="0" cellspacing="0" class="division">
<tr>
<td valign="top" class="head" colspan="4">Yeas &#8212; 48</td>
</tr>
<tr>
<td valign="top" width="25%">Eby <br>Farnworth <br>Sharma <br></td>
<td valign="top" width="25%">Dix <br>Beare <br>Boyle <br></td>
<td valign="top" width="25%">Kahlon <br>Bailey <br>Gibson <br></td>
<td valign="top" width="25%">Glumac <br>Arora <br>Shah <br></td>
</tr>
<tr>
<td valign="top" class="head" colspan="4">Nays &#8212; 40</td>
</tr>
<tr>
<td valign="top" width="25%">Rustad <br>Milobar <br>Halford <br></td>
<td valign="top" width="25%">Dew <br>Clare <br>Rattee <br></td>
<td valign="top" width="25%">Bird <br>Stamer <br>Day <br></td>
<td valign="top" width="25%">Doerkson <br>Luck <br>Block <br></td>
</tr>
</table>
</body></html>`

	divs := ParseBCVotesDivisionsForTest(html, "https://example.com/v251201.htm", "2025-12-01", 43, 1, 1)
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	d := divs[0]
	if d.Division.Yeas != 48 || d.Division.Nays != 40 {
		t.Fatalf("counts=(%d,%d), want (48,40)", d.Division.Yeas, d.Division.Nays)
	}
	if d.Division.Result != "Carried" {
		t.Fatalf("result=%q, want Carried", d.Division.Result)
	}
	if len(d.Votes) < 24 {
		t.Fatalf("len(votes)=%d, want >=24", len(d.Votes))
	}
	yeaCount, nayCount := 0, 0
	for _, v := range d.Votes {
		if v.Vote == "Yea" {
			yeaCount++
		} else if v.Vote == "Nay" {
			nayCount++
		}
	}
	if yeaCount == 0 || nayCount == 0 {
		t.Fatalf("yeaCount=%d nayCount=%d, want both >0", yeaCount, nayCount)
	}
}

func TestParseBCVotesDivisions_UsesPriorSubstantiveParagraphForDescription(t *testing.T) {
	html := `<html><body>
<p>On the motion of <em>Tara Armstrong</em> that Bill (No.&nbsp;M 201) intituled <em>Public Safety Statutes Amendment Act</em> be introduced and read a first time, the House divided.</p>
<p>Motion negatived on the following division:</p>
<table width="600" cellpadding="0" cellspacing="0" class="division">
<tr><td class="head" colspan="4">Yeas &#8212; 3</td></tr>
<tr><td>Armstrong <br></td><td>Jones <br></td><td>Brown <br></td><td></td></tr>
<tr><td class="head" colspan="4">Nays &#8212; 6</td></tr>
<tr><td>Allen <br></td><td>Foster <br></td><td>Mok <br></td><td>Lee <br>Smith <br>Taylor <br></td></tr>
</table>
</body></html>`

	divs := ParseBCVotesDivisionsForTest(html, "https://example.com/v251202.htm", "2025-12-02", 43, 1, 1)
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if divs[0].Division.Description != "On the motion of Tara Armstrong that Bill (No. M 201) intituled Public Safety Statutes Amendment Act be introduced and read a first time, the House divided." {
		t.Fatalf("description=%q", divs[0].Division.Description)
	}
	if billNumber := ExtractProvincialBillNumber(divs[0].Division.Description); billNumber != "M-201" {
		t.Fatalf("billNumber=%q, want M-201", billNumber)
	}
}

func TestParseBCVotesDivisions_NaysExceedYeadsIsNegatived(t *testing.T) {
	html := `<html><body>
<p>Amendment was defeated on the following division:</p>
<table width="600" cellpadding="0" cellspacing="0" class="division">
<tr><td valign="top" class="head" colspan="4">Nays &#8212; 6</td></tr>
<tr>
<td valign="top" width="25%">Smith <br>Jones <br></td>
<td valign="top" width="25%">Brown <br>Davis <br></td>
<td valign="top" width="25%">Wilson <br>Taylor <br></td>
<td valign="top" width="25%"></td>
</tr>
<tr><td valign="top" class="head" colspan="4">Yeas &#8212; 3</td></tr>
<tr>
<td valign="top" width="25%">Allen <br></td>
<td valign="top" width="25%">Foster <br></td>
<td valign="top" width="25%">Mok <br></td>
<td valign="top" width="25%"></td>
</tr>
</table>
</body></html>`

	divs := ParseBCVotesDivisionsForTest(html, "https://example.com/v251202.htm", "2025-12-02", 43, 1, 1)
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if divs[0].Division.Result != "Negatived" {
		t.Fatalf("result=%q, want Negatived", divs[0].Division.Result)
	}
	if divs[0].Division.Yeas != 3 || divs[0].Division.Nays != 6 {
		t.Fatalf("counts=(%d,%d), want (3,6)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
}
