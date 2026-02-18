// Package channel manages inbound/outbound messaging channels.
// Reference: openclaw/src/channels/, openclaw/src/telegram/
package channel

// Hub routes inbound messages to the correct agent runner.
type Hub struct {
	telegramBot *TelegramBot
}

// NewHub creates a new channel hub.
func NewHub() *Hub { return &Hub{} }

// SetTelegramBot registers a Telegram bot with the hub.
func (h *Hub) SetTelegramBot(bot *TelegramBot) {
	h.telegramBot = bot
}
