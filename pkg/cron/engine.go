// Package cron provides the scheduled job engine.
// Reference: openclaw/src/cron/, openclaw/src/gateway/server-cron.ts
// Job format is compatible with OpenClaw cron JSON format.
// Full implementation: Phase 3
package cron

// Engine wraps robfig/cron with job persistence.
// TODO: implement using github.com/robfig/cron/v3
type Engine struct{}

func NewEngine() *Engine { return &Engine{} }
