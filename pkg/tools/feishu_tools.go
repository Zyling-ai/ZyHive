package tools

// Feishu Tools — registered when an agent has a Feishu channel configured.
// Provides 7 capabilities: messaging, chat, bitable, contacts, calendar, tasks.
// Injected via Registry.WithFeishu(appID, appSecret).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	lllm "github.com/Zyling-ai/zyhive/pkg/llm"
)

// feishuClient is a lightweight Feishu API client with token caching.
type feishuClient struct {
	appID     string
	appSecret string
	mu        sync.Mutex
	token     string
	expiry    time.Time
	hc        *http.Client
}

func newFeishuClient(appID, appSecret string) *feishuClient {
	return &feishuClient{
		appID: appID, appSecret: appSecret,
		hc: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *feishuClient) getToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expiry) {
		return c.token, nil
	}
	type tokenResp struct {
		Code           int    `json:"code"`
		Msg            string `json:"msg"`
		AppAccessToken string `json:"app_access_token"`
		Expire         int    `json:"expire"`
	}
	body, _ := json.Marshal(map[string]string{"app_id": c.appID, "app_secret": c.appSecret})
	resp, err := c.hc.Post(
		"https://open.feishu.cn/open-apis/auth/v3/app_access_token/internal",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var tr tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}
	if tr.Code != 0 {
		return "", fmt.Errorf("feishu token error %d: %s", tr.Code, tr.Msg)
	}
	c.token = tr.AppAccessToken
	expire := tr.Expire
	if expire <= 0 {
		expire = 7200
	}
	c.expiry = time.Now().Add(time.Duration(expire-300) * time.Second)
	return c.token, nil
}

func (c *feishuClient) do(method, path string, body interface{}) (map[string]interface{}, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, "https://open.feishu.cn/open-apis"+path, bodyReader)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if code, ok := result["code"].(float64); ok && code != 0 {
		msg, _ := result["msg"].(string)
		return result, fmt.Errorf("feishu error %d: %s", int(code), msg)
	}
	return result, nil
}

func fJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func toUnixStr(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return fmt.Sprintf("%d", t.Unix())
}

// WithFeishu registers 7 Feishu tools using the provided app credentials.
func (r *Registry) WithFeishu(appID, appSecret string) {
	if appID == "" || appSecret == "" {
		return
	}
	fc := newFeishuClient(appID, appSecret)

	// 0. feishu_send_rich_message — unified rich message sender with format guidance
	r.register(lllm.ToolDef{
		Name: "feishu_send_rich_message",
		Description: `Send a rich Feishu message using the most appropriate native format.

Choose msg_type based on content:
- "text": plain text, supports @mention with <at user_id="open_id"></at>
- "post": rich text with title, supports bold/link/at/image in structured content
- "interactive": card with header/sections/buttons/columns (schema 1.0)
  - header: {title:{tag:"plain_text",content:"Title"}, template:"blue|green|red|orange|grey|purple"}
  - elements: div(text/fields), hr, action(buttons with url or callback), img, note
  - div.text: {tag:"lark_md", content:"**bold** [link](url) <at id=open_id></at>"}
  - div.fields: [{is_short:true, text:{tag:"lark_md",content:"**Label**\nvalue"}}, ...]
  - action: [{tag:"button", text:{tag:"plain_text",content:"OK"}, type:"primary|default|danger", url:"...", value:{"agent_id":"AGENT_ID","session_id":"SESSION_ID","action":"confirm","label":"确认",...custom fields}}]
    IMPORTANT: For interactive buttons (no url), ALWAYS include agent_id and session_id in value so callbacks route correctly. Get session_id from the current feishu session context (feishu-{chat_id}).
  - note: {elements:[{tag:"plain_text",content:"footer text"}]}
- "image": single image (requires image_key from upload API, not available yet)

For post format content structure:
[[{tag:"text",text:"line1"},{tag:"a",text:"link",href:"url"},{tag:"at",user_id:"open_id"}]]
Each inner array is a paragraph, elements are inline.`,
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"receive_id":{"type":"string","description":"open_id of user or chat_id of group"},
				"receive_id_type":{"type":"string","enum":["open_id","chat_id"],"description":"ID type"},
				"msg_type":{"type":"string","enum":["text","post","interactive"],"description":"Message format type"},
				"content":{"type":"object","description":"Message content object (structure depends on msg_type). For text: {\"text\":\"hello\"}. For post: {\"zh_cn\":{\"title\":\"Title\",\"content\":[[...]]}}. For interactive: card JSON (schema 1.0)."}
			},
			"required":["receive_id","receive_id_type","msg_type","content"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			ReceiveID     string          `json:"receive_id"`
			ReceiveIDType string          `json:"receive_id_type"`
			MsgType       string          `json:"msg_type"`
			Content       json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		token, err := fc.getToken()
		if err != nil {
			return "", err
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"receive_id": p.ReceiveID,
			"msg_type":   p.MsgType,
			"content":    string(p.Content),
		})
		req, _ := http.NewRequest("POST",
			"https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type="+p.ReceiveIDType,
			bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := fc.hc.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		var result map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&result)
		if code, ok := result["code"].(float64); ok && code != 0 {
			return "", fmt.Errorf("send message error %d: %s", int(code), result["msg"])
		}
		data, _ := result["data"].(map[string]interface{})
		if msgID, ok := data["message_id"].(string); ok {
			return fmt.Sprintf("消息已发送，message_id=%s", msgID), nil
		}
		return "消息已发送", nil
	})

	// 1. feishu_send_message
	r.register(lllm.ToolDef{
		Name:        "feishu_send_message",
		Description: "Send a message to a Feishu user (open_id) or group chat (chat_id).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"receive_id":{"type":"string","description":"open_id of user or chat_id of group"},
				"receive_id_type":{"type":"string","description":"\"open_id\" or \"chat_id\"","enum":["open_id","chat_id"]},
				"msg_type":{"type":"string","description":"\"text\" or \"interactive\"","enum":["text","interactive"]},
				"content":{"type":"string","description":"JSON string. For text: {\"text\":\"hello\"}. For interactive: card JSON."}
			},
			"required":["receive_id","receive_id_type","msg_type","content"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			ReceiveID     string `json:"receive_id"`
			ReceiveIDType string `json:"receive_id_type"`
			MsgType       string `json:"msg_type"`
			Content       string `json:"content"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		result, err := fc.do("POST", "/im/v1/messages?receive_id_type="+p.ReceiveIDType,
			map[string]string{"receive_id": p.ReceiveID, "msg_type": p.MsgType, "content": p.Content})
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// 2. feishu_create_chat
	r.register(lllm.ToolDef{
		Name:        "feishu_create_chat",
		Description: "Create a new Feishu group chat and optionally invite members.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Group chat name"},
				"description":{"type":"string","description":"Group description (optional)"},
				"user_id_list":{"type":"array","items":{"type":"string"},"description":"List of open_ids to invite (optional)"}
			},
			"required":["name"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			UserIDList  []string `json:"user_id_list"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		body := map[string]interface{}{"name": p.Name}
		if p.Description != "" {
			body["description"] = p.Description
		}
		if len(p.UserIDList) > 0 {
			body["user_id_list"] = p.UserIDList
		}
		result, err := fc.do("POST", "/im/v1/chats", body)
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// 3. feishu_list_bitable_records
	r.register(lllm.ToolDef{
		Name:        "feishu_list_bitable_records",
		Description: "List records from a Feishu Bitable (multi-dimensional spreadsheet).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"app_token":{"type":"string","description":"Bitable app token (from the URL)"},
				"table_id":{"type":"string","description":"Table ID"},
				"filter":{"type":"string","description":"Filter expression (optional)"},
				"page_size":{"type":"integer","description":"Records per page (default 20, max 100)"}
			},
			"required":["app_token","table_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			AppToken string `json:"app_token"`
			TableID  string `json:"table_id"`
			Filter   string `json:"filter"`
			PageSize int    `json:"page_size"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		ps := p.PageSize
		if ps <= 0 || ps > 100 {
			ps = 20
		}
		path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/records?page_size=%d", p.AppToken, p.TableID, ps)
		if p.Filter != "" {
			path += "&filter=" + p.Filter
		}
		result, err := fc.do("GET", path, nil)
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// 4. feishu_create_bitable_record
	r.register(lllm.ToolDef{
		Name:        "feishu_create_bitable_record",
		Description: "Create a new record in a Feishu Bitable table.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"app_token":{"type":"string","description":"Bitable app token"},
				"table_id":{"type":"string","description":"Table ID"},
				"fields":{"type":"object","description":"Field name to value mapping, e.g. {\"名称\":\"张三\",\"年龄\":25}"}
			},
			"required":["app_token","table_id","fields"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			AppToken string                 `json:"app_token"`
			TableID  string                 `json:"table_id"`
			Fields   map[string]interface{} `json:"fields"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/records", p.AppToken, p.TableID)
		result, err := fc.do("POST", path, map[string]interface{}{"fields": p.Fields})
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// 4b. feishu_create_bitable_app (create a new Bitable)
	r.register(lllm.ToolDef{
		Name:        "feishu_create_bitable_app",
		Description: "Create a new Feishu Bitable (multi-dimensional spreadsheet) app. After creation, send the user a card with a button to open it using feishu_send_bitable_card.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Name of the new Bitable"},
				"folder_token":{"type":"string","description":"Folder token to create in (optional, defaults to root)"}
			},
			"required":["name"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Name        string `json:"name"`
			FolderToken string `json:"folder_token"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		result, err := fc.do("POST", "/bitable/v1/apps", map[string]string{
			"name":         p.Name,
			"folder_token": p.FolderToken,
		})
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// 4d. feishu_send_bitable_card — send a card with open button for a Bitable
	r.register(lllm.ToolDef{
		Name:        "feishu_send_bitable_card",
		Description: "Send a Feishu card message with a button to open a Bitable. Use this after creating a Bitable to share it with the user.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"chat_id":{"type":"string","description":"chat_id to send the card to"},
				"title":{"type":"string","description":"Card title, e.g. name of the Bitable"},
				"description":{"type":"string","description":"Short description shown in the card"},
				"url":{"type":"string","description":"URL of the Bitable (from app.url field)"}
			},
			"required":["chat_id","title","url"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			ChatID      string `json:"chat_id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			URL         string `json:"url"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		desc := p.Description
		if desc == "" {
			desc = "点击按钮直接打开表格"
		}
		// Use schema 1.0 card with action button (schema 2.0 does not support action tag)
		card := map[string]interface{}{
			"config": map[string]interface{}{"wide_screen_mode": true},
			"header": map[string]interface{}{
				"title":    map[string]string{"tag": "plain_text", "content": "📊 " + p.Title},
				"template": "green",
			},
			"elements": []interface{}{
				map[string]interface{}{
					"tag":  "div",
					"text": map[string]string{"tag": "lark_md", "content": desc},
				},
				map[string]interface{}{
					"tag": "action",
					"actions": []interface{}{
						map[string]interface{}{
							"tag":  "button",
							"text": map[string]string{"tag": "plain_text", "content": "🔗 打开多维表格"},
							"type": "primary",
							"url":  p.URL,
						},
					},
				},
			},
		}
		cardJSON, _ := json.Marshal(card)
		payload := map[string]interface{}{
			"receive_id": p.ChatID,
			"msg_type":   "interactive",
			"content":    string(cardJSON),
		}
		data, _ := json.Marshal(payload)

		token, err := fc.getToken()
		if err != nil {
			return "", err
		}
		req, _ := http.NewRequest("POST",
			"https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id",
			bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := fc.hc.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		var result map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&result)
		if code, ok := result["code"].(float64); ok && code != 0 {
			return "", fmt.Errorf("send card error %d: %s", int(code), result["msg"])
		}
		return "卡片已发送", nil
	})

	// 4c. feishu_create_bitable_table (create a new table in existing Bitable)
	r.register(lllm.ToolDef{
		Name:        "feishu_create_bitable_table",
		Description: "Create a new table inside an existing Feishu Bitable app.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"app_token":{"type":"string","description":"Bitable app token"},
				"name":{"type":"string","description":"Table name"},
				"fields":{"type":"array","description":"Initial fields definition (optional)","items":{"type":"object"}}
			},
			"required":["app_token","name"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			AppToken string                   `json:"app_token"`
			Name     string                   `json:"name"`
			Fields   []map[string]interface{} `json:"fields"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		body := map[string]interface{}{"table": map[string]interface{}{"name": p.Name}}
		if len(p.Fields) > 0 {
			body["table"].(map[string]interface{})["fields"] = p.Fields
		}
		result, err := fc.do("POST", fmt.Sprintf("/bitable/v1/apps/%s/tables", p.AppToken), body)
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// 5. feishu_get_user_info
	r.register(lllm.ToolDef{
		Name:        "feishu_get_user_info",
		Description: "Get Feishu user profile by open_id or user_id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"user_id":{"type":"string","description":"User ID value"},
				"user_id_type":{"type":"string","description":"\"open_id\" (default) or \"user_id\"","enum":["open_id","user_id"]}
			},
			"required":["user_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			UserID     string `json:"user_id"`
			UserIDType string `json:"user_id_type"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if p.UserIDType == "" {
			p.UserIDType = "open_id"
		}
		path := fmt.Sprintf("/contact/v3/users/%s?user_id_type=%s", p.UserID, p.UserIDType)
		result, err := fc.do("GET", path, nil)
		if err != nil {
			return "", err
		}
		data, _ := result["data"].(map[string]interface{})
		if user, ok := data["user"]; ok {
			return fJSON(user), nil
		}
		return fJSON(data), nil
	})

	// 6. feishu_create_calendar_event
	r.register(lllm.ToolDef{
		Name:        "feishu_create_calendar_event",
		Description: "Create a calendar event in the Feishu calendar. Returns app_link to open in Feishu. After creation, send the app_link to users using feishu_send_rich_message.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"summary":{"type":"string","description":"Event title"},
				"start_time":{"type":"string","description":"Start time in RFC3339, e.g. 2026-04-02T14:00:00+08:00"},
				"end_time":{"type":"string","description":"End time in RFC3339"},
				"description":{"type":"string","description":"Event description (optional)"},
				"attendees":{"type":"array","items":{"type":"string"},"description":"List of attendee open_ids (optional)"}
			},
			"required":["summary","start_time","end_time"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Summary     string   `json:"summary"`
			StartTime   string   `json:"start_time"`
			EndTime     string   `json:"end_time"`
			Description string   `json:"description"`
			Attendees   []string `json:"attendees"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		body := map[string]interface{}{
			"summary":    p.Summary,
			"start_time": map[string]string{"timestamp": toUnixStr(p.StartTime), "timezone": "Asia/Shanghai"},
			"end_time":   map[string]string{"timestamp": toUnixStr(p.EndTime), "timezone": "Asia/Shanghai"},
		}
		if p.Description != "" {
			body["description"] = p.Description
		}
		result, err := fc.do("POST", "/calendar/v4/calendars/primary/events", body)
		if err != nil {
			return "", err
		}

		// Extract event info
		eventID := ""
		appLink := ""
		if data, ok := result["data"].(map[string]interface{}); ok {
			if ev, ok := data["event"].(map[string]interface{}); ok {
				eventID, _ = ev["event_id"].(string)
				appLink, _ = ev["app_link"].(string)
			}
		}

		// Add attendees if provided
		if len(p.Attendees) > 0 && eventID != "" {
			attendees := make([]map[string]interface{}, len(p.Attendees))
			for i, uid := range p.Attendees {
				attendees[i] = map[string]interface{}{"type": "user", "user_id": uid, "user_id_type": "open_id"}
			}
			_, _ = fc.do("POST",
				fmt.Sprintf("/calendar/v4/calendars/primary/events/%s/attendees", eventID),
				map[string]interface{}{"attendees": attendees})
		}

		return fmt.Sprintf(`{"event_id":%q,"app_link":%q,"summary":%q,"start":%q,"end":%q}`,
			eventID, appLink, p.Summary, p.StartTime, p.EndTime), nil
	})

	// 7. feishu_create_task
	r.register(lllm.ToolDef{
		Name:        "feishu_create_task",
		Description: "Create a task in Feishu Task.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"summary":{"type":"string","description":"Task title"},
				"description":{"type":"string","description":"Task description (optional)"},
				"due":{"type":"string","description":"Due date in RFC3339, e.g. 2026-04-10T18:00:00+08:00 (optional)"}
			},
			"required":["summary"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Due         string `json:"due"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		body := map[string]interface{}{"summary": p.Summary}
		if p.Description != "" {
			body["description"] = p.Description
		}
		if p.Due != "" {
			body["due"] = map[string]interface{}{
				"timestamp":  toUnixStr(p.Due),
				"is_all_day": false,
			}
		}
		result, err := fc.do("POST", "/task/v2/tasks", body)
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// ── Chat & Contact tools ───────────────────────────────────────────────

	// feishu_list_chat_members — list members of a group chat
	r.register(lllm.ToolDef{
		Name:        "feishu_list_chat_members",
		Description: "List all members of a Feishu group chat.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"chat_id":{"type":"string","description":"Group chat ID (e.g. oc_xxx)"},
				"page_size":{"type":"integer","description":"Max members to return (default 50)"}
			},
			"required":["chat_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			ChatID   string `json:"chat_id"`
			PageSize int    `json:"page_size"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		ps := p.PageSize
		if ps <= 0 { ps = 50 }
		path := fmt.Sprintf("/im/v1/chats/%s/members?member_id_type=open_id&page_size=%d", p.ChatID, ps)
		result, err := fc.do("GET", path, nil)
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_list_chats — list all chats the bot is in
	r.register(lllm.ToolDef{
		Name:        "feishu_list_chats",
		Description: "List all group chats the bot has joined.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"page_size":{"type":"integer","description":"Max chats to return (default 20)"}
			}
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct{ PageSize int `json:"page_size"` }
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		ps := p.PageSize
		if ps <= 0 { ps = 20 }
		result, err := fc.do("GET", fmt.Sprintf("/im/v1/chats?user_id_type=open_id&page_size=%d", ps), nil)
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_list_users — list enterprise users from contact directory
	r.register(lllm.ToolDef{
		Name:        "feishu_list_users",
		Description: "List users from the Feishu enterprise contact directory. IMPORTANT: Must provide department_id. Use '0' for root department (lists all users). Use feishu_list_departments to get department IDs first.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"department_id":{"type":"string","description":"Department ID to list users from. Use '0' for root (all users). Required."},
				"page_size":{"type":"integer","description":"Max users to return (default 50, max 50)"}
			},
			"required":["department_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			DepartmentID string `json:"department_id"`
			PageSize     int    `json:"page_size"`
		}
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		if p.DepartmentID == "" { p.DepartmentID = "0" }
		ps := p.PageSize
		if ps <= 0 || ps > 50 { ps = 50 }
		path := fmt.Sprintf("/contact/v3/users?user_id_type=open_id&department_id=%s&page_size=%d", p.DepartmentID, ps)
		result, err := fc.do("GET", path, nil)
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_list_departments — list departments
	r.register(lllm.ToolDef{
		Name:        "feishu_list_departments",
		Description: "List departments in the Feishu enterprise. Use department_id='0' for root to get all top-level departments.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"parent_department_id":{"type":"string","description":"Parent department ID. Use '0' for root (default)."}
			}
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct{ ParentDepartmentID string `json:"parent_department_id"` }
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		if p.ParentDepartmentID == "" { p.ParentDepartmentID = "0" }
		path := fmt.Sprintf("/contact/v3/departments?user_id_type=open_id&department_id_type=department_id&parent_department_id=%s", p.ParentDepartmentID)
		result, err := fc.do("GET", path, nil)
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_get_chat_info — get detailed info about a chat
	r.register(lllm.ToolDef{
		Name:        "feishu_get_chat_info",
		Description: "Get detailed information about a Feishu group chat (name, description, member count, etc.).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"chat_id":{"type":"string","description":"Group chat ID"}
			},
			"required":["chat_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct{ ChatID string `json:"chat_id"` }
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		result, err := fc.do("GET", fmt.Sprintf("/im/v1/chats/%s", p.ChatID), nil)
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_create_doc — create a new cloud document
	r.register(lllm.ToolDef{
		Name:        "feishu_create_doc",
		Description: "Create a new Feishu cloud document (飞书文档).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"title":{"type":"string","description":"Document title"},
				"folder_token":{"type":"string","description":"Folder token to create in (optional)"}
			},
			"required":["title"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Title       string `json:"title"`
			FolderToken string `json:"folder_token"`
		}
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		body := map[string]string{"title": p.Title}
		if p.FolderToken != "" { body["folder_token"] = p.FolderToken }
		result, err := fc.do("POST", "/docx/v1/documents", body)
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_add_chat_members — add users to a group chat
	r.register(lllm.ToolDef{
		Name:        "feishu_add_chat_members",
		Description: "Add users to a Feishu group chat.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"chat_id":{"type":"string","description":"Group chat ID"},
				"user_ids":{"type":"array","items":{"type":"string"},"description":"List of open_ids to add"}
			},
			"required":["chat_id","user_ids"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			ChatID  string   `json:"chat_id"`
			UserIDs []string `json:"user_ids"`
		}
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		members := make([]map[string]string, len(p.UserIDs))
		for i, uid := range p.UserIDs {
			members[i] = map[string]string{"member_id": uid, "member_type": "user", "member_id_type": "open_id"}
		}
		result, err := fc.do("POST", fmt.Sprintf("/im/v1/chats/%s/members", p.ChatID),
			map[string]interface{}{"id_list": p.UserIDs})
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_update_chat — update group chat name/description
	r.register(lllm.ToolDef{
		Name:        "feishu_update_chat",
		Description: "Update a Feishu group chat's name or description.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"chat_id":{"type":"string","description":"Group chat ID"},
				"name":{"type":"string","description":"New chat name (optional)"},
				"description":{"type":"string","description":"New description (optional)"}
			},
			"required":["chat_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			ChatID      string `json:"chat_id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		body := map[string]string{}
		if p.Name != "" { body["name"] = p.Name }
		if p.Description != "" { body["description"] = p.Description }
		result, err := fc.do("PUT", fmt.Sprintf("/im/v1/chats/%s", p.ChatID), body)
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_pin_message — pin a message in a chat
	r.register(lllm.ToolDef{
		Name:        "feishu_pin_message",
		Description: "Pin (置顶) a message in a Feishu chat. The message will be shown at the top of the chat.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"message_id":{"type":"string","description":"Message ID to pin (starts with om_)"}
			},
			"required":["message_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct{ MessageID string `json:"message_id"` }
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		result, err := fc.do("POST", "/im/v1/pins", map[string]string{"message_id": p.MessageID})
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_unpin_message — unpin a message
	r.register(lllm.ToolDef{
		Name:        "feishu_unpin_message",
		Description: "Unpin (取消置顶) a previously pinned message.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"message_id":{"type":"string","description":"Message ID to unpin"}
			},
			"required":["message_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct{ MessageID string `json:"message_id"` }
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		token, err := fc.getToken()
		if err != nil { return "", err }
		req, _ := http.NewRequest("DELETE",
			"https://open.feishu.cn/open-apis/im/v1/pins/"+p.MessageID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := fc.hc.Do(req)
		if err != nil { return "", err }
		defer resp.Body.Close()
		return "已取消置顶", nil
	})

	// feishu_list_pins — list pinned messages in a chat
	r.register(lllm.ToolDef{
		Name:        "feishu_list_pins",
		Description: "List all pinned messages in a Feishu chat.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"chat_id":{"type":"string","description":"Group chat ID"}
			},
			"required":["chat_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct{ ChatID string `json:"chat_id"` }
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		result, err := fc.do("GET", fmt.Sprintf("/im/v1/pins?chat_id=%s", p.ChatID), nil)
		if err != nil { return "", err }
		return fJSON(result["data"]), nil
	})

	// feishu_send_and_pin — send a message and immediately pin it
	r.register(lllm.ToolDef{
		Name:        "feishu_send_and_pin",
		Description: "Send a text message to a chat and immediately pin it. Useful for announcements.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"chat_id":{"type":"string","description":"Group chat ID"},
				"text":{"type":"string","description":"Message text content"}
			},
			"required":["chat_id","text"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}
		if err := json.Unmarshal(input, &p); err != nil { return "", err }
		// Send message
		content, _ := json.Marshal(map[string]string{"text": p.Text})
		sendResult, err := fc.do("POST", "/im/v1/messages?receive_id_type=chat_id",
			map[string]string{"receive_id": p.ChatID, "msg_type": "text", "content": string(content)})
		if err != nil { return "", err }
		msgID := ""
		if data, ok := sendResult["data"].(map[string]interface{}); ok {
			msgID, _ = data["message_id"].(string)
		}
		if msgID == "" { return "消息已发送（置顶失败：无法获取消息ID）", nil }
		// Pin it
		_, err = fc.do("POST", "/im/v1/pins", map[string]string{"message_id": msgID})
		if err != nil { return fmt.Sprintf("消息已发送（message_id=%s），但置顶失败: %v", msgID, err), nil }
		return fmt.Sprintf("消息已发送并置顶，message_id=%s", msgID), nil
	})

	// ── Bitable 补全 ──────────────────────────────────────────────────────

	// feishu_bitable_get_meta — 从 URL 提取 app_token
	r.register(lllm.ToolDef{
		Name:        "feishu_bitable_get_meta",
		Description: "从飞书 Bitable URL 中解析提取 app_token，支持 /base/XXX 和 /wiki/XXX 格式。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"url":{"type":"string","description":"飞书 Bitable 页面 URL"}
			},
			"required":["url"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct{ URL string `json:"url"` }
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		re := regexp.MustCompile(`/(?:base|wiki)/([A-Za-z0-9]+)`)
		m := re.FindStringSubmatch(p.URL)
		if m == nil {
			return "", fmt.Errorf("无法从 URL 中提取 app_token: %s", p.URL)
		}
		return fJSON(map[string]string{"app_token": m[1], "url": p.URL}), nil
	})

	// feishu_bitable_list_fields — 列出表格字段
	r.register(lllm.ToolDef{
		Name:        "feishu_bitable_list_fields",
		Description: "列出 Feishu Bitable 表格的所有字段定义。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"app_token":{"type":"string","description":"Bitable app token"},
				"table_id":{"type":"string","description":"表格 ID"}
			},
			"required":["app_token","table_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			AppToken string `json:"app_token"`
			TableID  string `json:"table_id"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/fields", p.AppToken, p.TableID)
		result, err := fc.do("GET", path, nil)
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// feishu_bitable_get_record — 获取单条记录
	r.register(lllm.ToolDef{
		Name:        "feishu_bitable_get_record",
		Description: "获取 Feishu Bitable 表格中的单条记录。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"app_token":{"type":"string","description":"Bitable app token"},
				"table_id":{"type":"string","description":"表格 ID"},
				"record_id":{"type":"string","description":"记录 ID"}
			},
			"required":["app_token","table_id","record_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			AppToken string `json:"app_token"`
			TableID  string `json:"table_id"`
			RecordID string `json:"record_id"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/records/%s", p.AppToken, p.TableID, p.RecordID)
		result, err := fc.do("GET", path, nil)
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// feishu_bitable_update_record — 更新单条记录
	r.register(lllm.ToolDef{
		Name:        "feishu_bitable_update_record",
		Description: "更新 Feishu Bitable 表格中的单条记录字段。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"app_token":{"type":"string","description":"Bitable app token"},
				"table_id":{"type":"string","description":"表格 ID"},
				"record_id":{"type":"string","description":"记录 ID"},
				"fields":{"type":"object","description":"要更新的字段键值对，例如 {\"状态\":\"已完成\"}"}
			},
			"required":["app_token","table_id","record_id","fields"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			AppToken string                 `json:"app_token"`
			TableID  string                 `json:"table_id"`
			RecordID string                 `json:"record_id"`
			Fields   map[string]interface{} `json:"fields"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/records/%s", p.AppToken, p.TableID, p.RecordID)
		result, err := fc.do("PUT", path, map[string]interface{}{"fields": p.Fields})
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// feishu_bitable_delete_record — 删除单条记录
	r.register(lllm.ToolDef{
		Name:        "feishu_bitable_delete_record",
		Description: "删除 Feishu Bitable 表格中的单条记录。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"app_token":{"type":"string","description":"Bitable app token"},
				"table_id":{"type":"string","description":"表格 ID"},
				"record_id":{"type":"string","description":"记录 ID"}
			},
			"required":["app_token","table_id","record_id"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			AppToken string `json:"app_token"`
			TableID  string `json:"table_id"`
			RecordID string `json:"record_id"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		token, err := fc.getToken()
		if err != nil {
			return "", err
		}
		url := fmt.Sprintf("https://open.feishu.cn/open-apis/bitable/v1/apps/%s/tables/%s/records/%s",
			p.AppToken, p.TableID, p.RecordID)
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := fc.hc.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		var result map[string]interface{}
		_ = json.Unmarshal(raw, &result)
		if code, ok := result["code"].(float64); ok && code != 0 {
			msg, _ := result["msg"].(string)
			return "", fmt.Errorf("feishu error %d: %s", int(code), msg)
		}
		return fmt.Sprintf(`{"deleted":true,"record_id":%q}`, p.RecordID), nil
	})

	// feishu_bitable_create_field — 创建表格字段
	r.register(lllm.ToolDef{
		Name:        "feishu_bitable_create_field",
		Description: "在 Feishu Bitable 表格中创建新字段。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"app_token":{"type":"string","description":"Bitable app token"},
				"table_id":{"type":"string","description":"表格 ID"},
				"field_name":{"type":"string","description":"字段名称"},
				"field_type":{"type":"integer","description":"字段类型（默认 1=文本，2=数字，3=单选，4=多选，5=日期，7=复选框，11=人员，15=超链接，17=附件，18=关联，20=公式，21=双向关联，22=地理位置，23=群，1001=创建时间，1002=最后更新时间，1003=创建人，1004=修改人，1005=自动编号）","default":1}
			},
			"required":["app_token","table_id","field_name"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			AppToken  string `json:"app_token"`
			TableID   string `json:"table_id"`
			FieldName string `json:"field_name"`
			FieldType int    `json:"field_type"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if p.FieldType == 0 {
			p.FieldType = 1
		}
		path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/fields", p.AppToken, p.TableID)
		result, err := fc.do("POST", path, map[string]interface{}{
			"field_name": p.FieldName,
			"type":       p.FieldType,
		})
		if err != nil {
			return "", err
		}
		return fJSON(result["data"]), nil
	})

	// ── 云文档 feishu_doc（多 action） ─────────────────────────────────────

	r.register(lllm.ToolDef{
		Name: "feishu_doc",
		Description: `飞书云文档操作，支持 action: read/write/create/list_blocks。
- read: 读取文档纯文本内容，需要 doc_token
- create: 创建新文档，需要 title，可选 folder_token
- list_blocks: 列出文档所有块，需要 doc_token
- write: 向文档末尾追加文本块，需要 doc_token 和 content`,
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"action":{"type":"string","enum":["read","write","create","list_blocks"],"description":"操作类型"},
				"doc_token":{"type":"string","description":"文档 token（create 外必填）"},
				"content":{"type":"string","description":"追加的文本内容（write 用）"},
				"title":{"type":"string","description":"文档标题（create 用）"},
				"folder_token":{"type":"string","description":"目标文件夹 token（create 可选）"}
			},
			"required":["action"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Action      string `json:"action"`
			DocToken    string `json:"doc_token"`
			Content     string `json:"content"`
			Title       string `json:"title"`
			FolderToken string `json:"folder_token"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		switch p.Action {
		case "read":
			if p.DocToken == "" {
				return "", fmt.Errorf("doc_token 必填")
			}
			result, err := fc.do("GET", fmt.Sprintf("/docx/v1/documents/%s/raw_content", p.DocToken), nil)
			if err != nil {
				return "", err
			}
			return fJSON(result["data"]), nil

		case "create":
			if p.Title == "" {
				return "", fmt.Errorf("title 必填")
			}
			body := map[string]string{"title": p.Title}
			if p.FolderToken != "" {
				body["folder_token"] = p.FolderToken
			}
			result, err := fc.do("POST", "/docx/v1/documents", body)
			if err != nil {
				return "", err
			}
			return fJSON(result["data"]), nil

		case "list_blocks":
			if p.DocToken == "" {
				return "", fmt.Errorf("doc_token 必填")
			}
			result, err := fc.do("GET", fmt.Sprintf("/docx/v1/documents/%s/blocks", p.DocToken), nil)
			if err != nil {
				return "", err
			}
			return fJSON(result["data"]), nil

		case "write":
			if p.DocToken == "" {
				return "", fmt.Errorf("doc_token 必填")
			}
			if p.Content == "" {
				return "", fmt.Errorf("content 必填")
			}
			// Step 1: Convert Markdown to Feishu block format
			convertResult, err := fc.do("POST", "/docx/v1/documents/blocks/convert",
				map[string]string{"content_type": "markdown", "content": p.Content})
			if err != nil {
				return "", fmt.Errorf("Markdown 转换失败: %w", err)
			}
			data, _ := convertResult["data"].(map[string]interface{})
			blocks, _ := data["blocks"].([]interface{})
			firstIDs, _ := data["first_level_block_ids"].([]interface{})
			if len(blocks) == 0 {
				return "", fmt.Errorf("内容为空，无法写入")
			}
			// Step 2: Filter to first-level blocks only
			var insertBlocks []interface{}
			if len(firstIDs) > 0 {
				firstIDSet := make(map[string]bool)
				for _, id := range firstIDs {
					firstIDSet[fmt.Sprintf("%v", id)] = true
				}
				for _, b := range blocks {
					bm, ok := b.(map[string]interface{})
					if !ok { continue }
					if firstIDSet[fmt.Sprintf("%v", bm["block_id"])] {
						insertBlocks = append(insertBlocks, b)
					}
				}
			}
			if len(insertBlocks) == 0 {
				insertBlocks = blocks
			}
			// Step 3: Insert into document
			insertResult, err := fc.do("POST",
				fmt.Sprintf("/docx/v1/documents/%s/blocks/%s/children", p.DocToken, p.DocToken),
				map[string]interface{}{"children": insertBlocks})
			if err != nil {
				return "", err
			}
			_ = insertResult
			return fmt.Sprintf("写入成功，插入了 %d 个内容块", len(insertBlocks)), nil

		default:
			return "", fmt.Errorf("不支持的 action: %s，可选: read/write/create/list_blocks", p.Action)
		}
	})

	// ── 云盘 feishu_drive（多 action） ────────────────────────────────────

	r.register(lllm.ToolDef{
		Name: "feishu_drive",
		Description: `飞书云盘操作，支持 action: list/info/create_folder。
- list: 列出文件夹内文件，folder_token 为空时列出根目录
- info: 获取单个文件信息，需要 file_token
- create_folder: 创建文件夹，需要 name，可选 folder_token`,
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"action":{"type":"string","enum":["list","info","create_folder"],"description":"操作类型"},
				"folder_token":{"type":"string","description":"文件夹 token（list/create_folder 用，为空则用根目录）"},
				"file_token":{"type":"string","description":"文件 token（info 用）"},
				"name":{"type":"string","description":"文件夹名称（create_folder 用）"}
			},
			"required":["action"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Action      string `json:"action"`
			FolderToken string `json:"folder_token"`
			FileToken   string `json:"file_token"`
			Name        string `json:"name"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		switch p.Action {
		case "list":
			path := "/drive/v1/files"
			if p.FolderToken != "" {
				path += "?folder_token=" + p.FolderToken
			}
			result, err := fc.do("GET", path, nil)
			if err != nil {
				return "", err
			}
			return fJSON(result["data"]), nil

		case "info":
			if p.FileToken == "" {
				return "", fmt.Errorf("file_token 必填")
			}
			result, err := fc.do("GET", fmt.Sprintf("/drive/v1/files/%s", p.FileToken), nil)
			if err != nil {
				return "", err
			}
			return fJSON(result["data"]), nil

		case "create_folder":
			if p.Name == "" {
				return "", fmt.Errorf("name 必填")
			}
			body := map[string]string{"name": p.Name}
			if p.FolderToken != "" {
				body["folder_token"] = p.FolderToken
			}
			result, err := fc.do("POST", "/drive/v1/files/create_folder", body)
			if err != nil {
				return "", err
			}
			return fJSON(result["data"]), nil

		default:
			return "", fmt.Errorf("不支持的 action: %s，可选: list/info/create_folder", p.Action)
		}
	})

	// ── 知识库 feishu_wiki（多 action） ───────────────────────────────────

	r.register(lllm.ToolDef{
		Name: "feishu_wiki",
		Description: `飞书知识库操作，支持 action: spaces/nodes/get。
- spaces: 列出所有知识空间
- nodes: 列出指定知识空间的节点，需要 space_id
- get: 获取指定节点信息，需要 node_token`,
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"action":{"type":"string","enum":["spaces","nodes","get"],"description":"操作类型"},
				"space_id":{"type":"string","description":"知识空间 ID（nodes 用）"},
				"node_token":{"type":"string","description":"节点 token（get 用）"}
			},
			"required":["action"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Action    string `json:"action"`
			SpaceID   string `json:"space_id"`
			NodeToken string `json:"node_token"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		switch p.Action {
		case "spaces":
			result, err := fc.do("GET", "/wiki/v2/spaces", nil)
			if err != nil {
				return "", err
			}
			return fJSON(result["data"]), nil

		case "nodes":
			if p.SpaceID == "" {
				return "", fmt.Errorf("space_id 必填")
			}
			result, err := fc.do("GET", fmt.Sprintf("/wiki/v2/spaces/%s/nodes", p.SpaceID), nil)
			if err != nil {
				return "", err
			}
			return fJSON(result["data"]), nil

		case "get":
			if p.NodeToken == "" {
				return "", fmt.Errorf("node_token 必填")
			}
			result, err := fc.do("GET", fmt.Sprintf("/wiki/v2/spaces/get_node?token=%s", p.NodeToken), nil)
			if err != nil {
				return "", err
			}
			return fJSON(result["data"]), nil

		default:
			return "", fmt.Errorf("不支持的 action: %s，可选: spaces/nodes/get", p.Action)
		}
	})

	// ── feishu_app_scopes — 验证应用权限 ──────────────────────────────────

	r.register(lllm.ToolDef{
		Name:        "feishu_app_scopes",
		Description: "验证飞书应用 token 权限，返回 token 过期时间（expire > 0 表示权限正常）。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		type tokenResp struct {
			Code           int    `json:"code"`
			Msg            string `json:"msg"`
			AppAccessToken string `json:"app_access_token"`
			Expire         int    `json:"expire"`
		}
		body, _ := json.Marshal(map[string]string{
			"app_id":     fc.appID,
			"app_secret": fc.appSecret,
		})
		resp, err := fc.hc.Post(
			"https://open.feishu.cn/open-apis/auth/v3/app_access_token/internal",
			"application/json", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		var tr tokenResp
		if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
			return "", err
		}
		if tr.Code != 0 {
			return "", fmt.Errorf("权限验证失败 code=%d: %s", tr.Code, tr.Msg)
		}
		return fJSON(map[string]interface{}{
			"status": "ok",
			"expire": tr.Expire,
			"token_preview": func() string {
				if len(tr.AppAccessToken) > 8 {
					return tr.AppAccessToken[:8] + "..."
				}
				return tr.AppAccessToken
			}(),
		}), nil
	})

	// suppress unused import warning
	_ = strings.TrimSpace
	_ = regexp.MustCompile
}
