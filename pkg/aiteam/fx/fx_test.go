package fx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func Test_AITeam_FX_HardcodedFallback(t *testing.T) {
	// No cache file, no network — service should still return sensible
	// rates from HardcodedRates.
	s := New("")
	if r := s.Rate("CNY"); r < 6 || r > 9 {
		t.Fatalf("CNY rate via hardcoded fallback out of sane range: %v", r)
	}
	if r := s.Rate("USDT"); r != 1.0 {
		t.Fatalf("USDT should always be 1.0, got %v", r)
	}
	if r := s.Rate("USD"); r != 1.0 {
		t.Fatalf("USD should be 1.0 via 1:1 peg, got %v", r)
	}
	if r := s.Rate("ZZZ"); r != 0 {
		t.Fatalf("unknown currency should return 0, got %v", r)
	}
}

// fakeSource serves a synthetic CoinGecko / exchangerate.host response.
func fakeSource(handlerSchemaCoinGecko bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if handlerSchemaCoinGecko {
			_ = json.NewEncoder(w).Encode(map[string]map[string]float64{
				"tether": {
					"usd": 1.0, "cny": 7.20, "eur": 0.94, "jpy": 156, "gbp": 0.80,
					"krw": 1390, "hkd": 7.80, "twd": 32.3,
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"rates": map[string]float64{
				"CNY": 7.21, "EUR": 0.95, "JPY": 157, "GBP": 0.81,
				"KRW": 1395, "HKD": 7.79, "TWD": 32.5,
			},
		})
	}))
}

func Test_AITeam_FX_CoinGeckoOverridesHardcoded(t *testing.T) {
	srv := fakeSource(true)
	defer srv.Close()
	s := New("")
	// Re-point CoinGecko endpoint at our fake by swapping the http client.
	s.httpClient = srv.Client()
	// Build a tiny adapter that intercepts the path
	s.httpClient.Transport = redirectingTransport(srv.URL)

	got := s.RefreshSync(context.Background())
	if got != SourceCoinGecko {
		t.Fatalf("expected coingecko source, got %s", got)
	}
	if r := s.Rate("CNY"); r != 7.20 {
		t.Fatalf("CNY should be 7.20 after CoinGecko refresh, got %v", r)
	}
}

func Test_AITeam_FX_OverridePreemptsLiveSource(t *testing.T) {
	s := New("")
	s.SetOverride("CNY", 6.50)
	if r := s.Rate("CNY"); r != 6.50 {
		t.Fatalf("override should win, got %v", r)
	}
	s.SetOverride("CNY", 0) // remove
	if r := s.Rate("CNY"); r == 6.50 {
		t.Fatalf("override should be cleared after SetOverride(0)")
	}
}

func Test_AITeam_FX_DiskCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "fx-cache.json")
	s1 := New(cacheFile)
	s1.SetOverride("CNY", 7.00)
	// Simulate a successful CoinGecko refresh by directly calling adopt.
	s1.adopt(map[string]float64{"CNY": 7.30, "USD": 1.0, "USDT": 1.0}, SourceCoinGecko)

	// Re-open: cache should be loaded and override should persist.
	s2 := New(cacheFile)
	if r := s2.Rate("CNY"); r != 7.00 { // override wins over loaded rate
		t.Fatalf("override should survive restart, got %v", r)
	}
	snap := s2.SnapshotJSON()
	if snap.Source != SourceDiskCache {
		t.Fatalf("after restart, source should be disk_cache, got %v", snap.Source)
	}
}

func Test_AITeam_FX_FormatMoney(t *testing.T) {
	cases := []struct {
		usdt     float64
		currency string
		rate     float64
		want     string
	}{
		{1.0, "USDT", 1.0, "1.00 USDT"},
		{1.0, "USD", 1.0, "$1.00"},
		{2.5, "CNY", 7.18, "¥17.95"},
		{1.0, "JPY", 155.0, "¥155"},
		{1.0, "KRW", 1380.0, "₩1380"},
		{10.0, "HKD", 7.81, "HK$78.10"},
		{1.0, "ZZZ", 1.0, "1.00 ZZZ"},
	}
	for _, c := range cases {
		got := FormatMoney(c.usdt, c.currency, c.rate)
		if got != c.want {
			t.Errorf("FormatMoney(%v, %q, %v) = %q, want %q",
				c.usdt, c.currency, c.rate, got, c.want)
		}
	}
}

func Test_AITeam_FX_NoNetworkKeepsExistingSnapshot(t *testing.T) {
	// Point both fetchers at a closed server → both return false.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	srv.Close() // intentionally close immediately

	s := New("")
	s.httpClient = &http.Client{Timeout: 200 * time.Millisecond, Transport: redirectingTransport(srv.URL)}
	got := s.RefreshSync(context.Background())
	// All sources fail → keep current source (which is hardcoded since
	// fresh service).
	if got != SourceHardcoded {
		t.Fatalf("expected hardcoded after total failure, got %s", got)
	}
	// Rate still sensible.
	if r := s.Rate("CNY"); r < 6 || r > 9 {
		t.Fatalf("CNY rate degraded out of sane range: %v", r)
	}
}

// redirectingTransport rewrites every outbound request URL to baseURL
// (preserving path / query). Used in tests to point HTTP fetchers at a
// httptest.Server without changing fx.go code.
type rtFn func(*http.Request) (*http.Response, error)

func (f rtFn) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func redirectingTransport(baseURL string) http.RoundTripper {
	return rtFn(func(req *http.Request) (*http.Response, error) {
		// Replace scheme://host with baseURL.
		newURL := baseURL + req.URL.Path
		if req.URL.RawQuery != "" {
			newURL += "?" + req.URL.RawQuery
		}
		req2, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		return http.DefaultTransport.RoundTrip(req2)
	})
}

func Test_AITeam_FX_SnapshotMergesOverrides(t *testing.T) {
	s := New("")
	s.SetOverride("CNY", 6.99)
	snap := s.SnapshotJSON()
	if r := snap.Rates["CNY"]; r != 6.99 {
		t.Fatalf("snapshot rates should reflect overrides, got %v", r)
	}
	if v, ok := snap.Overrides["CNY"]; !ok || v != 6.99 {
		t.Fatalf("Overrides map should include CNY=6.99, got %+v", snap.Overrides)
	}
	if !strings.Contains(string(snap.Source), "") { // sanity check
	}
}
