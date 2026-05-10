// Package fx provides the multi-currency display layer for aiteam.
//
// Internal accounting is always USDT (see PLAN § 2.7); FX rates only
// affect what the UI renders. The Service interface returns the rate
// of `target` currency per 1 USDT (i.e. CNY ≈ 7.18 means 1 USDT shows
// as ¥7.18). Rates are fetched from CoinGecko (primary), then
// exchangerate.host (fallback), with a hard-coded snapshot as a final
// safety net so a clean install on a network-restricted VM still
// renders sensible numbers.
//
// Operators can override any rate via SetOverride; overrides win over
// all live sources and survive restarts via the on-disk cache.
//
// Concurrency: Service is safe for concurrent Rate() / Refresh() /
// Snapshot() calls. Refresh() runs the network fetches under a
// separate goroutine to avoid blocking callers; tests can call
// RefreshSync() for deterministic behaviour.
package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SupportedCurrencies enumerates the displayable currencies. USDT is
// always present with rate 1.0.
var SupportedCurrencies = []string{
	"USDT", "USD", "CNY", "EUR", "JPY", "GBP", "KRW", "HKD", "TWD",
}

// HardcodedRates is the final fallback when both network sources fail
// AND no disk cache is present. Values are calibrated to spring 2026
// approximations and intentionally rounded — a hard-coded fallback is
// not meant to be precise. UI will show a "⚠️ FX estimate" badge when
// these are used.
var HardcodedRates = map[string]float64{
	"USDT": 1.0,
	"USD":  1.0,
	"CNY":  7.18,
	"EUR":  0.93,
	"JPY":  155.0,
	"GBP":  0.79,
	"KRW":  1380.0,
	"HKD":  7.81,
	"TWD":  32.4,
}

// Source describes the provenance of the active rate snapshot.
type Source string

const (
	SourceCoinGecko       Source = "coingecko"
	SourceExchangerateHost Source = "exchangerate.host"
	SourceHardcoded       Source = "hardcoded"
	SourceDiskCache       Source = "disk_cache"
)

// Snapshot is the read-only view returned to callers / API.
type Snapshot struct {
	Base       string             `json:"base"`         // always "USDT"
	Rates      map[string]float64 `json:"rates"`        // currency → rate (1 USDT = N <currency>)
	Source     Source             `json:"source"`
	FetchedAt  time.Time          `json:"fetched_at"`
	Overrides  map[string]float64 `json:"overrides,omitempty"` // currency → manually overridden rate
}

// Service is the public FX engine.
type Service struct {
	httpClient *http.Client
	cacheFile  string

	mainTTL     time.Duration
	fallbackTTL time.Duration

	mu        sync.RWMutex
	snap      Snapshot
	overrides map[string]float64
}

// New constructs a Service. cacheFile may be "" to disable disk cache.
// On startup, the disk cache is loaded if present so a fresh process
// is never "blind" before the first network refresh completes.
func New(cacheFile string) *Service {
	s := &Service{
		httpClient:  &http.Client{Timeout: 8 * time.Second},
		cacheFile:   cacheFile,
		mainTTL:     time.Hour,
		fallbackTTL: 24 * time.Hour,
		overrides:   map[string]float64{},
		snap: Snapshot{
			Base:      "USDT",
			Rates:     copyMap(HardcodedRates),
			Source:    SourceHardcoded,
			FetchedAt: time.Now(),
		},
	}
	_ = s.loadCache()
	return s
}

// SetTTL adjusts both TTLs at once (mainly for tests).
func (s *Service) SetTTL(main, fallback time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if main > 0 {
		s.mainTTL = main
	}
	if fallback > 0 {
		s.fallbackTTL = fallback
	}
}

// Rate returns the rate of 1 USDT in `currency`. Currency case-insensitive.
// Unknown currency → 0.
func (s *Service) Rate(currency string) float64 {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.overrides[currency]; ok {
		return v
	}
	return s.snap.Rates[currency]
}

// SetOverride installs a manual rate override. rate=0 removes the override.
// Persists to disk cache so it survives restart.
func (s *Service) SetOverride(currency string, rate float64) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return
	}
	s.mu.Lock()
	if rate > 0 {
		s.overrides[currency] = rate
	} else {
		delete(s.overrides, currency)
	}
	s.snap.Overrides = copyMap(s.overrides)
	s.mu.Unlock()
	_ = s.saveCache()
}

// SnapshotJSON returns a copy of the current snapshot, with overrides
// already merged into Rates so external consumers see the effective
// rates plus the Overrides map for transparency.
func (s *Service) SnapshotJSON() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := Snapshot{
		Base:      s.snap.Base,
		Rates:     copyMap(s.snap.Rates),
		Source:    s.snap.Source,
		FetchedAt: s.snap.FetchedAt,
		Overrides: copyMap(s.overrides),
	}
	for k, v := range s.overrides {
		out.Rates[k] = v
	}
	return out
}

// RefreshAsync triggers a background refresh; returns immediately.
func (s *Service) RefreshAsync() {
	go func() { _ = s.RefreshSync(context.Background()) }()
}

// RefreshSync runs the fetch synchronously; returns the source that
// ultimately won. Errors are not returned; failing every source merely
// keeps the existing snapshot.
func (s *Service) RefreshSync(ctx context.Context) Source {
	if rates, ok := s.fetchCoinGecko(ctx); ok {
		s.adopt(rates, SourceCoinGecko)
		return SourceCoinGecko
	}
	if rates, ok := s.fetchExchangerateHost(ctx); ok {
		s.adopt(rates, SourceExchangerateHost)
		return SourceExchangerateHost
	}
	// Neither network source succeeded. Keep current snapshot.
	s.mu.RLock()
	cur := s.snap.Source
	s.mu.RUnlock()
	return cur
}

// adopt updates the snapshot atomically.
func (s *Service) adopt(rates map[string]float64, src Source) {
	if rates == nil {
		return
	}
	// Ensure USDT and USD always have entries; CoinGecko returns USD but
	// not USDT (it is the base by definition).
	if _, ok := rates["USDT"]; !ok {
		rates["USDT"] = 1.0
	}
	s.mu.Lock()
	s.snap = Snapshot{
		Base:      "USDT",
		Rates:     rates,
		Source:    src,
		FetchedAt: time.Now(),
		Overrides: copyMap(s.overrides),
	}
	s.mu.Unlock()
	_ = s.saveCache()
}

// fetchCoinGecko queries CoinGecko's simple/price for USDT vs all
// SupportedCurrencies. Returns (rates, ok). ok=false on any error.
func (s *Service) fetchCoinGecko(ctx context.Context) (map[string]float64, bool) {
	// Build the comma list (excluding USDT which is the base).
	wanted := make([]string, 0, len(SupportedCurrencies))
	for _, c := range SupportedCurrencies {
		if c == "USDT" {
			continue
		}
		wanted = append(wanted, strings.ToLower(c))
	}
	url := "https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=" +
		strings.Join(wanted, ",")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
	if err != nil {
		return nil, false
	}
	var parsed map[string]map[string]float64
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false
	}
	tether, ok := parsed["tether"]
	if !ok {
		return nil, false
	}
	out := make(map[string]float64, len(tether)+1)
	for k, v := range tether {
		out[strings.ToUpper(k)] = v
	}
	out["USDT"] = 1.0
	return out, true
}

// fetchExchangerateHost is the backup source. It exposes a free
// no-key /latest endpoint with USD as the typical base, so we use
// base=USD then chain USDT≈USD=1:1.
func (s *Service) fetchExchangerateHost(ctx context.Context) (map[string]float64, bool) {
	// Endpoint: https://api.exchangerate.host/latest?base=USD&symbols=CNY,EUR,JPY,...
	wanted := make([]string, 0, len(SupportedCurrencies))
	for _, c := range SupportedCurrencies {
		if c == "USDT" || c == "USD" {
			continue
		}
		wanted = append(wanted, c)
	}
	url := "https://api.exchangerate.host/latest?base=USD&symbols=" + strings.Join(wanted, ",")
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
	if err != nil {
		return nil, false
	}
	var parsed struct {
		Success bool               `json:"success"`
		Rates   map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false
	}
	out := make(map[string]float64, len(parsed.Rates)+2)
	for k, v := range parsed.Rates {
		out[strings.ToUpper(k)] = v
	}
	out["USDT"] = 1.0
	out["USD"] = 1.0
	return out, true
}

// ── disk cache ──────────────────────────────────────────────────────────────

type cacheFile struct {
	Snap      Snapshot           `json:"snap"`
	Overrides map[string]float64 `json:"overrides"`
}

func (s *Service) saveCache() error {
	if s.cacheFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.cacheFile), 0o700); err != nil {
		return err
	}
	s.mu.RLock()
	c := cacheFile{Snap: s.snap, Overrides: copyMap(s.overrides)}
	s.mu.RUnlock()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.cacheFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.cacheFile)
}

func (s *Service) loadCache() error {
	if s.cacheFile == "" {
		return nil
	}
	data, err := os.ReadFile(s.cacheFile)
	if err != nil {
		return err
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Mark loaded snapshot as disk-cache provenance so callers know it
	// is stale until next Refresh succeeds.
	c.Snap.Source = SourceDiskCache
	s.snap = c.Snap
	if c.Overrides != nil {
		s.overrides = c.Overrides
	}
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func copyMap(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// FormatMoney formats an amount of USDT in a target currency with two
// decimals plus an ISO 4217 / Cryptocurrency symbol. Returns a string
// like "¥7.18", "$1.00", "1.00 USDT". Unknown currencies fall through
// to the bare numeric.
func FormatMoney(usdt float64, currency string, rate float64) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	amount := usdt * rate
	switch currency {
	case "USDT":
		return fmt.Sprintf("%.2f USDT", amount)
	case "USD":
		return fmt.Sprintf("$%.2f", amount)
	case "CNY":
		return fmt.Sprintf("¥%.2f", amount)
	case "EUR":
		return fmt.Sprintf("€%.2f", amount)
	case "JPY":
		return fmt.Sprintf("¥%.0f", amount) // JPY no decimals
	case "GBP":
		return fmt.Sprintf("£%.2f", amount)
	case "KRW":
		return fmt.Sprintf("₩%.0f", amount)
	case "HKD":
		return fmt.Sprintf("HK$%.2f", amount)
	case "TWD":
		return fmt.Sprintf("NT$%.2f", amount)
	default:
		return fmt.Sprintf("%.2f %s", amount, currency)
	}
}
