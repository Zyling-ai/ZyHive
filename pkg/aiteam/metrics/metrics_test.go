package metrics

import (
	"strings"
	"sync"
	"testing"
)

func Test_AITeam_S2_Metrics_EmptyRegistry(t *testing.T) {
	r := New()
	if out := r.Format(); out != "" {
		t.Fatalf("empty registry should emit nothing, got %q", out)
	}
}

func Test_AITeam_S2_Metrics_CounterIncrementsAndFormats(t *testing.T) {
	r := New()
	r.IncCounter(NameGuardPanic, map[string]string{"reason": "agent_daily"}, 1)
	r.IncCounter(NameGuardPanic, map[string]string{"reason": "agent_daily"}, 2)
	r.IncCounter(NameGuardPanic, map[string]string{"reason": "zero_balance"}, 1)
	out := r.Format()
	if !strings.Contains(out, `aiteam_guard_panic_total{reason="agent_daily"} 3`) {
		t.Fatalf("agent_daily count: %s", out)
	}
	if !strings.Contains(out, `aiteam_guard_panic_total{reason="zero_balance"} 1`) {
		t.Fatalf("zero_balance count: %s", out)
	}
	if !strings.Contains(out, "# TYPE aiteam_guard_panic_total counter") {
		t.Fatalf("missing TYPE: %s", out)
	}
}

func Test_AITeam_S2_Metrics_CounterRejectsNegative(t *testing.T) {
	r := New()
	r.IncCounter(NameGuardPanic, map[string]string{}, 5)
	r.IncCounter(NameGuardPanic, map[string]string{}, -3) // ignored
	if !strings.Contains(r.Format(), "aiteam_guard_panic_total 5") {
		t.Fatalf("negative inc should be ignored, got %s", r.Format())
	}
}

func Test_AITeam_S2_Metrics_GaugeOverwrites(t *testing.T) {
	r := New()
	r.SetGauge(NameWalletBalance, map[string]string{"agent_id": "alice"}, 5.0)
	r.SetGauge(NameWalletBalance, map[string]string{"agent_id": "alice"}, 7.5)
	out := r.Format()
	if !strings.Contains(out, `aiteam_wallet_balance_usdt{agent_id="alice"} 7.500000`) {
		t.Fatalf("gauge should overwrite: %s", out)
	}
	if strings.Count(out, "alice") != 1 {
		t.Fatalf("expected exactly 1 alice line: %s", out)
	}
}

func Test_AITeam_S2_Metrics_GaugeDelete(t *testing.T) {
	r := New()
	r.SetGauge(NameWalletBalance, map[string]string{"agent_id": "alice"}, 5.0)
	r.DeleteGauge(NameWalletBalance, map[string]string{"agent_id": "alice"})
	if strings.Contains(r.Format(), "alice") {
		t.Fatalf("alice should be removed from output")
	}
}

func Test_AITeam_S2_Metrics_StableSortedOutput(t *testing.T) {
	r := New()
	r.SetGauge(NameWalletBalance, map[string]string{"agent_id": "zebra"}, 1)
	r.SetGauge(NameWalletBalance, map[string]string{"agent_id": "alpha"}, 2)
	r.SetGauge(NameWalletBalance, map[string]string{"agent_id": "mango"}, 3)
	out := r.Format()
	// alpha < mango < zebra in lex order
	alphaIdx := strings.Index(out, "alpha")
	mangoIdx := strings.Index(out, "mango")
	zebraIdx := strings.Index(out, "zebra")
	if !(alphaIdx < mangoIdx && mangoIdx < zebraIdx) {
		t.Fatalf("series not sorted: a=%d m=%d z=%d\n%s", alphaIdx, mangoIdx, zebraIdx, out)
	}
}

func Test_AITeam_S2_Metrics_LabelEscaping(t *testing.T) {
	r := New()
	r.SetGauge("test_g", map[string]string{"reason": `back\slash and "quote"`}, 1)
	out := r.Format()
	if !strings.Contains(out, `back\\slash and \"quote\"`) {
		t.Fatalf("escaping wrong: %s", out)
	}
}

func Test_AITeam_S2_Metrics_NoLabelsLine(t *testing.T) {
	r := New()
	r.IncCounter(NameRevenueIncome, nil, 50)
	out := r.Format()
	if !strings.Contains(out, "aiteam_revenue_incoming_usdt_total 50\n") {
		t.Fatalf("no-label line should have no braces: %s", out)
	}
}

func Test_AITeam_S2_Metrics_FloatFormatting(t *testing.T) {
	r := New()
	r.SetGauge("t1", nil, 5)         // → "5"
	r.SetGauge("t2", nil, 0.123456)  // → "0.123456"
	r.SetGauge("t3", nil, 7.5)       // → "7.500000"
	out := r.Format()
	if !strings.Contains(out, "t1 5\n") {
		t.Errorf("integer gauge: %s", out)
	}
	if !strings.Contains(out, "t2 0.123456\n") {
		t.Errorf("fraction gauge: %s", out)
	}
	if !strings.Contains(out, "t3 7.500000\n") {
		t.Errorf("half gauge: %s", out)
	}
}

func Test_AITeam_S2_Metrics_ConcurrentSafety(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			r.IncCounter(NameGuardPanic, map[string]string{"reason": "agent_daily"}, 1)
		}(i)
		go func(i int) {
			defer wg.Done()
			r.SetGauge(NameWalletBalance, map[string]string{"agent_id": "alice"}, float64(i))
		}(i)
	}
	wg.Wait()
	out := r.Format()
	if !strings.Contains(out, "aiteam_guard_panic_total{reason=\"agent_daily\"} 100") {
		t.Fatalf("expected counter=100 after concurrent inc: %s", out)
	}
}

func Test_AITeam_S2_Metrics_NilRegistrySafe(t *testing.T) {
	var r *Registry
	r.IncCounter("x", nil, 1)
	r.SetGauge("y", nil, 1)
	r.DeleteGauge("y", nil)
	if r.Format() != "" {
		t.Fatal("nil registry should format to empty")
	}
}
