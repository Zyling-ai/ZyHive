// Package browser provides a shared headless browser manager for agent tools.
// Uses go-rod (Chrome DevTools Protocol) for full browser automation.
// One browser instance is shared across all agents; each agent gets its own page session.
package browser

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// Manager owns the shared browser process and per-agent page sessions.
type Manager struct {
	mu       sync.Mutex
	browser  *rod.Browser
	sessions map[string]*AgentSession // agentID → session
}

// AgentSession holds browser state for one agent (tab list + active index).
type AgentSession struct {
	mu      sync.Mutex
	pages   []*rod.Page
	current int // index of the active tab (-1 = none)
}

// TabInfo describes one open browser tab.
type TabInfo struct {
	Index  int
	Active bool
	Title  string
	URL    string
}

// SnapResult is returned by Navigate and Snapshot.
type SnapResult struct {
	Title          string
	URL            string
	ARIATree       string // interactive elements as readable text
	ScreenshotPath string // path to saved PNG (in workspace)
	ScreenshotB64  string // base64 PNG (for inline display)
}

// NewManager creates a Manager. Call it once per Pool.
func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*AgentSession)}
}

// ── Browser / session lifecycle ──────────────────────────────────────────────

func (m *Manager) ensureBrowser() (*rod.Browser, error) {
	if m.browser != nil {
		return m.browser, nil
	}
	l := launcher.New().Headless(true)
	if runtime.GOOS == "linux" {
		l = l.
			Set("no-sandbox", "").
			Set("disable-setuid-sandbox", "").
			Set("disable-dev-shm-usage", "").
			Set("disable-gpu", "").
			Set("disable-software-rasterizer", "")
	}
	if bin := os.Getenv("ZYHIVE_BROWSER_BIN"); bin != "" {
		l = l.Bin(bin)
	}
	u, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("启动浏览器失败: %w\n提示: 请安装 chromium 或设置环境变量 ZYHIVE_BROWSER_BIN", err)
	}
	m.browser = rod.New().ControlURL(u).MustConnect()
	return m.browser, nil
}

func (m *Manager) getSession(agentID string) *AgentSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[agentID]
	if !ok {
		s = &AgentSession{current: -1}
		m.sessions[agentID] = s
	}
	return s
}

func (m *Manager) activePage(agentID string) *rod.Page {
	s := m.getSession(agentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current < 0 || s.current >= len(s.pages) {
		return nil
	}
	return s.pages[s.current]
}

// Close shuts down the shared browser. Call on Pool shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.browser != nil {
		_ = m.browser.Close()
		m.browser = nil
	}
}

// ── Navigation ───────────────────────────────────────────────────────────────

// Navigate opens url in the active tab (creates one if needed).
func (m *Manager) Navigate(agentID, url, workspaceDir string) (*SnapResult, error) {
	m.mu.Lock()
	b, err := m.ensureBrowser()
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}

	s := m.getSession(agentID)
	s.mu.Lock()
	var page *rod.Page
	if s.current >= 0 && s.current < len(s.pages) {
		page = s.pages[s.current]
	} else {
		page = b.MustPage("")
		s.pages = append(s.pages, page)
		s.current = len(s.pages) - 1
	}
	s.mu.Unlock()

	if err := page.Navigate(url); err != nil {
		return nil, fmt.Errorf("打开页面失败: %w", err)
	}
	_ = page.Timeout(15 * time.Second).WaitLoad()

	return m.snapPage(page, workspaceDir)
}

// ── Snapshot & Screenshot ────────────────────────────────────────────────────

// Snapshot returns ARIA tree + screenshot for the agent's active page.
func (m *Manager) Snapshot(agentID, workspaceDir string) (*SnapResult, error) {
	page := m.activePage(agentID)
	if page == nil {
		return nil, fmt.Errorf("没有打开的页面，请先调用 browser_navigate")
	}
	return m.snapPage(page, workspaceDir)
}

// Screenshot returns (filePath, base64PNG) for the active page.
func (m *Manager) Screenshot(agentID, workspaceDir string) (string, string, error) {
	page := m.activePage(agentID)
	if page == nil {
		return "", "", fmt.Errorf("没有打开的页面，请先调用 browser_navigate")
	}
	return m.takeScreenshot(page, workspaceDir)
}

func (m *Manager) snapPage(page *rod.Page, workspaceDir string) (*SnapResult, error) {
	info, err := page.Info()
	if err != nil {
		info = &proto.TargetTargetInfo{Title: "unknown", URL: "unknown"}
	}
	tree, _ := buildARIATree(page)
	ssPath, ssB64, _ := m.takeScreenshot(page, workspaceDir)
	return &SnapResult{
		Title:          info.Title,
		URL:            info.URL,
		ARIATree:       tree,
		ScreenshotPath: ssPath,
		ScreenshotB64:  ssB64,
	}, nil
}

func (m *Manager) takeScreenshot(page *rod.Page, workspaceDir string) (string, string, error) {
	q := 85
	buf, err := page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatPng,
		Quality: &q,
	})
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(workspaceDir, ".browser_screenshots")
	_ = os.MkdirAll(dir, 0755)
	name := fmt.Sprintf("screenshot_%d.png", time.Now().UnixMilli())
	path := filepath.Join(dir, name)
	_ = os.WriteFile(path, buf, 0644)
	return path, base64.StdEncoding.EncodeToString(buf), nil
}

// ── Mouse interactions ───────────────────────────────────────────────────────

// Click clicks an element by its ref (assigned during last Snapshot).
func (m *Manager) Click(agentID, ref string, double bool) error {
	page := m.activePage(agentID)
	if page == nil {
		return fmt.Errorf("没有打开的页面")
	}
	sel := fmt.Sprintf("[data-zy-ref=%q]", ref)
	el, err := page.Element(sel)
	if err != nil {
		return fmt.Errorf("找不到 ref=%s (请重新 snapshot): %w", ref, err)
	}
	if double {
		return el.Click(proto.InputMouseButtonLeft, 2)
	}
	return el.Click(proto.InputMouseButtonLeft, 1)
}

// ClickXY clicks at page coordinates (x, y).
func (m *Manager) ClickXY(agentID string, x, y float64) error {
	page := m.activePage(agentID)
	if page == nil {
		return fmt.Errorf("没有打开的页面")
	}
	if err := page.Mouse.MoveTo(proto.Point{X: x, Y: y}); err != nil {
		return err
	}
	return page.Mouse.Click(proto.InputMouseButtonLeft, 1)
}

// Hover moves the mouse over an element by ref (no click).
func (m *Manager) Hover(agentID, ref string) error {
	page := m.activePage(agentID)
	if page == nil {
		return fmt.Errorf("没有打开的页面")
	}
	el, err := page.Element(fmt.Sprintf("[data-zy-ref=%q]", ref))
	if err != nil {
		return fmt.Errorf("找不到 ref=%s: %w", ref, err)
	}
	return el.Hover()
}

// Scroll scrolls the page by deltaX/deltaY pixels (positive = down/right).
func (m *Manager) Scroll(agentID string, deltaX, deltaY float64) error {
	page := m.activePage(agentID)
	if page == nil {
		return fmt.Errorf("没有打开的页面")
	}
	return page.Mouse.Scroll(deltaX, deltaY, 1)
}

// ── Keyboard interactions ────────────────────────────────────────────────────

// Type focuses element by ref and types text. ref="" = type into focused element.
func (m *Manager) Type(agentID, ref, text string) error {
	page := m.activePage(agentID)
	if page == nil {
		return fmt.Errorf("没有打开的页面")
	}
	if ref != "" {
		el, err := page.Element(fmt.Sprintf("[data-zy-ref=%q]", ref))
		if err != nil {
			return fmt.Errorf("找不到 ref=%s: %w", ref, err)
		}
		if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
			return err
		}
	}
	// Convert string to []input.Key (input.Key is just a rune)
	runes := []rune(text)
	keys := make([]input.Key, len(runes))
	for i, r := range runes {
		keys[i] = input.Key(r)
	}
	return page.Keyboard.Type(keys...)
}

// PressKey presses a named key (e.g., "Enter", "Tab", "Escape", "ArrowDown").
func (m *Manager) PressKey(agentID, key string) error {
	page := m.activePage(agentID)
	if page == nil {
		return fmt.Errorf("没有打开的页面")
	}
	k := resolveKey(key)
	return page.Keyboard.Press(k)
}

// Fill clears an input and types new text (like a form fill).
func (m *Manager) Fill(agentID, ref, text string) error {
	page := m.activePage(agentID)
	if page == nil {
		return fmt.Errorf("没有打开的页面")
	}
	el, err := page.Element(fmt.Sprintf("[data-zy-ref=%q]", ref))
	if err != nil {
		return fmt.Errorf("找不到 ref=%s: %w", ref, err)
	}
	return el.Input(text)
}

// SelectOption selects an option in a <select> element by visible text.
func (m *Manager) SelectOption(agentID, ref, optionText string) error {
	page := m.activePage(agentID)
	if page == nil {
		return fmt.Errorf("没有打开的页面")
	}
	el, err := page.Element(fmt.Sprintf("[data-zy-ref=%q]", ref))
	if err != nil {
		return fmt.Errorf("找不到 ref=%s: %w", ref, err)
	}
	return el.Select([]string{optionText}, true, rod.SelectorTypeText)
}

// ── JavaScript ───────────────────────────────────────────────────────────────

// Eval executes JavaScript on the current page and returns the stringified result.
func (m *Manager) Eval(agentID, js string) (string, error) {
	page := m.activePage(agentID)
	if page == nil {
		return "", fmt.Errorf("没有打开的页面")
	}
	res, err := page.Eval(js)
	if err != nil {
		return "", err
	}
	return res.Value.String(), nil
}

// ── Tab management ───────────────────────────────────────────────────────────

// Tabs returns info about all open tabs for an agent.
func (m *Manager) Tabs(agentID string) []TabInfo {
	s := m.getSession(agentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []TabInfo
	for i, p := range s.pages {
		info, _ := p.Info()
		tab := TabInfo{Index: i, Active: i == s.current}
		if info != nil {
			tab.Title = info.Title
			tab.URL = info.URL
		}
		out = append(out, tab)
	}
	return out
}

// NewTab opens a new tab, optionally navigating to url.
func (m *Manager) NewTab(agentID, url string) (int, error) {
	m.mu.Lock()
	b, err := m.ensureBrowser()
	m.mu.Unlock()
	if err != nil {
		return -1, err
	}
	var page *rod.Page
	if url != "" {
		page = b.MustPage(url)
		_ = page.Timeout(15 * time.Second).WaitLoad()
	} else {
		page = b.MustPage("")
	}
	s := m.getSession(agentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pages = append(s.pages, page)
	s.current = len(s.pages) - 1
	return s.current, nil
}

// SwitchTab switches the active tab to the given index.
func (m *Manager) SwitchTab(agentID string, index int) error {
	s := m.getSession(agentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.pages) {
		return fmt.Errorf("标签 %d 不存在（共 %d 个）", index, len(s.pages))
	}
	s.current = index
	_, _ = s.pages[index].Activate()
	return nil
}

// CloseTab closes the active tab.
func (m *Manager) CloseTab(agentID string) error {
	s := m.getSession(agentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current < 0 || s.current >= len(s.pages) {
		return fmt.Errorf("没有打开的标签页")
	}
	_ = s.pages[s.current].Close()
	s.pages = append(s.pages[:s.current], s.pages[s.current+1:]...)
	if s.current >= len(s.pages) {
		s.current = len(s.pages) - 1
	}
	return nil
}

// Wait pauses for the specified duration.
func (m *Manager) Wait(_ string, ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// ── ARIA snapshot ────────────────────────────────────────────────────────────

const snapshotJS = `(function() {
  var items = [];
  var rid = 1;

  function label(el) {
    var v = el.getAttribute('aria-label')
      || el.getAttribute('placeholder')
      || el.getAttribute('title')
      || el.getAttribute('alt')
      || el.textContent;
    return (v||'').trim().replace(/\s+/g,' ').slice(0,120);
  }

  function role(el) {
    var r = el.getAttribute('role');
    if (r) return r;
    var m = {A:'link',BUTTON:'button',INPUT:'input',SELECT:'combobox',
      TEXTAREA:'textbox',IMG:'img',
      H1:'heading',H2:'heading',H3:'heading',
      H4:'heading',H5:'heading',H6:'heading'};
    return m[el.tagName] || el.tagName.toLowerCase();
  }

  function visible(el) {
    var r = el.getBoundingClientRect();
    if (!r || (r.width===0 && r.height===0)) return false;
    var s = window.getComputedStyle(el);
    return s.display!=='none' && s.visibility!=='hidden' && parseFloat(s.opacity||'1')>0;
  }

  var sel = [
    'a[href]','button:not([disabled])',
    'input:not([type=hidden]):not([disabled])',
    'select:not([disabled])','textarea:not([disabled])',
    '[role=button]','[role=link]','[role=checkbox]','[role=radio]',
    '[role=tab]','[role=menuitem]','[role=option]','[role=switch]',
    '[role=combobox]','[tabindex]:not([tabindex="-1"])'
  ].join(',');

  document.querySelectorAll(sel).forEach(function(el) {
    if (!visible(el)) return;
    var ref = 'e'+(rid++);
    el.setAttribute('data-zy-ref', ref);
    items.push({
      ref:     ref,
      role:    role(el),
      label:   label(el),
      type:    el.getAttribute('type')||'',
      value:   (el.value||'').slice(0,80),
      href:    (el.getAttribute('href')||'').slice(0,120),
      checked: !!el.checked,
      disabled:!!el.disabled
    });
  });
  return items;
})()`

func buildARIATree(page *rod.Page) (string, error) {
	type elem struct {
		Ref      string `json:"ref"`
		Role     string `json:"role"`
		Label    string `json:"label"`
		Type     string `json:"type"`
		Value    string `json:"value"`
		Href     string `json:"href"`
		Checked  bool   `json:"checked"`
		Disabled bool   `json:"disabled"`
	}
	res, err := page.Eval(snapshotJS)
	if err != nil {
		return "(无法解析页面元素)", err
	}
	var elems []elem
	if err := res.Value.Unmarshal(&elems); err != nil {
		return "(解析失败)", err
	}
	if len(elems) == 0 {
		return "(页面无可交互元素)", nil
	}
	var sb strings.Builder
	for _, e := range elems {
		sb.WriteString(fmt.Sprintf("[%s", e.Role))
		if e.Label != "" {
			sb.WriteString(fmt.Sprintf(" %q", e.Label))
		}
		if e.Type != "" && e.Type != "text" && e.Type != "submit" {
			sb.WriteString(fmt.Sprintf(" type=%s", e.Type))
		}
		if e.Value != "" {
			sb.WriteString(fmt.Sprintf(" value=%q", e.Value))
		}
		if e.Href != "" && e.Href != "#" {
			sb.WriteString(fmt.Sprintf(" href=%q", e.Href))
		}
		if e.Checked {
			sb.WriteString(" checked")
		}
		if e.Disabled {
			sb.WriteString(" disabled")
		}
		sb.WriteString(fmt.Sprintf(" ref=%s]\n", e.Ref))
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// ── Key mapping ──────────────────────────────────────────────────────────────

func resolveKey(key string) input.Key {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "enter", "return":
		return input.Enter
	case "tab":
		return input.Tab
	case "escape", "esc":
		return input.Escape
	case "backspace":
		return input.Backspace
	case "delete":
		return input.Delete
	case "arrowup", "up":
		return input.ArrowUp
	case "arrowdown", "down":
		return input.ArrowDown
	case "arrowleft", "left":
		return input.ArrowLeft
	case "arrowright", "right":
		return input.ArrowRight
	case "home":
		return input.Home
	case "end":
		return input.End
	case "pageup":
		return input.PageUp
	case "pagedown":
		return input.PageDown
	case "space":
		return input.Space
	case "f5":
		return input.F5
	default:
		if len([]rune(key)) == 1 {
			return input.Key([]rune(key)[0])
		}
		return input.Enter
	}
}
