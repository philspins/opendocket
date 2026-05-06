package opennorth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// redirectTransport rewrites every outgoing request to target the given base
// URL, letting tests intercept production HTTP calls via httptest.Server.
type redirectTransport struct {
	base *url.URL
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = rt.base.Scheme
	cloned.URL.Host = rt.base.Host
	return http.DefaultTransport.RoundTrip(cloned)
}

// useTestServer replaces the package-level httpClient so that all requests are
// directed at srv, and restores the original client via t.Cleanup.
func useTestServer(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := httpClient
	t.Cleanup(func() { httpClient = orig })
	base, _ := url.Parse(srv.URL)
	httpClient = &http.Client{Transport: &redirectTransport{base: base}}
}

// ── GetRepresentativesByLatLng ────────────────────────────────────────────────

func TestGetRepresentativesByLatLng_Success(t *testing.T) {
	want := []Representative{
		{Name: "Jane Doe", ElectedOffice: "MP", PartyName: "Liberal"},
		{Name: "John Smith", ElectedOffice: "MPP", PartyName: "NDP"},
	}
	body, _ := json.Marshal(openNorthPage{Objects: want})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()
	useTestServer(t, srv)

	got, err := GetRepresentativesByLatLng(context.Background(), 43.65, -79.38)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d representatives, want 2", len(got))
	}
	if got[0].Name != "Jane Doe" {
		t.Errorf("got[0].Name = %q, want Jane Doe", got[0].Name)
	}
}

func TestGetRepresentativesByLatLng_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	useTestServer(t, srv)

	_, err := GetRepresentativesByLatLng(context.Background(), 43.65, -79.38)
	if err == nil {
		t.Error("expected error for non-200 status, got nil")
	}
}

func TestGetRepresentativesByLatLng_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json{{"))
	}))
	defer srv.Close()
	useTestServer(t, srv)

	_, err := GetRepresentativesByLatLng(context.Background(), 43.65, -79.38)
	if err == nil {
		t.Error("expected decode error for invalid JSON, got nil")
	}
}

func TestGetRepresentativesByLatLng_EmptyList(t *testing.T) {
	body, _ := json.Marshal(openNorthPage{Objects: []Representative{}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()
	useTestServer(t, srv)

	got, err := GetRepresentativesByLatLng(context.Background(), 43.65, -79.38)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d items", len(got))
	}
}

// ── GeocodeAddress ────────────────────────────────────────────────────────────

func TestGeocodeAddress_Success(t *testing.T) {
	resp := geocodeResponse{
		Status: "OK",
		Results: []struct {
			Geometry struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		}{
			{Geometry: struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			}{Location: struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			}{Lat: 43.65, Lng: -79.38}}},
		},
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()
	useTestServer(t, srv)

	lat, lng, err := GeocodeAddress(context.Background(), "Toronto, ON", "fake-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lat != 43.65 || lng != -79.38 {
		t.Errorf("got lat=%v lng=%v, want 43.65 -79.38", lat, lng)
	}
}

func TestGeocodeAddress_NonOKStatus(t *testing.T) {
	resp := geocodeResponse{Status: "ZERO_RESULTS"}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()
	useTestServer(t, srv)

	_, _, err := GeocodeAddress(context.Background(), "Nowhere", "fake-key")
	if err == nil {
		t.Error("expected error for ZERO_RESULTS status, got nil")
	}
}

func TestGeocodeAddress_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{bad json"))
	}))
	defer srv.Close()
	useTestServer(t, srv)

	_, _, err := GeocodeAddress(context.Background(), "Toronto", "fake-key")
	if err == nil {
		t.Error("expected decode error for invalid JSON, got nil")
	}
}
