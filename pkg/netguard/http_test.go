package netguard

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"testing"
	"time"
)

type staticResolver map[string][]netip.Addr

func (r staticResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	addrs, ok := r[host]
	if !ok {
		return nil, errors.New("host not found")
	}
	return addrs, nil
}

type sequenceResolver struct {
	results [][]netip.Addr
	index   int
}

func (r *sequenceResolver) LookupNetIP(_ context.Context, _, _ string) ([]netip.Addr, error) {
	if r.index >= len(r.results) {
		return nil, errors.New("no more DNS results")
	}
	result := r.results[r.index]
	r.index++
	return result, nil
}

func TestValidateURLRejectsUnsafeTargets(t *testing.T) {
	resolver := staticResolver{
		"private.example": {netip.MustParseAddr("10.0.0.8")},
		"mixed.example": {
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("169.254.169.254"),
		},
	}
	tests := []string{
		"file:///etc/passwd",
		"http://localhost/admin",
		"http://service.internal/admin",
		"http://127.0.0.1/admin",
		"http://[::1]/admin",
		"http://169.254.169.254/latest/meta-data",
		"http://100.64.0.1/",
		"http://private.example/",
		"http://mixed.example/",
		"http://user:password@example.com/",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			if err := validateURL(context.Background(), target, resolver); !errors.Is(err, ErrBlocked) {
				t.Fatalf("expected ErrBlocked, got %v", err)
			}
		})
	}
}

func TestValidateURLAllowsOnlyPublicResolution(t *testing.T) {
	resolver := staticResolver{
		"public.example": {
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("2606:2800:220:1:248:1893:25c8:1946"),
		},
	}
	if err := validateURL(context.Background(), "https://public.example/path", resolver); err != nil {
		t.Fatalf("public URL rejected: %v", err)
	}
}

func TestValidateWebSocketURLUsesPublicOnlyPolicy(t *testing.T) {
	resolver := staticResolver{
		"public.example":  {netip.MustParseAddr("93.184.216.34")},
		"private.example": {netip.MustParseAddr("10.0.0.8")},
	}
	if err := validateWebSocketURL(context.Background(), "wss://public.example/events", resolver); err != nil {
		t.Fatalf("public WebSocket rejected: %v", err)
	}
	for _, target := range []string{
		"wss://private.example/events",
		"ws://localhost/events",
		"https://public.example/events",
		"wss://user:pass@public.example/events",
	} {
		if err := validateWebSocketURL(context.Background(), target, resolver); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: expected ErrBlocked, got %v", target, err)
		}
	}
}

func TestSafeClientRejectsPrivateRedirect(t *testing.T) {
	resolver := staticResolver{"private.example": {netip.MustParseAddr("192.168.1.5")}}
	client := newSafeClient(time.Second, resolver)
	req, _ := http.NewRequest(http.MethodGet, "http://private.example/admin", nil)
	if err := client.CheckRedirect(req, nil); !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected private redirect rejection, got %v", err)
	}
}

func TestExactLoopbackPolicyAllowsOnlyConfiguredOrigin(t *testing.T) {
	policy, err := ExactLoopbackPolicy("http://localhost:11434/v1")
	if err != nil {
		t.Fatal(err)
	}
	resolver := staticResolver{
		"localhost": {netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("::1")},
	}
	if err := validateURLWithPolicy(
		context.Background(),
		"http://localhost:11434/v1/models",
		resolver,
		policy,
	); err != nil {
		t.Fatalf("configured loopback endpoint rejected: %v", err)
	}
	for _, target := range []string{
		"http://localhost:11435/v1/models",
		"https://localhost:11434/v1/models",
		"http://127.0.0.1:11434/v1/models",
		"http://192.168.1.20:11434/v1/models",
		"https://example.com/v1/models",
	} {
		if err := validateURLWithPolicy(context.Background(), target, resolver, policy); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: expected ErrBlocked, got %v", target, err)
		}
	}
}

func TestExactLoopbackPolicyRejectsLANConfiguration(t *testing.T) {
	for _, target := range []string{
		"http://192.168.1.20:11434/v1",
		"http://ollama.local:11434/v1",
		"http://user:pass@localhost:11434/v1",
	} {
		if _, err := ExactLoopbackPolicy(target); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: expected ErrBlocked, got %v", target, err)
		}
	}
}

func TestExactLoopbackClientRejectsCrossOriginRedirect(t *testing.T) {
	policy, err := ExactLoopbackPolicy("http://localhost:11434/v1")
	if err != nil {
		t.Fatal(err)
	}
	resolver := staticResolver{
		"localhost": {netip.MustParseAddr("127.0.0.1")},
	}
	client := newPolicyClient(time.Second, resolver, policy)
	req, _ := http.NewRequest(http.MethodGet, "http://localhost:11435/private", nil)
	if err := client.CheckRedirect(req, nil); !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected cross-origin redirect rejection, got %v", err)
	}
}

func TestSafeTransportDisablesProxyAndBlocksPrivateDial(t *testing.T) {
	resolver := staticResolver{"private.example": {netip.MustParseAddr("10.0.0.5")}}
	transport := newSafeTransport(resolver)
	if transport.Proxy != nil {
		t.Fatal("safe transport must ignore environment proxies")
	}
	_, err := transport.DialContext(context.Background(), "tcp", "private.example:80")
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected private dial rejection, got %v", err)
	}
}

func TestExactLoopbackTransportDisablesProxyAndRejectsWrongPort(t *testing.T) {
	policy, err := ExactLoopbackPolicy("http://localhost:11434/v1")
	if err != nil {
		t.Fatal(err)
	}
	resolver := staticResolver{
		"localhost": {netip.MustParseAddr("127.0.0.1")},
	}
	transport := newPolicyTransport(resolver, policy)
	if transport.Proxy != nil {
		t.Fatal("loopback transport must ignore environment proxies")
	}
	_, err = transport.DialContext(context.Background(), "tcp", "localhost:11435")
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected wrong-port rejection, got %v", err)
	}
}

func TestSafeClientBlocksDNSRebindingBetweenValidationAndDial(t *testing.T) {
	resolver := &sequenceResolver{results: [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34")},
		{netip.MustParseAddr("127.0.0.1")},
	}}
	target := "http://rebind.example/"
	if err := validateURL(context.Background(), target, resolver); err != nil {
		t.Fatalf("first public resolution should pass: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, target, nil)
	_, err := newSafeClient(time.Second, resolver).Do(req)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected DNS rebinding block, got %v", err)
	}
}

func TestBlockedAddressClassification(t *testing.T) {
	blocked := []string{
		"0.0.0.1", "10.0.0.1", "100.64.0.1", "127.0.0.1",
		"169.254.169.254", "192.168.1.1", "198.18.0.1", "::1", "fc00::1", "fe80::1",
	}
	for _, raw := range blocked {
		if !isBlockedAddr(netip.MustParseAddr(raw)) {
			t.Errorf("%s should be blocked", raw)
		}
	}
	if isBlockedAddr(netip.MustParseAddr("93.184.216.34")) {
		t.Fatal("public address should not be blocked")
	}
}
