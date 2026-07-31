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
	return validateURL(ctx, rawURL, defaultResolver{resolver: net.DefaultResolver})
}

func validateURL(ctx context.Context, rawURL string, resolver ipResolver) error {
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
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("%w: local hostnames are not allowed", ErrBlocked)
	}
	_, err = resolvePublicIPs(ctx, resolver, host)
	return err
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
	return newSafeClient(timeout, defaultResolver{resolver: net.DefaultResolver})
}

func newSafeClient(timeout time.Duration, resolver ipResolver) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid outbound address: %w", err)
		}
		addrs, err := resolvePublicIPs(ctx, resolver, host)
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

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if err := validateURL(req.Context(), req.URL.String(), resolver); err != nil {
				return err
			}
			return nil
		},
	}
}
