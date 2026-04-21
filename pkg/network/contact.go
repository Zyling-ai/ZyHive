package network

import (
	"time"
)

// Contact represents one person (from any source) this agent has exchanged
// messages with at least once.
//
// The markdown file at contacts/<id>.md is the source of truth for human-
// facing content (profile text). The frontmatter block at the top of the
// markdown mirrors this struct (minus Notes) and is re-serialized on every save.
type Contact struct {
	// ID is the canonical identifier "{source}:{externalId}" (e.g.
	// "feishu:ou_abc123" / "telegram:123456789" / "web:sid-xxxx").
	ID string `json:"id" yaml:"id"`

	Source     string `json:"source" yaml:"source"`
	ExternalID string `json:"externalId" yaml:"externalId"`

	DisplayName string `json:"displayName" yaml:"displayName"`

	// Tags allow free-form categorization (家人/同事/客户/合作伙伴/朋友/AI 成员 + custom).
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Aliases lists other Contact IDs that refer to the same real-world person.
	// Set via manual merge; only the primary=true contact holds the canonical profile.
	Aliases []string `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Primary bool     `json:"primary" yaml:"primary"`

	// IsOwner marks a contact as "this is the agent owner under another identity"
	// (e.g. the panel operator also appears in Feishu). When resolveContact finds
	// a contact with IsOwner=true, the system prompt uses owner-profile.md instead
	// of a full contact profile to avoid duplication.
	IsOwner bool `json:"isOwner,omitempty" yaml:"isOwner,omitempty"`

	CreatedAt  time.Time `json:"createdAt" yaml:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt" yaml:"lastSeenAt"`
	MsgCount   int       `json:"msgCount" yaml:"msgCount"`

	// Body is the human-authored markdown (everything below the frontmatter).
	// Not serialized as frontmatter; stored as file body.
	Body string `json:"body" yaml:"-"`
}

// ContactSummary is the lightweight projection used in INDEX.json / API list.
type ContactSummary struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	DisplayName string    `json:"displayName"`
	Tags        []string  `json:"tags,omitempty"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
	MsgCount    int       `json:"msgCount"`
	Primary     bool      `json:"primary"`
	IsOwner     bool      `json:"isOwner,omitempty"`
}

func (c *Contact) Summary() ContactSummary {
	return ContactSummary{
		ID:          c.ID,
		Source:      c.Source,
		DisplayName: c.DisplayName,
		Tags:        append([]string{}, c.Tags...),
		LastSeenAt:  c.LastSeenAt,
		MsgCount:    c.MsgCount,
		Primary:     c.Primary,
		IsOwner:     c.IsOwner,
	}
}

// Index is the machine-readable INDEX.json snapshot.
type Index struct {
	Contacts  []ContactSummary `json:"contacts"`
	UpdatedAt time.Time        `json:"updatedAt"`
}
