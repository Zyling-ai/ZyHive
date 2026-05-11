package metrics

import (
	"strings"
	"sync"
	"testing"
)

// High cardinality — Prometheus wisdom is "don't use labels for unbounded
// values", but our wallet gauge uses agent_id which could be hundreds.
// Verify the registry doesn't explode at 1000 unique series.
func Test_AITeam_S8_Edge_HighCardinality(t *testing.T) {
	r := New()
	for i := 0; i < 1000; i++ {
		r.SetGauge(NameWalletBalance, map[string]string{"agent_id": "agent-" + string(rune('a'+(i%26))) + "-" + string(rune('A'+(i/26%26))) + "-" + string(rune('0'+(i%10)))}, float64(i))
	}
	out := r.Format()
	if len(out) < 1000 {
		t.Fatal("output suspiciously short")
	}
	// Count lines
	lines := strings.Count(out, "\n")
	if lines < 100 {
		t.Fatalf("expected hundreds of lines, got %d", lines)
	}
}

// Label values with special chars (newlines, quotes, backslash).
func Test_AITeam_S8_Edge_LabelSpecialChars(t *testing.T) {
	r := New()
	r.SetGauge("test_g",
		map[string]string{"weird": "line1\nline2\"quote\\back"},
		1.0)
	out := r.Format()
	// Verify escaping
	if !strings.Contains(out, `line1\nline2\"quote\\back`) {
		t.Fatalf("escape wrong: %s", out)
	}
}

// Concurrent reads + writes — race detector should catch any issue.
func Test_AITeam_S8_Edge_ConcurrentReadWrite(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			r.IncCounter("c", map[string]string{"k": "v"}, 1)
		}()
		go func() {
			defer wg.Done()
			r.SetGauge("g", map[string]string{"k": "v"}, 5.0)
		}()
		go func() {
			defer wg.Done()
			_ = r.Format()
		}()
	}
	wg.Wait()
}

// Empty label value.
func Test_AITeam_S8_Edge_EmptyLabelValue(t *testing.T) {
	r := New()
	r.SetGauge("test_g", map[string]string{"agent": ""}, 1.0)
	out := r.Format()
	if !strings.Contains(out, `test_g{agent=""} 1`) {
		t.Fatalf("empty value: %s", out)
	}
}

// Very small float (precision boundary).
func Test_AITeam_S8_Edge_TinyFloat(t *testing.T) {
	r := New()
	r.SetGauge("t1", nil, 0.000001) // 1e-6
	out := r.Format()
	if !strings.Contains(out, "0.000001") {
		t.Fatalf("tiny float: %s", out)
	}
}

// Extreme float.
func Test_AITeam_S8_Edge_HugeFloat(t *testing.T) {
	r := New()
	r.SetGauge("t1", nil, 1e15)
	out := r.Format()
	// Verify it didn't break (whatever the format is)
	if !strings.Contains(out, "t1 ") {
		t.Fatalf("huge float not emitted: %s", out)
	}
}
