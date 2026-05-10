// Package network manages an agent's private contact book (通讯录), chat
// profiles (群档案), and relationship graph.
//
// Design philosophy — "每个 AI 成员都有一本自己的通讯录":
//   - Each agent owns a workspace/network/ directory.
//   - Contacts (真人联系人 from any source: feishu/telegram/web/panel) live as
//     one markdown file each in workspace/network/contacts/.
//   - Chat profiles (群档案 from group chats) live in workspace/network/chats/
//     — a sister type to Contact, modelling rooms instead of people. Created
//     automatically the first time a group/supergroup/channel message arrives.
//   - Agent-to-agent relations continue to live in workspace/network/RELATIONS.md
//     (migrated from workspace/RELATIONS.md on first access).
//   - A single INDEX.md is auto-generated on every mutation and injected into
//     the system prompt as the lightweight "progressive disclosure" layer —
//     it indexes contacts AND chats AND AI peers in one ~1k-char document.
//   - Full contact / chat profile is never forced into prompt — AI uses the
//     generic read tool on network/contacts/<id>.md or network/chats/<id>.md
//     when needed.
//
// Thread-safety: Store holds a mutex, safe for concurrent calls.
package network

// Source strings for contact IDs. Format of contact ID: "{source}:{externalId}".
const (
	SourceFeishu   = "feishu"
	SourceTelegram = "telegram"
	SourceWeb      = "web"
	SourcePanel    = "panel"
	SourceCron     = "cron"
)
