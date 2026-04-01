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
  - action: [{tag:"button", text:{tag:"plain_text",content:"OK"}, type:"primary|default|danger", url:"...", value:{...}}]
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
		Description: "Create a calendar event in the primary Feishu calendar.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"summary":{"type":"string","description":"Event title"},
				"start_time":{"type":"string","description":"Start time in RFC3339, e.g. 2026-04-01T14:00:00+08:00"},
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
		// Add attendees if provided
		if len(p.Attendees) > 0 {
			eventID := ""
			if data, ok := result["data"].(map[string]interface{}); ok {
				if ev, ok := data["event"].(map[string]interface{}); ok {
					eventID, _ = ev["event_id"].(string)
				}
			}
			if eventID != "" {
				attendees := make([]map[string]interface{}, len(p.Attendees))
				for i, uid := range p.Attendees {
					attendees[i] = map[string]interface{}{"type": "user", "user_id": uid}
				}
				_, _ = fc.do("POST",
					fmt.Sprintf("/calendar/v4/calendars/primary/events/%s/attendees/batch_delete", eventID),
					nil) // ignore
				_, _ = fc.do("POST",
					fmt.Sprintf("/calendar/v4/calendars/primary/events/%s/attendees", eventID),
					map[string]interface{}{"attendees": attendees})
			}
		}
		return fJSON(result["data"]), nil
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

	// suppress unused import warning
	_ = strings.TrimSpace
}
