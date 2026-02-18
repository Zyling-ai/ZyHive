// Package channel manages inbound/outbound messaging channels.
// Reference: openclaw/src/channels/, openclaw/src/telegram/
// Full implementation: Phase 3
package channel

// Hub routes inbound messages to the correct agent runner.
type Hub struct {
	// TODO: map of channelID → agentID
	// TODO: Telegram bot integration
	// TODO: iMessage integration
}

func NewHub() *Hub { return &Hub{} }
