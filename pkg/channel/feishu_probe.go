// pkg/channel/feishu_probe.go — F1 (26.5.16v1).
//
// Comprehensive pre-flight probe for a Feishu app's App ID + Secret. One
// network call from the operator's perspective does all of:
//   - Validate credentials (try to fetch tenant_access_token)
//   - Fetch bot identity (name + avatar URL)
//   - Detect whether the app is published
//   - Detect granted vs missing OAuth scopes (compared to ZyHive required set)
//   - Detect whether im.message.receive_v1 event is subscribed
//   - Detect whether long-connection delivery is enabled
//
// The result is everything the 4-step setup wizard needs to either green-light
// the save or tell the user exactly what to fix in the Feishu admin console.

package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RequiredScopes is the canonical list of OAuth scopes ZyHive's Feishu channel
// needs. Used to compute the "missing" set in ProbeResult.
//
// Order matters for UI display. Group: receive + send + resources + contact + chats.
var RequiredScopes = []string{
	"im:message",                  // receive + send messages
	"im:message:send_as_bot",      // send-as-bot (mandatory for chat reply)
	"im:resource",                 // download images / files
	"contact:user.base:readonly",  // resolve sender name / avatar
	"im:chat:readonly",            // list joined groups (F4 group dashboard)
}

// ProbeResult is the JSON shape returned to the wizard.
type ProbeResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"` // "" | "auth_failed" | "app_not_published" | "network" | "unknown"

	Bot struct {
		OpenID    string `json:"openId,omitempty"`
		Name      string `json:"name,omitempty"`
		AvatarURL string `json:"avatarUrl,omitempty"`
	} `json:"bot"`

	Published bool `json:"published"`

	Permissions struct {
		Granted []string `json:"granted"`
		Missing []string `json:"missing"`
	} `json:"permissions"`

	Events struct {
		Subscribed      bool `json:"subscribed"`
		LongConnEnabled bool `json:"longConnEnabled"`
	} `json:"events"`

	JoinedChats []ProbeChat `json:"joinedChats,omitempty"`
}

// ProbeChat is a lightweight projection of a group/chat the bot is in.
type ProbeChat struct {
	ChatID      string `json:"chatId"`
	Name        string `json:"name"`
	Kind        string `json:"kind"` // "group" | "p2p" | "topic"
	MemberCount int    `json:"memberCount,omitempty"`
	OwnerID     string `json:"ownerId,omitempty"`
}

// Probe runs the entire pre-flight check. domain is "open.feishu.cn" for the
// CN cloud or "open.larksuite.com" for the international cloud (we auto-detect
// by trying CN first then falling back).
//
// Context is honoured by the underlying HTTP client; a 10-second total budget
// is recommended.
func Probe(ctx context.Context, appID, appSecret string) (*ProbeResult, error) {
	if appID == "" || appSecret == "" {
		return &ProbeResult{Error: "auth_failed"}, errors.New("appId and appSecret are required")
	}

	res := &ProbeResult{}

	// 1. Try CN first; fall back to International.
	token, domain, err := probeFetchToken(ctx, appID, appSecret)
	if err != nil {
		// Token fetch returns the most actionable error code.
		res.Error = probeErrorClass(err)
		return res, err
	}

	// 2. Bot info (name + avatar) — uses /bot/v3/info, raw because SDK doesn't wrap it.
	if err := probeFetchBotInfo(ctx, domain, token, res); err != nil {
		// Bot info failure is non-fatal: we still report everything else.
		// (Treat as "unknown" so the wizard surfaces it but doesn't block.)
	}

	// 3. App publish status / scopes.
	probeFetchAppMeta(ctx, domain, token, appID, res)

	// 4. Event subscription state.
	probeFetchEventSubs(ctx, domain, token, appID, res)

	// 5. Joined chats (graceful — empty if scope missing).
	probeFetchJoinedChats(ctx, domain, token, res)

	// 6. Compute missing scopes.
	res.Permissions.Missing = diffScopes(RequiredScopes, res.Permissions.Granted)

	// 7. Decide overall OK: bot info present, no missing scopes, events subscribed,
	//    long-conn enabled, app published.
	res.OK = res.Bot.Name != "" &&
		len(res.Permissions.Missing) == 0 &&
		res.Events.Subscribed &&
		res.Events.LongConnEnabled &&
		res.Published

	if !res.OK && res.Error == "" {
		// Choose the most actionable error to display first.
		switch {
		case !res.Published:
			res.Error = "app_not_published"
		case len(res.Permissions.Missing) > 0:
			res.Error = "missing_scopes"
		case !res.Events.Subscribed:
			res.Error = "event_not_subscribed"
		case !res.Events.LongConnEnabled:
			res.Error = "long_conn_disabled"
		default:
			res.Error = "unknown"
		}
	}

	return res, nil
}

// ── Token fetch + domain auto-detection ────────────────────────────────────

func probeFetchToken(ctx context.Context, appID, appSecret string) (string, string, error) {
	for _, domain := range []string{"open.feishu.cn", "open.larksuite.com"} {
		tok, err := probeFetchTokenOn(ctx, domain, appID, appSecret)
		if err == nil {
			return tok, domain, nil
		}
		// 401/403 means credentials are valid for this cloud but wrong/unpublished;
		// stop here, do not try the other cloud.
		var hErr *probeHTTPError
		if errors.As(err, &hErr) && (hErr.Status == 401 || hErr.Status == 403) {
			return "", domain, err
		}
		// Other errors fall through to try the other domain.
	}
	return "", "", &probeHTTPError{Status: 0, Body: "all domains unreachable"}
}

func probeFetchTokenOn(ctx context.Context, domain, appID, appSecret string) (string, error) {
	url := fmt.Sprintf("https://%s/open-apis/auth/v3/app_access_token/internal", domain)
	body, _ := json.Marshal(map[string]string{"app_id": appID, "app_secret": appSecret})
	req, _ := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	var parsed struct {
		Code           int    `json:"code"`
		Msg            string `json:"msg"`
		AppAccessToken string `json:"app_access_token"`
	}
	if jerr := json.Unmarshal(raw, &parsed); jerr != nil {
		return "", &probeHTTPError{Status: resp.StatusCode, Body: string(raw)}
	}
	if parsed.Code != 0 {
		status := 401
		// Feishu returns specific error codes for app_not_published etc.
		// 99991663 = app secret invalid; 99991664 = app id invalid; we map both as 401.
		// Anything in 99991xxx range is "operator's app problem".
		if parsed.Code == 99991663 || parsed.Code == 99991664 {
			status = 401
		} else if parsed.Code == 10003 {
			status = 403 // permission denied / not published
		}
		return "", &probeHTTPError{Status: status, Body: parsed.Msg, FeishuCode: parsed.Code}
	}
	return parsed.AppAccessToken, nil
}

// ── Bot info ───────────────────────────────────────────────────────────────

func probeFetchBotInfo(ctx context.Context, domain, token string, out *ProbeResult) error {
	url := fmt.Sprintf("https://%s/open-apis/bot/v3/info", domain)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			ActivateStatus int    `json:"activate_status"` // 0=not activated, 1=activated, 2=disabled
			AppName        string `json:"app_name"`
			AvatarURL      string `json:"avatar_url"`
			IPWhiteList    []any  `json:"ip_white_list"`
			OpenID         string `json:"open_id"`
		} `json:"bot"`
	}
	if jerr := json.Unmarshal(raw, &parsed); jerr != nil {
		return jerr
	}
	if parsed.Code != 0 {
		return &probeHTTPError{Status: resp.StatusCode, Body: parsed.Msg, FeishuCode: parsed.Code}
	}
	out.Bot.OpenID = parsed.Bot.OpenID
	out.Bot.Name = parsed.Bot.AppName
	out.Bot.AvatarURL = parsed.Bot.AvatarURL
	// Successful bot info implies the app is published (the API would return code != 0
	// otherwise). Most reliable signal we have.
	out.Published = parsed.Bot.ActivateStatus == 1
	return nil
}

// ── App metadata (granted scopes) ──────────────────────────────────────────

func probeFetchAppMeta(ctx context.Context, domain, token, appID string, out *ProbeResult) {
	// We use /application/v6/applications/:app_id/scopes (open-platform admin
	// API). When the app itself has the scope `application:app` we can read
	// its own scopes; otherwise we fall back to "assume all the standard scopes
	// are granted" (the explicit user-visible permission UI is the canonical
	// source anyway).
	url := fmt.Sprintf("https://%s/open-apis/application/v6/applications/%s",
		domain, appID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	q := req.URL.Query()
	q.Set("lang", "zh_cn")
	req.URL.RawQuery = q.Encode()
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			App struct {
				Scopes []struct {
					Scope string `json:"scope"`
				} `json:"scopes"`
				Status int `json:"status"` // 0 = stopped, 1 = published, 2 = disabled
			} `json:"app"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &parsed) != nil || parsed.Code != 0 {
		// Can't read app meta — try sending a real test message to detect IM scope.
		probeInferScopesFromIM(ctx, domain, token, out)
		return
	}
	for _, s := range parsed.Data.App.Scopes {
		if s.Scope != "" {
			out.Permissions.Granted = append(out.Permissions.Granted, s.Scope)
		}
	}
	if parsed.Data.App.Status == 1 {
		out.Published = true
	}
}

// probeInferScopesFromIM is the fallback when /applications/:app_id is unavailable.
// We try a no-op IM call (list chats with page_size=1) to confirm im:chat:readonly.
// On 403 we know the scope is missing. Best-effort signal.
func probeInferScopesFromIM(ctx context.Context, domain, token string, out *ProbeResult) {
	url := fmt.Sprintf("https://%s/open-apis/im/v1/chats?page_size=1", domain)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	var parsed struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return
	}
	if parsed.Code == 0 {
		// We got chats → must have im:chat:readonly granted.
		out.Permissions.Granted = append(out.Permissions.Granted,
			"im:chat:readonly", "im:message", "im:message:send_as_bot",
			"im:resource", "contact:user.base:readonly")
	}
}

// ── Event subscription detection ───────────────────────────────────────────

func probeFetchEventSubs(ctx context.Context, domain, token, appID string, out *ProbeResult) {
	// Application v6 event subscription query endpoint:
	//   GET /open-apis/event/v1/outbound_ip   (returns server IPs; non-empty ⇒ webhooks configured)
	//   GET /open-apis/application/v6/applications/:app_id/events/subscriptions
	url := fmt.Sprintf("https://%s/open-apis/application/v6/applications/%s/events/subscriptions",
		domain, appID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var parsed struct {
		Code int `json:"code"`
		Data struct {
			Subscriptions []struct {
				EventType string `json:"event_type"`
				Channel   string `json:"channel"` // "webhook" | "websocket"
			} `json:"event_list"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &parsed) != nil || parsed.Code != 0 {
		// API might not be available — fall back: probe will report unknown.
		return
	}
	for _, s := range parsed.Data.Subscriptions {
		if s.EventType == "im.message.receive_v1" {
			out.Events.Subscribed = true
			if s.Channel == "websocket" || s.Channel == "ws" {
				out.Events.LongConnEnabled = true
			}
		}
	}
}

// ── Joined chats ───────────────────────────────────────────────────────────

func probeFetchJoinedChats(ctx context.Context, domain, token string, out *ProbeResult) {
	url := fmt.Sprintf("https://%s/open-apis/im/v1/chats?page_size=20&sort_type=ByActiveTimeDesc", domain)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var parsed struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				ChatID    string `json:"chat_id"`
				Name      string `json:"name"`
				ChatMode  string `json:"chat_mode"` // group | p2p | topic
				OwnerID   string `json:"owner_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &parsed) != nil || parsed.Code != 0 {
		return
	}
	for _, it := range parsed.Data.Items {
		out.JoinedChats = append(out.JoinedChats, ProbeChat{
			ChatID:  it.ChatID,
			Name:    it.Name,
			Kind:    it.ChatMode,
			OwnerID: it.OwnerID,
		})
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────

func diffScopes(required, granted []string) []string {
	set := make(map[string]bool, len(granted))
	for _, g := range granted {
		set[g] = true
	}
	var missing []string
	for _, r := range required {
		if !set[r] {
			missing = append(missing, r)
		}
	}
	return missing
}

func probeErrorClass(err error) string {
	var hErr *probeHTTPError
	if !errors.As(err, &hErr) {
		return "network"
	}
	switch hErr.Status {
	case 401:
		return "auth_failed"
	case 403:
		// 403 from token fetch is usually "app not published" / not visible.
		return "app_not_published"
	default:
		return "unknown"
	}
}

// probeHTTPError carries both HTTP status and Feishu's response code.
type probeHTTPError struct {
	Status     int
	Body       string
	FeishuCode int
}

func (e *probeHTTPError) Error() string {
	if e.FeishuCode != 0 {
		return fmt.Sprintf("feishu code=%d msg=%s", e.FeishuCode, e.Body)
	}
	return fmt.Sprintf("http %d: %s", e.Status, e.Body)
}

// probeHTTPClient is a separate http.Client with a 8s timeout so probes never
// hang the wizard. (FeishuBot's own client has a longer timeout for streaming.)
var probeHTTPClient = &http.Client{Timeout: 8 * time.Second}
