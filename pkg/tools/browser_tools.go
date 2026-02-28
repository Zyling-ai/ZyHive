// pkg/tools/browser_tools.go — Browser automation tools for agent use.
// Registered via Registry.WithBrowser(). Requires a browser.Manager instance.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/browser"
	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// WithBrowser registers all browser automation tools on the Registry.
// mgr is the shared browser.Manager (created once per Pool).
// workspaceDir is used to save screenshots into .browser_screenshots/.
func (r *Registry) WithBrowser(mgr *browser.Manager, workspaceDir string) {
	agentID := r.agentID

	// ── browser_navigate ────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_navigate",
		Description: "在浏览器中打开指定 URL。返回页面标题、当前 URL、可交互元素列表（含 ref）和截图路径。首次使用会自动启动浏览器（需要已安装 Chromium）。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"url":{"type":"string","description":"要打开的网址，如 https://example.com"}
			},
			"required":["url"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if p.URL == "" {
			return "", fmt.Errorf("url 不能为空")
		}
		result, err := mgr.Navigate(agentID, p.URL, workspaceDir)
		if err != nil {
			return "", err
		}
		return formatSnapResult(result), nil
	})

	// ── browser_snapshot ────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_snapshot",
		Description: "获取当前页面的可交互元素列表（ARIA 树），并截图保存。每个元素有唯一 ref，用于 browser_click / browser_type 等操作。页面导航或刷新后需重新调用获取新 refs。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		result, err := mgr.Snapshot(agentID, workspaceDir)
		if err != nil {
			return "", err
		}
		return formatSnapResult(result), nil
	})

	// ── browser_screenshot ──────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_screenshot",
		Description: "对当前页面截图，保存到工作区并返回文件路径。可用 send_file 工具将截图发给用户。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		path, _, err := mgr.Screenshot(agentID, workspaceDir)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("截图已保存: %s\n可以用 send_file 工具发给用户。", path), nil
	})

	// ── browser_click ───────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_click",
		Description: "点击页面上的元素。使用 browser_snapshot 获取的 ref（如 e3）来定位元素，或使用 x/y 坐标。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"ref":    {"type":"string","description":"元素 ref（从 snapshot 获取，如 e3）"},
				"x":      {"type":"number","description":"点击的 X 坐标（替代 ref）"},
				"y":      {"type":"number","description":"点击的 Y 坐标（替代 ref）"},
				"double": {"type":"boolean","description":"是否双击，默认 false"}
			}
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Ref    string  `json:"ref"`
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Double bool    `json:"double"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if p.Ref != "" {
			if err := mgr.Click(agentID, p.Ref, p.Double); err != nil {
				return "", err
			}
			return fmt.Sprintf("已点击 ref=%s", p.Ref), nil
		}
		if p.X != 0 || p.Y != 0 {
			if err := mgr.ClickXY(agentID, p.X, p.Y); err != nil {
				return "", err
			}
			return fmt.Sprintf("已点击坐标 (%.0f, %.0f)", p.X, p.Y), nil
		}
		return "", fmt.Errorf("请提供 ref 或 x/y 坐标")
	})

	// ── browser_type ────────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_type",
		Description: "在输入框中输入文字。ref 为 snapshot 获取的元素引用；不填 ref 则输入到当前焦点元素。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"ref":  {"type":"string","description":"输入框的 ref（如 e5），不填则用当前焦点"},
				"text": {"type":"string","description":"要输入的文字"}
			},
			"required":["text"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Ref  string `json:"ref"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if err := mgr.Type(agentID, p.Ref, p.Text); err != nil {
			return "", err
		}
		return fmt.Sprintf("已输入: %q", p.Text), nil
	})

	// ── browser_fill ────────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_fill",
		Description: "清空输入框并填入新内容（比 browser_type 更可靠，自动清除旧内容）。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"ref":  {"type":"string","description":"输入框的 ref"},
				"text": {"type":"string","description":"要填入的文字"}
			},
			"required":["ref","text"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Ref  string `json:"ref"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if err := mgr.Fill(agentID, p.Ref, p.Text); err != nil {
			return "", err
		}
		return fmt.Sprintf("已填入 ref=%s: %q", p.Ref, p.Text), nil
	})

	// ── browser_press ───────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_press",
		Description: "按下键盘按键。支持：Enter, Tab, Escape, Backspace, Delete, ArrowUp/Down/Left/Right, Home, End, PageUp, PageDown, Space, F5，或单个字符。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"key": {"type":"string","description":"按键名称，如 Enter、Tab、Escape、ArrowDown"}
			},
			"required":["key"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if err := mgr.PressKey(agentID, p.Key); err != nil {
			return "", err
		}
		return fmt.Sprintf("已按下: %s", p.Key), nil
	})

	// ── browser_hover ───────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_hover",
		Description: "将鼠标悬停在元素上（不点击）。可触发 hover 菜单、tooltip 等。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"ref": {"type":"string","description":"元素 ref"}
			},
			"required":["ref"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Ref string `json:"ref"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if err := mgr.Hover(agentID, p.Ref); err != nil {
			return "", err
		}
		return fmt.Sprintf("已悬停在 ref=%s", p.Ref), nil
	})

	// ── browser_scroll ──────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_scroll",
		Description: "滚动页面。direction 为 up/down/left/right，amount 为滚动步数（默认 5）。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"direction": {"type":"string","description":"滚动方向: up | down | left | right"},
				"amount":    {"type":"integer","description":"滚动步数，默认 5"}
			},
			"required":["direction"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Direction string `json:"direction"`
			Amount    int    `json:"amount"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		steps := p.Amount
		if steps <= 0 {
			steps = 5
		}
		var offsetX, offsetY float64
		switch strings.ToLower(p.Direction) {
		case "down":
			offsetY = float64(steps) * 100
		case "up":
			offsetY = -float64(steps) * 100
		case "right":
			offsetX = float64(steps) * 100
		case "left":
			offsetX = -float64(steps) * 100
		default:
			return "", fmt.Errorf("不支持的滚动方向: %s（支持 up/down/left/right）", p.Direction)
		}
		if err := mgr.Scroll(agentID, offsetX, offsetY); err != nil {
			return "", err
		}
		return fmt.Sprintf("已向 %s 滚动 %d 步", p.Direction, steps), nil
	})

	// ── browser_select ──────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_select",
		Description: "在下拉选择框（<select>）中选择指定选项。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"ref":   {"type":"string","description":"select 元素的 ref"},
				"value": {"type":"string","description":"要选择的选项文字或 value"}
			},
			"required":["ref","value"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Ref   string `json:"ref"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if err := mgr.SelectOption(agentID, p.Ref, p.Value); err != nil {
			return "", err
		}
		return fmt.Sprintf("已选择 ref=%s: %q", p.Ref, p.Value), nil
	})

	// ── browser_eval ────────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_eval",
		Description: "在当前页面执行 JavaScript 并返回结果。可用于读取页面数据、修改 DOM、触发自定义事件等。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"js": {"type":"string","description":"要执行的 JavaScript 代码，最后一个表达式的值作为结果返回"}
			},
			"required":["js"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			JS string `json:"js"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		result, err := mgr.Eval(agentID, p.JS)
		if err != nil {
			return "", err
		}
		return result, nil
	})

	// ── browser_wait ────────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_wait",
		Description: "等待指定毫秒数（如等待动画、异步加载完成）。最大 10000ms。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"ms": {"type":"integer","description":"等待毫秒数，如 1000 = 1秒"}
			},
			"required":["ms"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Ms int `json:"ms"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if p.Ms > 10000 {
			p.Ms = 10000
		}
		if p.Ms < 0 {
			p.Ms = 0
		}
		mgr.Wait(agentID, p.Ms)
		return fmt.Sprintf("已等待 %dms", p.Ms), nil
	})

	// ── browser_tabs ────────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_tabs",
		Description: "查看当前所有打开的标签页（标题、URL、是否活跃）。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		tabs := mgr.Tabs(agentID)
		if len(tabs) == 0 {
			return "没有打开的标签页", nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("共 %d 个标签页：\n", len(tabs)))
		for _, t := range tabs {
			active := ""
			if t.Active {
				active = " ← 当前"
			}
			sb.WriteString(fmt.Sprintf("[%d] %s\n    %s%s\n", t.Index, t.Title, t.URL, active))
		}
		return strings.TrimRight(sb.String(), "\n"), nil
	})

	// ── browser_new_tab ─────────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_new_tab",
		Description: "打开新标签页，可选填入 URL。返回新标签的编号。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"url": {"type":"string","description":"可选：新标签要打开的 URL"}
			}
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(input, &p)
		idx, err := mgr.NewTab(agentID, p.URL)
		if err != nil {
			return "", err
		}
		if p.URL != "" {
			return fmt.Sprintf("已在标签 [%d] 中打开 %s", idx, p.URL), nil
		}
		return fmt.Sprintf("已打开新标签 [%d]", idx), nil
	})

	// ── browser_switch_tab ──────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_switch_tab",
		Description: "切换到指定编号的标签页（编号从 0 开始，通过 browser_tabs 查看）。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"index": {"type":"integer","description":"标签编号（从 0 开始）"}
			},
			"required":["index"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var p struct {
			Index int `json:"index"`
		}
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
		if err := mgr.SwitchTab(agentID, p.Index); err != nil {
			return "", err
		}
		return fmt.Sprintf("已切换到标签 [%d]", p.Index), nil
	})

	// ── browser_close_tab ───────────────────────────────────────────────────
	r.register(llm.ToolDef{
		Name:        "browser_close_tab",
		Description: "关闭当前标签页。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		if err := mgr.CloseTab(agentID); err != nil {
			return "", err
		}
		return "当前标签页已关闭", nil
	})
}

// formatSnapResult formats a SnapResult into a readable tool response.
func formatSnapResult(r *browser.SnapResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("页面: %s\nURL: %s\n\n", r.Title, r.URL))
	sb.WriteString("可交互元素 (ref 用于 click/type/fill 等操作):\n")
	if r.ARIATree != "" {
		sb.WriteString(r.ARIATree)
	} else {
		sb.WriteString("(无)")
	}
	if r.ScreenshotPath != "" {
		sb.WriteString(fmt.Sprintf("\n\n截图已保存: %s\n(可用 send_file 工具发给用户)", r.ScreenshotPath))
	}
	return sb.String()
}
