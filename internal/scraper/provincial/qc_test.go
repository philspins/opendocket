package provincial

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCrawlQuebecVotes_UsesProvinceMatcher(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><a href="/registre-votes/registre-votes-details.html?vote=1">Registre votes details</a></body></html>`))
	})
	mux.HandleFunc("/registre-votes/registre-votes-details.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><table><tr><td>Pour: 77</td><td>Contre: 32</td></tr></table></body></html>`))
	})

	divs, err := CrawlQuebecVotes(srv.URL, 43, 2, srv.Client())
	if err != nil {
		t.Fatalf("CrawlQuebecVotes: %v", err)
	}
	if len(divs) == 0 {
		t.Fatal("expected at least one parsed quebec division")
	}
}

func TestCrawlQuebecVotes_UsesJSONSearchAndParsesDetailVotes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/index", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body>
			<select class="sessionLegislature">
				<option value="-1">All sessions</option>
				<option value="1617" title="43rd Legislature, 2nd Session (September 30, 2025 - April 8, 2026)">Current</option>
			</select>
		</body></html>`)
	})
	mux.HandleFunc("/Gabarits/RegistreDesVotes.aspx/Rechercher", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"d":{"NumeroPage":0,"QuantiteParPage":25,"NombreTotalDonnees":1,"NomRequete":"mock-query","Donnees":[{"DateVote":"2026-04-02","Titre":"Budget motion","Numero":"171","VoteURL":"/vote/43-2-171"}]}}`)
	})
	mux.HandleFunc("/vote/43-2-171", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body>
			<input type="hidden" id="nbPour" value="53" />
			<input type="hidden" id="nbContre" value="20" />
			<div id="ctl00_ColCentre_ContenuColonneGauche_pnlPour" class="votes">
				<div class="depute"><span class="nom">Allaire</span></div>
			</div>
			<div id="ctl00_ColCentre_ContenuColonneGauche_pnlContre" class="votes">
				<div class="depute"><span class="nom">Tanguay</span></div>
			</div>
		</body></html>`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	divs, err := CrawlQuebecVotes(srv.URL+"/index", 43, 2, srv.Client())
	if err != nil {
		t.Fatalf("CrawlQuebecVotes: %v", err)
	}
	if len(divs) != 1 {
		t.Fatalf("len(divs)=%d, want 1", len(divs))
	}
	if divs[0].Division.Yeas != 53 || divs[0].Division.Nays != 20 {
		t.Fatalf("counts=(%d,%d), want (53,20)", divs[0].Division.Yeas, divs[0].Division.Nays)
	}
	if len(divs[0].Votes) != 2 {
		t.Fatalf("len(votes)=%d, want 2", len(divs[0].Votes))
	}
}

// ── QC TLS client tests ───────────────────────────────────────────────────────

// TestSectigoIntermediatePEM_Valid verifies the embedded cert parses correctly
// and is the expected intermediate (Sectigo Public Server Authentication CA OV R40).
// This will catch accidental corruption of the PEM constant.
func TestSectigoIntermediatePEM_Valid(t *testing.T) {
	block, rest := pem.Decode([]byte(sectigoIntermediatePEM))
	if block == nil {
		t.Fatal("sectigoIntermediatePEM: pem.Decode returned nil block")
	}
	if len(rest) > 0 {
		t.Errorf("sectigoIntermediatePEM: unexpected trailing bytes after PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("sectigoIntermediatePEM: x509.ParseCertificate: %v", err)
	}
	const wantCN = "Sectigo Public Server Authentication CA OV R40"
	if cert.Subject.CommonName != wantCN {
		t.Errorf("CN=%q, want %q", cert.Subject.CommonName, wantCN)
	}
	if cert.IsCA != true {
		t.Error("cert.IsCA=false, expected a CA certificate")
	}
}

// TestQCSourceClient_LocalURLPassesClientThrough verifies that qcSourceClient
// returns the original client unchanged for localhost and 127.0.0.1 URLs,
// which is the condition that lets existing httptest-based tests work.
func TestQCSourceClient_LocalURLPassesClientThrough(t *testing.T) {
	original := &http.Client{Timeout: 42 * time.Second}
	for _, url := range []string{
		"http://localhost/index",
		"http://127.0.0.1:8080/votes",
	} {
		got := qcSourceClient(url, original)
		if got != original {
			t.Errorf("qcSourceClient(%q): returned different client, want passthrough", url)
		}
	}
}

// TestQCSourceClient_ProductionURLReturnsNewClient verifies that qcSourceClient
// creates a new client for real assnat.qc.ca URLs so the Sectigo intermediate
// cert is in scope for the TLS handshake.
func TestQCSourceClient_ProductionURLReturnsNewClient(t *testing.T) {
	original := &http.Client{Timeout: 30 * time.Second}
	got := qcSourceClient("https://www.assnat.qc.ca/en/travaux-parlementaires/registre-des-votes/index.html", original)
	if got == original {
		t.Fatal("qcSourceClient: returned same client for production URL, expected a new client with custom TLS")
	}
}

// TestQCSourceClient_InheritsTimeoutFromOriginalClient verifies that the new
// client created for production URLs uses the caller's timeout, not a hardcoded default.
func TestQCSourceClient_InheritsTimeoutFromOriginalClient(t *testing.T) {
	original := &http.Client{Timeout: 42 * time.Second}
	got := qcSourceClient("https://www.assnat.qc.ca/en/index.html", original)
	if got.Timeout != 42*time.Second {
		t.Errorf("Timeout=%v, want 42s", got.Timeout)
	}
}

// TestQCSourceClient_NilClientDefaultsTo15s verifies the nil-client fallback
// so crawlQuebecVotes doesn't panic when no client is provided.
func TestQCSourceClient_NilClientDefaultsTo15s(t *testing.T) {
	got := qcSourceClient("https://www.assnat.qc.ca/en/index.html", nil)
	if got == nil {
		t.Fatal("qcSourceClient(nil): returned nil")
	}
	if got.Timeout != 15*time.Second {
		t.Errorf("Timeout=%v, want 15s", got.Timeout)
	}
}

// TestNewQCHTTPClient_HasCustomCertPool verifies that the transport inside the
// returned client has a non-nil TLSClientConfig with a populated RootCAs pool.
// This is the structural check that the Sectigo intermediate was actually wired in.
func TestNewQCHTTPClient_HasCustomCertPool(t *testing.T) {
	client := newQCHTTPClient(15 * time.Second)

	qt, ok := client.Transport.(*qcTransport)
	if !ok {
		t.Fatalf("Transport type=%T, want *qcTransport", client.Transport)
	}
	base, ok := qt.base.(*http.Transport)
	if !ok {
		t.Fatalf("qcTransport.base type=%T, want *http.Transport", qt.base)
	}
	if base.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil; Sectigo intermediate was not injected")
	}
	if base.TLSClientConfig.RootCAs == nil {
		t.Fatal("TLSClientConfig.RootCAs is nil; cert pool was not set")
	}
}

// TestQCSourceClient_PassesThroughToTestServer confirms that the existing
// httptest-based QC tests still work after the TLS client change: a local
// URL returns the unmodified test client so plain-HTTP test servers are reachable.
func TestQCSourceClient_PassesThroughToTestServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body>
			<select class="sessionLegislature">
				<option value="1617" title="43rd Legislature, 2nd Session">Current</option>
			</select>
		</body></html>`)
	}))
	defer srv.Close()

	client := qcSourceClient(srv.URL+"/index", srv.Client())
	resp, err := client.Get(srv.URL + "/index")
	if err != nil {
		t.Fatalf("GET via qcSourceClient passthrough: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status=%d, want 200", resp.StatusCode)
	}
}
