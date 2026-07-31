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

func TestSafeClientRejectsPrivateRedirect(t *testing.T) {
	resolver := staticResolver{"private.example": {netip.MustParseAddr("192.168.1.5")}}
	client := newSafeClient(time.Second, resolver)
	req, _ := http.NewRequest(http.MethodGet, "http://private.example/admin", nil)
	if err := client.CheckRedirect(req, nil); !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected private redirect rejection, got %v", err)
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
