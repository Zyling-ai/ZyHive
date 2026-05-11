// Package metrics exposes aiteam KPIs in Prometheus text exposition
// format without pulling in the prometheus client_golang dependency.
//
// 5 metrics (P3-S2 v0):
//   * aiteam_wallet_balance_usdt{agent_id=...}        gauge
//   * aiteam_guard_panic_total{reason=...}            counter
//   * aiteam_payroll_runs_total{outcome=...}          counter
//   * aiteam_judge_score_avg_7d{agent_id=...}         gauge
//   * aiteam_revenue_incoming_usdt_total              counter
//
// All metrics live in this single in-memory registry. Concurrent-safe
// via sync.RWMutex. The Handler() / Format() entry points produce
// Prometheus text format compatible with Grafana / VictoriaMetrics /
// Mimir / etc.
//
// Why no client_golang: it's a ~3MB dependency tree. For 5 metrics we
// can hand-roll exposition + counter/gauge in <200 lines and stay
// CGO_ENABLED=0 single-binary.
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry holds the in-memory metric state. Construct via New().
// Safe for concurrent use.
type Registry struct {
	mu sync.RWMutex

	// counters keyed by metric name → label set fingerprint → value
	counters map[string]map[string]float64
	// gauges same shape
	gauges map[string]map[string]float64
	// labelOrder preserves the canonical order of label keys per series
	// for stable text output (helps grep + diff).
	labelOrder map[string][]string
}

// New constructs an empty Registry.
func New() *Registry {
	return &Registry{
		counters:   map[string]map[string]float64{},
		gauges:     map[string]map[string]float64{},
		labelOrder: map[string][]string{},
	}
}

// IncCounter adds delta to the named counter with the given labels.
// Negative delta is rejected (counters are monotonic). Nil labels OK.
func (r *Registry) IncCounter(name string, labels map[string]string, delta float64) {
	if r == nil || delta < 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.counters[name]; !ok {
		r.counters[name] = map[string]float64{}
	}
	fp := fingerprint(labels)
	r.counters[name][fp] += delta
	if _, ok := r.labelOrder[fp]; !ok {
		r.labelOrder[fp] = sortedKeys(labels)
	}
}

// SetGauge sets the named gauge with the given labels to value.
func (r *Registry) SetGauge(name string, labels map[string]string, value float64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.gauges[name]; !ok {
		r.gauges[name] = map[string]float64{}
	}
	fp := fingerprint(labels)
	r.gauges[name][fp] = value
	if _, ok := r.labelOrder[fp]; !ok {
		r.labelOrder[fp] = sortedKeys(labels)
	}
}

// DeleteGauge removes a gauge series. Used when an agent is deleted
// so stale balance series don't linger. Counters never delete (Prom
// convention).
func (r *Registry) DeleteGauge(name string, labels map[string]string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.gauges[name]; ok {
		delete(m, fingerprint(labels))
	}
}

// Format renders the entire registry in Prometheus text format.
// Series within a metric are sorted by fingerprint for stable output.
func (r *Registry) Format() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder

	// Sort metric names so output is deterministic
	names := make([]string, 0, len(r.counters)+len(r.gauges))
	for n := range r.counters {
		names = append(names, n)
	}
	for n := range r.gauges {
		names = append(names, n)
	}
	sort.Strings(names)
	seen := map[string]bool{}

	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true

		if series, ok := r.counters[name]; ok {
			fmt.Fprintf(&sb, "# HELP %s aiteam counter\n", name)
			fmt.Fprintf(&sb, "# TYPE %s counter\n", name)
			r.writeSeriesLocked(&sb, name, series)
		}
		if series, ok := r.gauges[name]; ok {
			fmt.Fprintf(&sb, "# HELP %s aiteam gauge\n", name)
			fmt.Fprintf(&sb, "# TYPE %s gauge\n", name)
			r.writeSeriesLocked(&sb, name, series)
		}
	}
	return sb.String()
}

// writeSeriesLocked emits one metric family. Caller holds r.mu.
func (r *Registry) writeSeriesLocked(sb *strings.Builder, name string, series map[string]float64) {
	// Sort fingerprints for deterministic output.
	fps := make([]string, 0, len(series))
	for fp := range series {
		fps = append(fps, fp)
	}
	sort.Strings(fps)
	for _, fp := range fps {
		v := series[fp]
		labels := decodeFingerprint(fp)
		if len(labels) == 0 {
			fmt.Fprintf(sb, "%s %s\n", name, formatFloat(v))
			continue
		}
		fmt.Fprintf(sb, "%s{%s} %s\n", name, formatLabels(labels), formatFloat(v))
	}
}

// fingerprint produces a stable cache key from a label set. We use the
// label string itself so decodeFingerprint can recover the labels for
// output without round-tripping through a separate index.
//
// Format: "k1=v1\x00k2=v2" with keys sorted alphabetically. NUL
// separator avoids the rare case where a value contains '='.
func fingerprint(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := sortedKeys(labels)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(0)
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(labels[k])
	}
	return sb.String()
}

func decodeFingerprint(fp string) map[string]string {
	if fp == "" {
		return nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(fp, "\x00") {
		eq := strings.IndexByte(pair, '=')
		if eq < 0 {
			continue
		}
		out[pair[:eq]] = pair[eq+1:]
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// formatLabels emits Prometheus-format label string with proper
// escaping of backslashes, double-quotes, and newlines per spec.
func formatLabels(labels map[string]string) string {
	keys := sortedKeys(labels)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+`="`+escapeLabelValue(labels[k])+`"`)
	}
	return strings.Join(pairs, ",")
}

func escapeLabelValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return v
}

// formatFloat renders a float64 in a way Prometheus accepts. We avoid
// scientific notation for small numbers since some scrapers don't like
// "1e-05" and we expect mostly readable USDT amounts.
func formatFloat(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.6f", v)
}

// Metric name constants. Use these everywhere in aiteam to avoid typos.
const (
	NameWalletBalance  = "aiteam_wallet_balance_usdt"
	NameGuardPanic     = "aiteam_guard_panic_total"
	NamePayrollRuns    = "aiteam_payroll_runs_total"
	NameJudgeScoreAvg  = "aiteam_judge_score_avg_7d"
	NameRevenueIncome  = "aiteam_revenue_incoming_usdt_total"
)
