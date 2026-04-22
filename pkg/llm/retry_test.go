package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIsTransientMatches(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", errors.New("rate limit exceeded (429)"), true},
		{"too many requests", errors.New("too many requests"), true},
		{"502", errors.New("bad gateway 502"), true},
		{"503 str", errors.New("service unavailable: 503"), true},
		{"504", errors.New("504 gateway timeout"), true},
		{"connection reset", errors.New("read tcp: connection reset by peer"), true},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"eof mid-stream", errors.New("unexpected EOF"), true},
		{"i/o timeout", errors.New("net/http: TLS handshake timeout — i/o timeout"), true},
		{"tls handshake", errors.New("tls handshake failure"), true},
		{"401 auth", errors.New("401 unauthorized"), false},
		{"400 bad request", errors.New("400 bad request: malformed body"), false},
		{"context filter", errors.New("content filter triggered: policy violation"), false},
		{"context length", errors.New("context_length_exceeded: prompt too long"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsTransient(tc.err)
			if got != tc.want {
				t.Fatalf("IsTransient(%q) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsAuthFailure(t *testing.T) {
	if !IsAuthFailure(errors.New("401 Unauthorized")) {
		t.Fatal("401 should be auth failure")
	}
	if !IsAuthFailure(errors.New("Invalid API key provided")) {
		t.Fatal("invalid api key should be auth failure")
	}
	if IsAuthFailure(errors.New("429 too many requests")) {
		t.Fatal("429 is not auth failure")
	}
	if IsAuthFailure(nil) {
		t.Fatal("nil is not auth failure")
	}
}

// mockClient is a Client stub for testing retry behavior.
type mockClient struct {
	calls   int
	errFn   func(attempt int) error  // returns error for attempt N (0-indexed)
	channel chan StreamEvent
}

func (m *mockClient) Stream(_ context.Context, _ *ChatRequest) (<-chan StreamEvent, error) {
	n := m.calls
	m.calls++
	if m.errFn != nil {
		if err := m.errFn(n); err != nil {
			return nil, err
		}
	}
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: EventStop}
	close(ch)
	return ch, nil
}

func TestRetryClient_RetriesTransient(t *testing.T) {
	mock := &mockClient{
		errFn: func(n int) error {
			if n < 2 {
				return errors.New("502 bad gateway")
			}
			return nil
		},
	}
	// Fast schedule to keep test speedy.
	client := WithRetrySchedule(mock, []time.Duration{1 * time.Millisecond, 1 * time.Millisecond, 1 * time.Millisecond})
	ctx := context.Background()
	ch, err := client.Stream(ctx, &ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	if mock.calls != 3 {
		t.Fatalf("expected 3 calls (2 fails + 1 success), got %d", mock.calls)
	}
}

func TestRetryClient_DoesNotRetryNonTransient(t *testing.T) {
	mock := &mockClient{
		errFn: func(_ int) error { return errors.New("401 unauthorized") },
	}
	client := WithRetrySchedule(mock, []time.Duration{1 * time.Millisecond})
	_, err := client.Stream(context.Background(), &ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected auth error not to retry")
	}
	if mock.calls != 1 {
		t.Fatalf("non-transient error should not retry; got %d calls", mock.calls)
	}
}

func TestRetryClient_GivesUpAfterMaxAttempts(t *testing.T) {
	mock := &mockClient{
		errFn: func(_ int) error { return errors.New("503 service unavailable") },
	}
	client := WithRetrySchedule(mock, []time.Duration{1 * time.Millisecond, 1 * time.Millisecond})
	_, err := client.Stream(context.Background(), &ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected final error after all retries exhausted")
	}
	// Initial call + 2 retries = 3 total
	if mock.calls != 3 {
		t.Fatalf("expected 3 total calls, got %d", mock.calls)
	}
}

func TestRetryClient_RespectsContextCancellation(t *testing.T) {
	mock := &mockClient{
		errFn: func(_ int) error { return errors.New("503 service unavailable") },
	}
	client := WithRetrySchedule(mock, []time.Duration{100 * time.Millisecond, 100 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := client.Stream(ctx, &ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	if mock.calls < 1 {
		t.Fatal("expected at least one call before cancel")
	}
}
