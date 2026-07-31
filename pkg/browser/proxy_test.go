package browser

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/gorilla/websocket"
)

func openTestProxy(t *testing.T) *safeProxy {
	t.Helper()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: time.Second}
	proxy, err := startSafeProxy(
		transport,
		dialer.DialContext,
		func(context.Context, string) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(proxy.Close)
	return proxy
}

func clientThroughProxy(t *testing.T, proxy *safeProxy, insecureTLS bool) *http.Client {
	t.Helper()
	proxyURL, err := url.Parse(proxy.URL())
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureTLS}, // test server only
		},
	}
}

func TestSafeProxyForwardsHTTPWithValidatedDialer(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()

	proxy := openTestProxy(t)
	resp, err := clientThroughProxy(t, proxy, false).Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
}

func TestSafeProxyBlocksPrivateHTTPWithoutReachingTarget(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	proxy, err := newSafeProxy()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	resp, err := clientThroughProxy(t, proxy, false).Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", resp.StatusCode)
	}
	if hits.Load() != 0 {
		t.Fatal("blocked private target was reached")
	}
}

func TestSafeProxyBlocksPrivateCONNECTWithoutReachingTarget(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	proxy, err := newSafeProxy()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	_, err = clientThroughProxy(t, proxy, true).Get(target.URL)
	if err == nil {
		t.Fatal("private CONNECT should fail")
	}
	if hits.Load() != 0 {
		t.Fatal("blocked private CONNECT reached target")
	}
}

func TestSafeProxyTunnelsHTTPSWithValidatedDialer(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "secure")
	}))
	defer target.Close()
	proxy := openTestProxy(t)

	resp, err := clientThroughProxy(t, proxy, true).Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "secure" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
}

func TestSafeProxyForwardsWebSocketWithValidatedDialer(t *testing.T) {
	upgrader := websocket.Upgrader{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		messageType, message, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(messageType, message)
		}
	}))
	defer target.Close()
	proxy := openTestProxy(t)
	proxyURL, _ := url.Parse(proxy.URL())
	dialer := websocket.Dialer{
		Proxy:            http.ProxyURL(proxyURL),
		HandshakeTimeout: 2 * time.Second,
	}
	conn, _, err := dialer.Dial(strings.Replace(target.URL, "http://", "ws://", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("echo")); err != nil {
		t.Fatal(err)
	}
	_, message, err := conn.ReadMessage()
	if err != nil || string(message) != "echo" {
		t.Fatalf("message=%q err=%v", message, err)
	}
}

func TestSafeProxyCloseStopsListener(t *testing.T) {
	proxy := openTestProxy(t)
	address := proxy.listener.Addr().String()
	proxy.Close()
	conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("proxy listener still accepts connections after Close")
	}
}

func TestManagerCloseStopsSafetyProxy(t *testing.T) {
	proxy := openTestProxy(t)
	address := proxy.listener.Addr().String()
	manager := NewManager(t.TempDir())
	manager.proxy = proxy
	manager.Close()
	if manager.proxy != nil {
		t.Fatal("manager retained closed safety proxy")
	}
	conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("manager Close left safety proxy listening")
	}
}

func TestBrowserLauncherForcesAllTrafficThroughSafetyProxy(t *testing.T) {
	launcher := newBrowserLauncher("/tmp/chromium", "http://127.0.0.1:43210")
	if got := launcher.Get(flags.ProxyServer); got != "http://127.0.0.1:43210" {
		t.Fatalf("proxy-server=%q", got)
	}
	expected := map[flags.Flag]string{
		"proxy-bypass-list":               "<-loopback>",
		"disable-quic":                    "",
		"dns-prefetch-disable":            "",
		"force-webrtc-ip-handling-policy": "disable_non_proxied_udp",
	}
	for flag, want := range expected {
		if got := launcher.Get(flag); got != want {
			t.Errorf("%s=%q, want %q", flag, got, want)
		}
	}
}

func TestBrowserSafetyProxyBlocksPageSubresource(t *testing.T) {
	if os.Getenv("ZYHIVE_BROWSER_E2E") != "1" {
		t.Skip("set ZYHIVE_BROWSER_E2E=1 to run Chromium integration test")
	}
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	manager := NewManager(t.TempDir())
	defer manager.Close()
	page := `<script>fetch("` + target.URL + `/private")</script>`
	if _, err := manager.Navigate("test-agent", "data:text/html,"+url.PathEscape(page), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatal("Chromium bypassed safety proxy for a private subresource")
	}
}
