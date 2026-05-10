// pkg/network/chat.go — 群档案 (Chat Profile) 数据模型.
//
// 与 Contact 形状对称但更简洁:
//   - Contact 模型一个"人" (家人/同事/客户/AI 等).
//   - Chat    模型一个"群" / "会话室" (飞书群 / TG 群 / TG 频道 / 私聊).
//
// 物理隔离: contacts/ 与 chats/ 是 workspace/network/ 下两个独立子目录,
// 即使一个 Contact ID 与一个 Chat ID 文本相同也互不冲突 (例如同时存在
// "feishu:ou_abc" 联系人和 "feishu:oc_abc" 群聊).
//
// 设计精简点 (相比 Contact):
//   - 无 Aliases / Primary 概念 (群不会"是同一个群").
//   - 无 IsOwner   概念 (群没有"主人本人在该渠道身份").
//   - 多了 Title / Kind / MemberCount 三个群专属字段.
package network

import "time"

// Chat represents one group/channel/private-chat conversation an agent has
// participated in. Sister type to Contact: contacts model people, chats model
// rooms. Both live under workspace/network/.
type Chat struct {
	// ID is the canonical identifier "{source}:{externalChatID}".
	// 例: feishu:oc_abc, telegram:-1001234, web:room-xxxx
	ID string `json:"id" yaml:"id"`

	Source     string `json:"source" yaml:"source"`
	ExternalID string `json:"externalId" yaml:"externalId"`

	// Title is the human-readable group/chat name. Empty string means "unknown"
	// (some platforms don't expose chat title in inbound message events).
	Title string `json:"title" yaml:"title"`

	// Kind: "group" | "supergroup" | "channel" | "private" — platform-specific
	// kind label, used only for UI / prompt categorization.
	Kind string `json:"kind" yaml:"kind"`

	// Tags allow free-form categorization (内部/客户/支持/社区 + custom).
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// MemberCount is the latest known member count (filled opportunistically
	// by channel handlers when API gives it; 0 means unknown).
	MemberCount int `json:"memberCount,omitempty" yaml:"memberCount,omitempty"`

	CreatedAt  time.Time `json:"createdAt" yaml:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt" yaml:"lastSeenAt"`
	MsgCount   int       `json:"msgCount" yaml:"msgCount"`

	// Body is the human/AI-authored markdown body, 4 sections by default:
	// 基础信息 / 群规则 / 重要议题 / 待跟进.
	Body string `json:"body" yaml:"-"`
}

// ChatSummary is the lightweight projection used in INDEX.json / API list.
type ChatSummary struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	Title       string    `json:"title"`
	Kind        string    `json:"kind"`
	Tags        []string  `json:"tags,omitempty"`
	MemberCount int       `json:"memberCount,omitempty"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
	MsgCount    int       `json:"msgCount"`
}

func (c *Chat) Summary() ChatSummary {
	return ChatSummary{
		ID:          c.ID,
		Source:      c.Source,
		Title:       c.Title,
		Kind:        c.Kind,
		Tags:        append([]string{}, c.Tags...),
		MemberCount: c.MemberCount,
		LastSeenAt:  c.LastSeenAt,
		MsgCount:    c.MsgCount,
	}
}

// ExternalIDPart returns just the externalID portion of the chat ID.
func (c ChatSummary) ExternalIDPart() string {
	_, ext, _ := SplitID(c.ID)
	if ext != "" {
		return ext
	}
	return c.ID
}
