// Package netguard provides HTTP clients that reject private and local targets.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var ErrBlocked = errors.New("outbound target is blocked")

type Policy struct {
	exactLoopback *endpoint
}

type endpoint struct {
	scheme string
	host   string
	port   string
}

func PublicOnlyPolicy() Policy {
	return Policy{}
}

// ExactLoopbackPolicy allows only the exact loopback origin configured by rawURL.
// It is intended for explicit local providers such as Ollama, not arbitrary LAN hosts.
func ExactLoopbackPolicy(rawURL string) (Policy, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Policy{}, fmt.Errorf("invalid loopback endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Policy{}, fmt.Errorf("%w: loopback endpoint must use http or https", ErrBlocked)
	}
	if parsed.Hostname() == "" || parsed.User != nil {
		return Policy{}, fmt.Errorf("%w: loopback endpoint host is missing or contains credentials", ErrBlocked)
	}
	host := canonicalHost(parsed.Hostname())
	if host != "localhost" {
		addr, parseErr := netip.ParseAddr(host)
		if parseErr != nil || !addr.Unmap().IsLoopback() {
			return Policy{}, fmt.Errorf("%w: only localhost or a loopback IP is allowed", ErrBlocked)
		}
	}
	return Policy{exactLoopback: &endpoint{
		scheme: parsed.Scheme,
		host:   host,
		port:   effectivePort(parsed),
	}}, nil
}

type ipResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type defaultResolver struct {
	resolver *net.Resolver
}

func (r defaultResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return r.resolver.LookupNetIP(ctx, network, host)
}

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("fc00::/7"),
}

func ValidateURL(ctx context.Context, rawURL string) error {
	return ValidateURLWithPolicy(ctx, rawURL, PublicOnlyPolicy())
}

func ValidateURLWithPolicy(ctx context.Context, rawURL string, policy Policy) error {
	return validateURLWithPolicy(ctx, rawURL, defaultResolver{resolver: net.DefaultResolver}, policy)
}

func ValidateWebSocketURL(ctx context.Context, rawURL string) error {
	return validateWebSocketURL(ctx, rawURL, defaultResolver{resolver: net.DefaultResolver})
}

func validateWebSocketURL(ctx context.Context, rawURL string, resolver ipResolver) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid WebSocket URL: %w", err)
	}
	switch parsed.Scheme {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	default:
		return fmt.Errorf("%w: only ws and wss are allowed", ErrBlocked)
	}
	return validateURLWithPolicy(ctx, parsed.String(), resolver, PublicOnlyPolicy())
}

func validateURL(ctx context.Context, rawURL string, resolver ipResolver) error {
	return validateURLWithPolicy(ctx, rawURL, resolver, PublicOnlyPolicy())
}

func validateURLWithPolicy(ctx context.Context, rawURL string, resolver ipResolver, policy Policy) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid outbound URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: only http and https are allowed", ErrBlocked)
	}
	if parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("%w: URL host is missing or contains credentials", ErrBlocked)
	}
	host := canonicalHost(parsed.Hostname())
	if policy.exactLoopback != nil {
		expected := policy.exactLoopback
		if parsed.Scheme != expected.scheme || host != expected.host || effectivePort(parsed) != expected.port {
			return fmt.Errorf("%w: URL does not match the configured loopback endpoint", ErrBlocked)
		}
		_, err = resolveLoopbackIPs(ctx, resolver, host)
		return err
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("%w: local hostnames are not allowed", ErrBlocked)
	}
	_, err = resolvePublicIPs(ctx, resolver, host)
	return err
}

func canonicalHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

func effectivePort(parsed *url.URL) string {
	if port := parsed.Port(); port != "" {
		return port
	}
	if parsed.Scheme == "https" {
		return "443"
	}
	return "80"
}

func resolveLoopbackIPs(ctx context.Context, resolver ipResolver, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		if !addr.IsLoopback() {
			return nil, fmt.Errorf("%w: address %s is not loopback", ErrBlocked, addr)
		}
		return []netip.Addr{addr}, nil
	}
	if host != "localhost" {
		return nil, fmt.Errorf("%w: loopback hostname must be localhost", ErrBlocked)
	}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve loopback host: %w", err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("resolve loopback host: no addresses")
	}
	for i, addr := range addrs {
		addr = addr.Unmap()
		if !addr.IsLoopback() {
			return nil, fmt.Errorf("%w: localhost resolved to non-loopback address %s", ErrBlocked, addr)
		}
		addrs[i] = addr
	}
	return addrs, nil
}

func resolvePublicIPs(ctx context.Context, resolver ipResolver, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		if isBlockedAddr(addr) {
			return nil, fmt.Errorf("%w: address %s is not public", ErrBlocked, addr)
		}
		return []netip.Addr{addr}, nil
	}

	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve outbound host: %w", err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("resolve outbound host: no addresses")
	}
	public := make([]netip.Addr, 0, len(addrs))
	for _, addr := range addrs {
		addr = addr.Unmap()
		if isBlockedAddr(addr) {
			return nil, fmt.Errorf("%w: %s resolved to non-public address %s", ErrBlocked, host, addr)
		}
		public = append(public, addr)
	}
	return public, nil
}

func isBlockedAddr(addr netip.Addr) bool {
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() ||
		addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func NewSafeClient(timeout time.Duration) *http.Client {
	return NewClient(timeout, PublicOnlyPolicy())
}

func newSafeClient(timeout time.Duration, resolver ipResolver) *http.Client {
	return newPolicyClient(timeout, resolver, PublicOnlyPolicy())
}

func NewExactLoopbackClient(timeout time.Duration, rawURL string) (*http.Client, error) {
	policy, err := ExactLoopbackPolicy(rawURL)
	if err != nil {
		return nil, err
	}
	return NewClient(timeout, policy), nil
}

func NewClient(timeout time.Duration, policy Policy) *http.Client {
	return newPolicyClient(timeout, defaultResolver{resolver: net.DefaultResolver}, policy)
}

func newPolicyClient(timeout time.Duration, resolver ipResolver, policy Policy) *http.Client {
	transport := newPolicyTransport(resolver, policy)
	return &http.Client{
		Timeout: timeout,
		Transport: validatingRoundTripper{
			next:     transport,
			resolver: resolver,
			policy:   policy,
		},
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if err := validateURLWithPolicy(req.Context(), req.URL.String(), resolver, policy); err != nil {
				return err
			}
			return nil
		},
	}
}

type validatingRoundTripper struct {
	next     http.RoundTripper
	resolver ipResolver
	policy   Policy
}

func (t validatingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := validateURLWithPolicy(req.Context(), req.URL.String(), t.resolver, t.policy); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(req)
}

// NewSafeTransport returns an HTTP transport that resolves and directly dials
// only public addresses. Environment proxies are deliberately disabled.
func NewSafeTransport() *http.Transport {
	return NewTransport(PublicOnlyPolicy())
}

func newSafeTransport(resolver ipResolver) *http.Transport {
	return newPolicyTransport(resolver, PublicOnlyPolicy())
}

func NewTransport(policy Policy) *http.Transport {
	return newPolicyTransport(defaultResolver{resolver: net.DefaultResolver}, policy)
}

func newPolicyTransport(resolver ipResolver, policy Policy) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = newPolicyDialContext(resolver, policy)
	transport.ResponseHeaderTimeout = 30 * time.Second
	return transport
}

// DialContext resolves address and dials the validated public IP directly.
func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return newPolicyDialContext(defaultResolver{resolver: net.DefaultResolver}, PublicOnlyPolicy())(ctx, network, address)
}

func newSafeDialContext(resolver ipResolver) func(context.Context, string, string) (net.Conn, error) {
	return newPolicyDialContext(resolver, PublicOnlyPolicy())
}

func newPolicyDialContext(resolver ipResolver, policy Policy) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid outbound address: %w", err)
		}
		var addrs []netip.Addr
		if policy.exactLoopback != nil {
			expected := policy.exactLoopback
			if canonicalHost(host) != expected.host || port != expected.port {
				return nil, fmt.Errorf("%w: dial target does not match the configured loopback endpoint", ErrBlocked)
			}
			addrs, err = resolveLoopbackIPs(ctx, resolver, canonicalHost(host))
		} else {
			addrs, err = resolvePublicIPs(ctx, resolver, host)
		}
		if err != nil {
			return nil, err
		}
		var dialErr error
		for _, addr := range addrs {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErr = err
		}
		return nil, fmt.Errorf("dial outbound host: %w", dialErr)
	}
}
