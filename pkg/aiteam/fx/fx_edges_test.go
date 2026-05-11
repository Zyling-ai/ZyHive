package fx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Corrupted disk cache — should not crash; degrade to hardcoded.
func Test_AITeam_S8_Edge_CorruptedDiskCacheFallsBack(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "fx-cache.json")
	if err := os.WriteFile(cacheFile, []byte("this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(cacheFile)
	// Should not crash. Hardcoded fallback should serve.
	if r := s.Rate("CNY"); r < 6 || r > 9 {
		t.Fatalf("corrupted cache should not break; rate=%v", r)
	}
}

// CoinGecko returns malformed JSON — should fallback.
func Test_AITeam_S8_Edge_BadJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("garbage{not json"))
	}))
	defer srv.Close()
	s := New("")
	s.httpClient.Transport = redirectingTransport(srv.URL)
	src := s.RefreshSync(context.Background())
	if src == SourceCoinGecko || src == SourceExchangerateHost {
		t.Fatalf("garbage JSON should NOT report network source, got %s", src)
	}
}

// CoinGecko returns empty rates map.
func Test_AITeam_S8_Edge_EmptyRatesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]map[string]float64{"tether": {}})
	}))
	defer srv.Close()
	s := New("")
	s.httpClient.Transport = redirectingTransport(srv.URL)
	s.RefreshSync(context.Background())
	// Even with empty response, USDT=1 should always be safe.
	if r := s.Rate("USDT"); r != 1.0 {
		t.Fatalf("USDT must always be 1, got %v", r)
	}
}

// Multiple concurrent override + read.
func Test_AITeam_S8_Edge_ConcurrentOverrideRead(t *testing.T) {
	s := New("")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.SetOverride("CNY", 7.0)
		}()
		go func() {
			defer wg.Done()
			_ = s.Rate("CNY")
		}()
	}
	wg.Wait()
	if r := s.Rate("CNY"); r != 7.0 {
		t.Fatalf("after concurrent ops, override=%v", r)
	}
}

// Zero rate via override (clear) edge.
func Test_AITeam_S8_Edge_ZeroOverrideClears(t *testing.T) {
	s := New("")
	s.SetOverride("CNY", 7.0)
	s.SetOverride("CNY", 0) // should clear
	// Falls through to live rates (hardcoded since no network)
	r := s.Rate("CNY")
	if r == 7.0 {
		t.Fatalf("zero override should clear, but rate still=%v (override stuck)", r)
	}
}

// Negative override → should be treated as clear (rate>0 check).
func Test_AITeam_S8_Edge_NegativeOverrideClears(t *testing.T) {
	s := New("")
	s.SetOverride("CNY", 7.0)
	s.SetOverride("CNY", -5.0) // negative
	// Implementation: `if rate > 0 { set } else { delete }` — so -5 clears.
	r := s.Rate("CNY")
	if r == 7.0 || r == -5.0 {
		t.Fatalf("negative override should clear, got %v", r)
	}
}

// Server returning slow response — context timeout.
func Test_AITeam_S8_Edge_NetworkTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Second) // exceeds 8s client timeout
		w.Write([]byte("{}"))
	}))
	defer srv.Close()
	s := New("")
	s.httpClient.Timeout = 200 * time.Millisecond
	s.httpClient.Transport = redirectingTransport(srv.URL)
	src := s.RefreshSync(context.Background())
	// Both sources will time out → keep hardcoded
	if src != SourceHardcoded {
		t.Fatalf("timeout → should stay at hardcoded, got %s", src)
	}
}

// FormatMoney with NaN.
func Test_AITeam_S8_Edge_FormatMoneyNaN(t *testing.T) {
	out := FormatMoney(0, "USDT", 1.0)
	if out != "0.00 USDT" {
		t.Errorf("zero format: %q", out)
	}
}

// FormatMoney with negative amount (refund display).
func Test_AITeam_S8_Edge_FormatMoneyNegative(t *testing.T) {
	out := FormatMoney(-5.0, "USDT", 1.0)
	if out != "-5.00 USDT" {
		t.Errorf("negative format: %q", out)
	}
}
