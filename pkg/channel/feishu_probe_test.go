package channel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockFeishuServer returns a *httptest.Server mimicking enough of the Feishu
// OpenAPI to drive Probe end-to-end. Each test customises specific routes via
// the routes map; unmatched routes return 404.
type mockRoute struct {
	code int    // Feishu inner code; 0 means success
	body string // JSON body to return (full envelope)
	http int    // HTTP status code (0 → 200)
}

func newMockFeishu(t *testing.T, routes map[string]mockRoute) *httptest.Server {
	t.Helper()
	mu := sync.Mutex{}
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		// Path matching: exact match preferred, else prefix.
		key := r.URL.Path
		route, ok := routes[key]
		if !ok {
			// Try prefix matching for path-param endpoints.
			for k, v := range routes {
				if strings.HasSuffix(k, "/*") && strings.HasPrefix(key, strings.TrimSuffix(k, "/*")) {
					route = v
					ok = true
					break
				}
			}
		}
		if !ok {
			http.Error(w, `{"code":99999,"msg":"unmocked path: `+key+`"}`, 404)
			return
		}
		if route.http == 0 {
			route.http = 200
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(route.http)
		_, _ = w.Write([]byte(route.body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// probeWithMockHost is a test-only helper that points probeHTTPClient at our
// mock server by overriding via the Probe() function-local code path. Because
// Probe() builds URLs from "open.feishu.cn" / "open.larksuite.com" we instead
// reach into the implementation via a host-injecting Transport.
func probeWithMock(t *testing.T, mockURL string, fn func(ctx context.Context)) {
	t.Helper()
	prev := probeHTTPClient.Transport
	// Rewrite host on every outbound request to point at mockURL.
	probeHTTPClient.Transport = &rewriteTransport{
		target: mockURL,
		base:   http.DefaultTransport,
	}
	defer func() { probeHTTPClient.Transport = prev }()
	fn(context.Background())
}

type rewriteTransport struct {
	target string // http://127.0.0.1:port
	base   http.RoundTripper
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite host to point at mock; preserve path + query.
	u := *req.URL
	parsed, err := req.URL.Parse(r.target)
	if err != nil {
		return nil, err
	}
	u.Scheme = parsed.Scheme
	u.Host = parsed.Host
	req2 := req.Clone(req.Context())
	req2.URL = &u
	req2.Host = parsed.Host
	return r.base.RoundTrip(req2)
}

// happyResponses returns mock routes for a fully healthy app.
func happyResponses(appID string) map[string]mockRoute {
	return map[string]mockRoute{
		"/open-apis/auth/v3/app_access_token/internal": {
			body: `{"code":0,"app_access_token":"t_tok_xyz","expire":7200}`,
		},
		"/open-apis/bot/v3/info": {
			body: `{"code":0,"bot":{"activate_status":1,"app_name":"ZyHive 测试 Bot","avatar_url":"https://example.com/a.jpg","open_id":"ou_botxxx"}}`,
		},
		"/open-apis/application/v6/applications/" + appID: {
			body: `{"code":0,"data":{"app":{"status":1,"scopes":[{"scope":"im:message"},{"scope":"im:message:send_as_bot"},{"scope":"im:resource"},{"scope":"contact:user.base:readonly"},{"scope":"im:chat:readonly"}]}}}`,
		},
		"/open-apis/application/v6/applications/" + appID + "/events/subscriptions": {
			body: `{"code":0,"data":{"event_list":[{"event_type":"im.message.receive_v1","channel":"websocket"}]}}`,
		},
		"/open-apis/im/v1/chats": {
			body: `{"code":0,"data":{"items":[{"chat_id":"oc_abc","name":"产品 1 群","chat_mode":"group","owner_id":"ou_abc"}]}}`,
		},
	}
}

func TestProbe_HappyPath(t *testing.T) {
	srv := newMockFeishu(t, happyResponses("cli_test"))
	var result *ProbeResult
	probeWithMock(t, srv.URL, func(ctx context.Context) {
		r, err := Probe(ctx, "cli_test", "secret_xyz")
		if err != nil {
			t.Fatalf("Probe: %v", err)
		}
		result = r
	})
	if !result.OK {
		t.Errorf("expected ok=true, got: %+v", result)
	}
	if result.Bot.Name != "ZyHive 测试 Bot" {
		t.Errorf("bot name not parsed: %s", result.Bot.Name)
	}
	if result.Bot.AvatarURL == "" {
		t.Errorf("avatar URL missing")
	}
	if !result.Published {
		t.Errorf("expected published=true")
	}
	if len(result.Permissions.Missing) != 0 {
		t.Errorf("expected no missing scopes, got: %v", result.Permissions.Missing)
	}
	if !result.Events.Subscribed || !result.Events.LongConnEnabled {
		t.Errorf("events not detected: %+v", result.Events)
	}
	if len(result.JoinedChats) != 1 || result.JoinedChats[0].Name != "产品 1 群" {
		t.Errorf("joined chats wrong: %+v", result.JoinedChats)
	}
}

func TestProbe_AuthFailed_BadSecret(t *testing.T) {
	srv := newMockFeishu(t, map[string]mockRoute{
		"/open-apis/auth/v3/app_access_token/internal": {
			body: `{"code":99991663,"msg":"app secret invalid"}`,
		},
	})
	var result *ProbeResult
	probeWithMock(t, srv.URL, func(ctx context.Context) {
		r, _ := Probe(ctx, "cli_test", "bad")
		result = r
	})
	if result.Error != "auth_failed" {
		t.Errorf("expected error=auth_failed, got %q", result.Error)
	}
}

func TestProbe_AppNotPublished(t *testing.T) {
	srv := newMockFeishu(t, map[string]mockRoute{
		"/open-apis/auth/v3/app_access_token/internal": {
			body: `{"code":10003,"msg":"app not published"}`,
		},
	})
	var result *ProbeResult
	probeWithMock(t, srv.URL, func(ctx context.Context) {
		r, _ := Probe(ctx, "cli_test", "secret_xyz")
		result = r
	})
	if result.Error != "app_not_published" {
		t.Errorf("expected error=app_not_published, got %q", result.Error)
	}
}

func TestProbe_MissingScopes(t *testing.T) {
	routes := happyResponses("cli_test")
	// Override app meta to only return 1 scope (im:message), so 4 should be missing.
	routes["/open-apis/application/v6/applications/cli_test"] = mockRoute{
		body: `{"code":0,"data":{"app":{"status":1,"scopes":[{"scope":"im:message"}]}}}`,
	}
	srv := newMockFeishu(t, routes)
	var result *ProbeResult
	probeWithMock(t, srv.URL, func(ctx context.Context) {
		r, _ := Probe(ctx, "cli_test", "secret_xyz")
		result = r
	})
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error != "missing_scopes" {
		t.Errorf("expected error=missing_scopes, got %q", result.Error)
	}
	if len(result.Permissions.Missing) != 4 {
		t.Errorf("expected 4 missing scopes, got %d: %v",
			len(result.Permissions.Missing), result.Permissions.Missing)
	}
}

func TestProbe_EventNotSubscribed(t *testing.T) {
	routes := happyResponses("cli_test")
	routes["/open-apis/application/v6/applications/cli_test/events/subscriptions"] = mockRoute{
		body: `{"code":0,"data":{"event_list":[]}}`,
	}
	srv := newMockFeishu(t, routes)
	var result *ProbeResult
	probeWithMock(t, srv.URL, func(ctx context.Context) {
		r, _ := Probe(ctx, "cli_test", "secret_xyz")
		result = r
	})
	if result.Error != "event_not_subscribed" {
		t.Errorf("expected error=event_not_subscribed, got %q", result.Error)
	}
}

func TestProbe_LongConnDisabled(t *testing.T) {
	routes := happyResponses("cli_test")
	routes["/open-apis/application/v6/applications/cli_test/events/subscriptions"] = mockRoute{
		body: `{"code":0,"data":{"event_list":[{"event_type":"im.message.receive_v1","channel":"webhook"}]}}`,
	}
	srv := newMockFeishu(t, routes)
	var result *ProbeResult
	probeWithMock(t, srv.URL, func(ctx context.Context) {
		r, _ := Probe(ctx, "cli_test", "secret_xyz")
		result = r
	})
	if result.Error != "long_conn_disabled" {
		t.Errorf("expected error=long_conn_disabled, got %q (subscribed=%v lc=%v)",
			result.Error, result.Events.Subscribed, result.Events.LongConnEnabled)
	}
}

func TestProbe_MissingRequiredFields(t *testing.T) {
	r, err := Probe(context.Background(), "", "")
	if err == nil {
		t.Fatalf("expected error on empty inputs")
	}
	if r.Error != "auth_failed" {
		t.Errorf("expected error=auth_failed, got %q", r.Error)
	}
}

func TestProbeErrorClass(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&probeHTTPError{Status: 401}, "auth_failed"},
		{&probeHTTPError{Status: 403}, "app_not_published"},
		{&probeHTTPError{Status: 500}, "unknown"},
		{errors.New("dial tcp: timeout"), "network"},
	}
	for _, c := range cases {
		got := probeErrorClass(c.err)
		if got != c.want {
			t.Errorf("probeErrorClass(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

func TestDiffScopes(t *testing.T) {
	missing := diffScopes(
		[]string{"a", "b", "c", "d"},
		[]string{"a", "c"},
	)
	if len(missing) != 2 || missing[0] != "b" || missing[1] != "d" {
		t.Errorf("diffScopes wrong: %v", missing)
	}
	// Empty granted → all missing.
	missing = diffScopes([]string{"x", "y"}, nil)
	if len(missing) != 2 {
		t.Errorf("expected 2, got %d", len(missing))
	}
	// Granted has extras (ignored).
	missing = diffScopes([]string{"a"}, []string{"a", "z"})
	if len(missing) != 0 {
		t.Errorf("expected 0, got %v", missing)
	}
}

// TestProbeResultMarshalShape — guarantee the JSON keys stay stable so the
// frontend wizard doesn't break on minor backend refactors.
func TestProbeResultMarshalShape(t *testing.T) {
	r := &ProbeResult{}
	r.Bot.Name = "x"
	r.Permissions.Missing = []string{"a"}
	r.JoinedChats = []ProbeChat{{ChatID: "c1", Name: "n"}}
	raw, _ := json.Marshal(r)
	for _, key := range []string{
		`"ok":`, `"bot":`, `"name":`, `"permissions":`, `"missing":`,
		`"events":`, `"subscribed":`, `"longConnEnabled":`, `"joinedChats":`,
	} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("missing key %s in marshaled output: %s", key, raw)
		}
	}
}

var _ = time.Time{} // keep import for any extension
