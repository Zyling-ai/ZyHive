// Package channel — BotPool manages the lifecycle of running TelegramBot instances.
// Supports hot-add, hot-update, and hot-remove of Telegram channels without restarting
// the entire process.
package channel

import (
	"context"
	"log"
	"sync"
)

// BotPool tracks one running *TelegramBot per (agentID, channelID).
type BotPool struct {
	mu     sync.Mutex
	bots   map[string]*botEntry
	rootCtx context.Context
}

type botEntry struct {
	bot    *TelegramBot
	cancel context.CancelFunc
}

// NewBotPool creates a BotPool using the provided root context.
func NewBotPool(ctx context.Context) *BotPool {
	return &BotPool{
		bots:    make(map[string]*botEntry),
		rootCtx: ctx,
	}
}

func poolKey(agentID, channelID string) string {
	return agentID + "/" + channelID
}

// StartBot starts (or restarts) a bot for the given (agentID, channelID).
// Safe to call if already running — stops the old instance first.
func (p *BotPool) StartBot(agentID, channelID string, bot *TelegramBot) {
	p.mu.Lock()
	defer p.mu.Unlock()

	k := poolKey(agentID, channelID)
	if e, ok := p.bots[k]; ok {
		e.cancel()
		delete(p.bots, k)
		log.Printf("[botpool] stopped old bot agent=%s channel=%s", agentID, channelID)
	}

	ctx, cancel := context.WithCancel(p.rootCtx)
	p.bots[k] = &botEntry{bot: bot, cancel: cancel}
	go bot.Start(ctx)
	log.Printf("[botpool] started bot agent=%s channel=%s", agentID, channelID)
}

// StopBot stops the bot for the given (agentID, channelID) if running.
func (p *BotPool) StopBot(agentID, channelID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	k := poolKey(agentID, channelID)
	if e, ok := p.bots[k]; ok {
		e.cancel()
		delete(p.bots, k)
		log.Printf("[botpool] stopped bot agent=%s channel=%s", agentID, channelID)
	}
}

// IsRunning returns true if a bot is currently running for the given pair.
func (p *BotPool) IsRunning(agentID, channelID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.bots[poolKey(agentID, channelID)]
	return ok
}

// GetBot returns the running TelegramBot for the given (agentID, channelID), if any.
func (p *BotPool) GetBot(agentID, channelID string) (*TelegramBot, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.bots[poolKey(agentID, channelID)]
	if !ok {
		return nil, false
	}
	return e.bot, true
}

// GetFirstBot returns the first running bot for the given agentID (any channelID).
// Useful when the caller only knows the agentID and there's typically one bot per agent.
func (p *BotPool) GetFirstBot(agentID string) (*TelegramBot, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	prefix := agentID + "/"
	for k, e := range p.bots {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			channelID := k[len(prefix):]
			return e.bot, channelID, true
		}
	}
	return nil, "", false
}
