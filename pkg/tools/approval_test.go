package tools

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

func TestBrokerRequestApproved(t *testing.T) {
	b := NewBroker(nil)
	ctx := context.Background()

	events, unsub := b.Subscribe("c1")
	defer unsub()

	go func() {
		for {
			pending := b.ListPending("")
			if len(pending) == 1 {
				_ = b.Decide(pending[0].ID, ApprovalDecision{Approved: true, By: "tester"})
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	dec, req, err := b.Request(ctx, "alice", "ses-1", "exec", json.RawMessage(`{"cmd":"ls"}`), time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if !dec.Approved || dec.By != "tester" {
		t.Errorf("decision wrong: %+v", dec)
	}
	if req.ToolName != "exec" {
		t.Errorf("req.ToolName: %s", req.ToolName)
	}

	gotReq, gotRes := false, false
	deadline := time.After(100 * time.Millisecond)
loop:
	for {
		select {
		case ev := <-events:
			if ev.Type == "approval_request" {
				gotReq = true
			}
			if ev.Type == "approval_resolved" {
				gotRes = true
			}
		case <-deadline:
			break loop
		}
	}
	if !gotReq || !gotRes {
		t.Errorf("missing events req=%v res=%v", gotReq, gotRes)
	}

	if len(b.ListPending("")) != 0 {
		t.Errorf("pending not cleared")
	}
}

func TestBrokerRequestDenied(t *testing.T) {
	b := NewBroker(nil)
	go func() {
		for {
			p := b.ListPending("")
			if len(p) == 1 {
				_ = b.Decide(p[0].ID, ApprovalDecision{Approved: false, Reason: "no go"})
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	dec, _, err := b.Request(context.Background(), "a", "s", "exec", nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Approved {
		t.Errorf("expected denied")
	}
	if dec.Reason != "no go" {
		t.Errorf("reason: %s", dec.Reason)
	}
}

func TestBrokerRequestTimeout(t *testing.T) {
	b := NewBroker(nil)
	start := time.Now()
	dec, _, err := b.Request(context.Background(), "a", "s", "exec", nil, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Approved {
		t.Errorf("expected timeout to deny")
	}
	if dec.By != "auto-timeout" {
		t.Errorf("By: %s", dec.By)
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Errorf("returned too fast")
	}
}

func TestBrokerRequestContextCancel(t *testing.T) {
	b := NewBroker(nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	dec, _, err := b.Request(ctx, "a", "s", "exec", nil, time.Second)
	if err == nil {
		t.Errorf("expected ctx err propagated")
	}
	if dec.Approved {
		t.Errorf("expected denied on cancel")
	}
	if dec.By != "auto-cancel" {
		t.Errorf("By: %s", dec.By)
	}
}

func TestBrokerDecideNotFound(t *testing.T) {
	b := NewBroker(nil)
	if err := b.Decide("nope", ApprovalDecision{Approved: true}); err == nil {
		t.Errorf("expected error on unknown id")
	}
}

func TestBrokerConcurrentRequests(t *testing.T) {
	b := NewBroker(nil)
	const N = 8
	var wg sync.WaitGroup
	results := make([]bool, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			dec, _, _ := b.Request(context.Background(), "agent", "s", "exec", json.RawMessage(`{}`), time.Second)
			results[i] = dec.Approved
		}(i)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(b.ListPending("")) == N {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if len(b.ListPending("")) != N {
		t.Fatalf("expected %d pending, got %d", N, len(b.ListPending("")))
	}

	for _, req := range b.ListPending("") {
		_ = b.Decide(req.ID, ApprovalDecision{Approved: true, By: "tester"})
	}
	wg.Wait()
	for i, ok := range results {
		if !ok {
			t.Errorf("[%d] not approved", i)
		}
	}
}

func TestBrokerAuditHookFires(t *testing.T) {
	var (
		mu     sync.Mutex
		events []string
	)
	hook := AuditHook(func(req ApprovalRequest, dec ApprovalDecision, ev string) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})
	b := NewBroker(hook)

	go func() {
		for {
			p := b.ListPending("")
			if len(p) == 1 {
				_ = b.Decide(p[0].ID, ApprovalDecision{Approved: true})
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	_, _, _ = b.Request(context.Background(), "a", "s", "exec", nil, time.Second)

	go func() {
		for {
			p := b.ListPending("")
			if len(p) == 1 {
				_ = b.Decide(p[0].ID, ApprovalDecision{Approved: false, Reason: "nope"})
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	_, _, _ = b.Request(context.Background(), "a", "s", "exec", nil, time.Second)

	_, _, _ = b.Request(context.Background(), "a", "s", "exec", nil, 30*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 3 {
		t.Errorf("expected 3 audit events, got %d: %v", len(events), events)
	}
	want := []string{"approval_approved", "approval_denied", "approval_expired"}
	for i, w := range want {
		if i < len(events) && events[i] != w {
			t.Errorf("event[%d]: got %s want %s", i, events[i], w)
		}
	}
}

func TestBrokerSubscribeUnsub(t *testing.T) {
	b := NewBroker(nil)
	ch, unsub := b.Subscribe("x")
	unsub()
	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("expected closed channel")
		}
	default:
		t.Errorf("expected immediate read")
	}
	unsub() // safe to call again
}

// ── Registry integration ──────────────────────────────────────────────────

// registerEcho is a test helper: adds an "echo" tool that records whether it ran.
func registerEcho(r *Registry, called *bool) {
	r.register(llm.ToolDef{Name: "echo"}, func(ctx context.Context, in json.RawMessage) (string, error) {
		*called = true
		return "hello", nil
	})
}

func TestRegistryExecuteAskPolicyApproved(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, dir, "agent1")
	var called bool
	registerEcho(r, &called)

	b := NewBroker(nil)
	r.WithApprovalBroker(b, []string{"echo"}, time.Second)

	go func() {
		for {
			p := b.ListPending("")
			if len(p) == 1 {
				_ = b.Decide(p[0].ID, ApprovalDecision{Approved: true})
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	res, err := r.Execute(context.Background(), "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !called {
		t.Fatalf("handler should run after approval")
	}
	if res != "hello" {
		t.Errorf("res: %s", res)
	}
}

func TestRegistryExecuteAskPolicyDenied(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, dir, "agent1")
	var called bool
	registerEcho(r, &called)

	b := NewBroker(nil)
	r.WithApprovalBroker(b, []string{"echo"}, time.Second)

	go func() {
		for {
			p := b.ListPending("")
			if len(p) == 1 {
				_ = b.Decide(p[0].ID, ApprovalDecision{Approved: false, Reason: "拒绝"})
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	res, err := r.Execute(context.Background(), "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute should not return error on user-denied: %v", err)
	}
	if called {
		t.Errorf("handler must NOT run after denial")
	}
	if res == "" || res[0] != 0xe2 { // starts with ⛔ ?
		t.Logf("got %q", res)
	}
}

func TestRegistryExecuteAskPolicyTimeout(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, dir, "agent1")
	var called bool
	registerEcho(r, &called)

	b := NewBroker(nil)
	r.WithApprovalBroker(b, []string{"echo"}, 30*time.Millisecond)

	res, err := r.Execute(context.Background(), "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if called {
		t.Errorf("handler must NOT run on timeout")
	}
	if res == "" {
		t.Errorf("expected polite deny message")
	}
}

// Ensure tools not on Ask list bypass the broker entirely.
func TestRegistryExecuteAskPolicyNotInList(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, dir, "agent1")
	var called bool
	registerEcho(r, &called)

	b := NewBroker(nil)
	r.WithApprovalBroker(b, []string{"other_tool"}, time.Second)

	res, err := r.Execute(context.Background(), "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !called {
		t.Fatalf("handler should have run without approval")
	}
	if res != "hello" {
		t.Errorf("res: %s", res)
	}
	if len(b.ListPending("")) != 0 {
		t.Errorf("no pending should exist")
	}
}
