package judge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// fakeClient implements llm.Client for tests. The events slice is
// emitted in order; if err is non-nil, Stream returns it instead.
type fakeClient struct {
	events []llm.StreamEvent
	err    error
}

func (f *fakeClient) Stream(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan llm.StreamEvent, len(f.events)+1)
	go func() {
		defer close(ch)
		for _, ev := range f.events {
			ch <- ev
		}
	}()
	return ch, nil
}

func textDelta(s string) llm.StreamEvent {
	return llm.StreamEvent{Type: llm.EventTextDelta, Text: s}
}

func Test_AITeam_S0_LLMCallFromClient_HappyPath(t *testing.T) {
	client := &fakeClient{
		events: []llm.StreamEvent{
			textDelta(`{"completion":7`),
			textDelta(`,"quality":8`),
			textDelta(`,"communication":7,"creativity":6,"cost":8`),
			textDelta(`,"rationale":"solid"}`),
			{Type: llm.EventStop},
		},
	}
	call := LLMCallFromClient(client, "test-model", "fake-key", 0, 0)
	out, err := call("system prompt", "user prompt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "completion") || !strings.Contains(out, "rationale") {
		t.Fatalf("expected accumulated JSON, got %q", out)
	}
}

func Test_AITeam_S0_LLMCallFromClient_StreamErrorEvent(t *testing.T) {
	client := &fakeClient{
		events: []llm.StreamEvent{
			textDelta("partial..."),
			{Type: llm.EventError, Err: errors.New("oops")},
		},
	}
	call := LLMCallFromClient(client, "m", "k", 0, 0)
	_, err := call("sys", "user")
	if err == nil || !strings.Contains(err.Error(), "stream error") {
		t.Fatalf("expected stream error, got %v", err)
	}
}

func Test_AITeam_S0_LLMCallFromClient_StartError(t *testing.T) {
	client := &fakeClient{err: errors.New("network down")}
	call := LLMCallFromClient(client, "m", "k", 0, 0)
	_, err := call("sys", "user")
	if err == nil || !strings.Contains(err.Error(), "stream start") {
		t.Fatalf("expected start error, got %v", err)
	}
}

func Test_AITeam_S0_LLMCallFromClient_EmptyResponse(t *testing.T) {
	client := &fakeClient{events: []llm.StreamEvent{{Type: llm.EventStop}}}
	call := LLMCallFromClient(client, "m", "k", 0, 0)
	_, err := call("sys", "user")
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("expected empty response error, got %v", err)
	}
}

func Test_AITeam_S0_LLMCallFromClient_NilClient(t *testing.T) {
	call := LLMCallFromClient(nil, "m", "k", 0, 0)
	_, err := call("sys", "user")
	if err == nil || !strings.Contains(err.Error(), "nil client") {
		t.Fatalf("expected nil client error, got %v", err)
	}
}

func Test_AITeam_S0_LLMScorer_E2E_WithAdapter(t *testing.T) {
	// E2E proof: feed adapter into a LLMScorer, score a transcript,
	// verify the full chain works.
	client := &fakeClient{
		events: []llm.StreamEvent{
			textDelta(`{"completion":8,"quality":7,"communication":9,"creativity":6,"cost":8,"rationale":"good"}`),
			{Type: llm.EventStop},
		},
	}
	scorer := LLMScorer{
		Call:        LLMCallFromClient(client, "test", "k", 100, 5*time.Second),
		PromptGuard: nil, // tests don't need promptdef
		Fallback:    HeuristicScorer{},
	}
	sc := scorer.Score(Signals{AgentID: "alice", Period: "2026-05-10", Notes: "did good work"})
	if sc.Source != "llm" {
		t.Fatalf("expected llm source, got %q", sc.Source)
	}
	if sc.Average != 7.6 {
		t.Fatalf("avg = %v want 7.6", sc.Average)
	}
}
