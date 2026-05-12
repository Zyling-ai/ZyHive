// pkg/tools/approval.go — 工具调用人工审批机制 (F-01, 26.5.12v1).
//
// 工作流：
//  1. Runner 调用 r.Tools.Execute(ctx, name, input)
//  2. 若 ToolPolicy.Ask 命中 name (或其 group)，Execute 不直接执行，而是
//     通过全局 Broker 创建一个 ApprovalRequest，阻塞等用户决策
//  3. UI 通过 SSE 收到 approval_request 事件，弹出审批卡 → 用户允许/拒绝
//  4. UI 调 POST /api/approvals/:id/{approve,deny} → Broker.Decide
//  5. 阻塞解除：approve → 真正执行工具；deny → 返回 "用户拒绝执行" 字符串
//  6. 默认 timeout (5min) 内未决策 → 自动拒绝 (UI 应展示倒计时)
//
// 每次决策（approve/deny/expired）都通过 audit hook 持久化，方便溯源。
//
// Broker 设计为进程级单例（main.go 注入），全 agent 共享。每个 pending
// request 持有一个 1-buffered channel，Decide 推一次 ApprovalDecision；
// timeout 与 ctx.Done 用 select 一并 case。

package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefaultApprovalTimeout 是没显式配置时的兜底 timeout。
const DefaultApprovalTimeout = 5 * time.Minute

// ApprovalRequest 是 broker 公开给 UI 的 pending item。
type ApprovalRequest struct {
	ID         string          `json:"id"`
	AgentID    string          `json:"agentId"`
	SessionID  string          `json:"sessionId,omitempty"`
	ToolName   string          `json:"toolName"`
	Input      json.RawMessage `json:"input"`
	CreatedAt  time.Time       `json:"createdAt"`
	ExpiresAt  time.Time       `json:"expiresAt"`
}

// ApprovalDecision 是 UI 通过 REST 推回的决策。
type ApprovalDecision struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"` // 拒绝时的可选理由
	By       string `json:"by,omitempty"`     // 决策人（token / username / "auto-timeout"）
}

// ApprovalEvent 通过 Subscribe 推给 SSE pipeline 的事件。
type ApprovalEvent struct {
	Type    string           `json:"type"` // "approval_request" | "approval_resolved" | "approval_expired"
	Request *ApprovalRequest `json:"request,omitempty"`
	// Resolved/Expired 时携带：
	ID       string             `json:"id,omitempty"`
	Decision *ApprovalDecision  `json:"decision,omitempty"`
}

// AuditHook 由 Broker 调用，把每次决策落盘到 audit 包（或任何 sink）。
// 调用站不持锁；hook 可以慢/可以 IO。
type AuditHook func(req ApprovalRequest, dec ApprovalDecision, eventType string)

// Broker 是审批中枢，进程级单例。
type Broker struct {
	mu      sync.Mutex
	pending map[string]*pendingItem
	subs    map[string]chan ApprovalEvent
	hook    AuditHook
}

type pendingItem struct {
	req     ApprovalRequest
	respCh  chan ApprovalDecision // buffered(1)
	expires time.Time
}

// NewBroker 返回一个新的 Broker。AuditHook 可为 nil（不持久化）。
func NewBroker(hook AuditHook) *Broker {
	return &Broker{
		pending: make(map[string]*pendingItem),
		subs:    make(map[string]chan ApprovalEvent),
		hook:    hook,
	}
}

// SetHook 允许在 main.go 启动顺序里晚一点注入 audit hook（先建 broker、
// 再建 audit log）。
func (b *Broker) SetHook(hook AuditHook) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hook = hook
}

// Request 创建一个新的 pending 请求并阻塞等待 Decide / timeout / ctx 取消。
// 返回的 ApprovalDecision 一定有意义：超时/取消会构造 Approved=false 并把
// Reason 设为 "timeout" / "cancelled"。
func (b *Broker) Request(ctx context.Context, agentID, sessionID, toolName string, input json.RawMessage, timeout time.Duration) (ApprovalDecision, ApprovalRequest, error) {
	if timeout <= 0 {
		timeout = DefaultApprovalTimeout
	}
	id := genApprovalID()
	now := time.Now().UTC()
	req := ApprovalRequest{
		ID:        id,
		AgentID:   agentID,
		SessionID: sessionID,
		ToolName:  toolName,
		Input:     input,
		CreatedAt: now,
		ExpiresAt: now.Add(timeout),
	}
	item := &pendingItem{
		req:     req,
		respCh:  make(chan ApprovalDecision, 1),
		expires: req.ExpiresAt,
	}

	b.mu.Lock()
	b.pending[id] = item
	b.mu.Unlock()

	// Broadcast "approval_request" so SSE clients render the approval card.
	b.broadcast(ApprovalEvent{Type: "approval_request", Request: &req})

	defer func() {
		// Always remove from pending on exit (avoid leaks).
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}()

	select {
	case dec := <-item.respCh:
		// Decide() already broadcast approval_resolved and ran hook.
		return dec, req, nil
	case <-time.After(timeout):
		// Auto-deny on timeout.
		dec := ApprovalDecision{Approved: false, Reason: "审批超时（默认 5 分钟）", By: "auto-timeout"}
		b.broadcast(ApprovalEvent{Type: "approval_expired", ID: id, Decision: &dec})
		if hook := b.snapshotHook(); hook != nil {
			hook(req, dec, "approval_expired")
		}
		return dec, req, nil
	case <-ctx.Done():
		dec := ApprovalDecision{Approved: false, Reason: "请求已取消（context cancelled）", By: "auto-cancel"}
		b.broadcast(ApprovalEvent{Type: "approval_expired", ID: id, Decision: &dec})
		if hook := b.snapshotHook(); hook != nil {
			hook(req, dec, "approval_cancelled")
		}
		return dec, req, ctx.Err()
	}
}

// Decide 由 REST handler 调用，把决策推给阻塞中的 Request 调用。
// 找不到对应 ID 时返回 error（说明已过期 / 已被决策 / 不存在）。
func (b *Broker) Decide(id string, dec ApprovalDecision) error {
	b.mu.Lock()
	item, ok := b.pending[id]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("approval %q not found (expired or already decided?)", id)
	}
	// 非阻塞 send：respCh 容量 1。重复 Decide 走默认分支抛错。
	select {
	case item.respCh <- dec:
		// Broadcast resolved + hook.
		b.broadcast(ApprovalEvent{Type: "approval_resolved", ID: id, Decision: &dec})
		if hook := b.snapshotHook(); hook != nil {
			req := item.req
			eventType := "approval_approved"
			if !dec.Approved {
				eventType = "approval_denied"
			}
			hook(req, dec, eventType)
		}
		return nil
	default:
		return errors.New("approval already decided")
	}
}

// ListPending 返回当前所有 pending（可选按 agentID 过滤）。
func (b *Broker) ListPending(agentID string) []ApprovalRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]ApprovalRequest, 0, len(b.pending))
	for _, item := range b.pending {
		if agentID != "" && item.req.AgentID != agentID {
			continue
		}
		out = append(out, item.req)
	}
	return out
}

// Subscribe 注册一个 SSE 订阅。返回的 channel 在 unsub() 调用后关闭。
// 同时 channel 是 buffered(16)；满了直接丢（不阻塞 broker）。
func (b *Broker) Subscribe(clientID string) (<-chan ApprovalEvent, func()) {
	ch := make(chan ApprovalEvent, 16)
	b.mu.Lock()
	b.subs[clientID] = ch
	b.mu.Unlock()
	unsub := func() {
		b.mu.Lock()
		if c, ok := b.subs[clientID]; ok {
			delete(b.subs, clientID)
			close(c)
		}
		b.mu.Unlock()
	}
	return ch, unsub
}

// broadcast 推一条事件给所有订阅者（非阻塞）。
func (b *Broker) broadcast(ev ApprovalEvent) {
	b.mu.Lock()
	subs := make([]chan ApprovalEvent, 0, len(b.subs))
	for _, c := range b.subs {
		subs = append(subs, c)
	}
	b.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- ev:
		default:
			// Drop on full buffer rather than block the broker.
		}
	}
}

func (b *Broker) snapshotHook() AuditHook {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hook
}

// genApprovalID returns a short, URL-safe identifier.
func genApprovalID() string {
	var bts [9]byte
	_, _ = rand.Read(bts[:])
	return "apv_" + hex.EncodeToString(bts[:])
}

// ── Registry hooks ────────────────────────────────────────────────────────

// WithApprovalBroker 把 broker + ask 列表 + timeout 注入到 Registry。
// 调用 ApplyPolicy 之后再调用此方法（ApplyPolicy 会重置 askNames）。
func (r *Registry) WithApprovalBroker(b *Broker, ask []string, timeout time.Duration) *Registry {
	r.broker = b
	r.askTimeout = timeout
	r.askNames = expandNames(ask)
	return r
}

// SetApprovalContext 在创建 chat session 时调用，让 Broker 知道当前 agent
// 和 session（用于 broadcast 给前端 "属于这个 session" 的事件过滤）。
// Registry 自身已有 agentID + sessionID，所以这里其实是一个语法糖。
// 我们已在 WithSessionID 里设了 sessionID；现在补一个 agentID。
func (r *Registry) WithAgentID(agentID string) *Registry {
	r.agentID = agentID
	return r
}
