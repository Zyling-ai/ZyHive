// Package tools provides the built-in tool registry.
// Reference: pi-coding-agent/dist/core/tools/index.js
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
)

// Handler executes a tool call and returns the result string.
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

// Registry maps tool names to their definition and handler.
type Registry struct {
	defs     []llm.ToolDef
	handlers map[string]Handler
}

// New creates a Registry pre-loaded with all built-in tools.
func New() *Registry {
	r := &Registry{handlers: make(map[string]Handler)}
	r.register(readToolDef, handleRead)
	r.register(writeToolDef, handleWrite)
	r.register(editToolDef, handleEdit)
	r.register(bashToolDef, handleBash)
	r.register(grepToolDef, handleGrep)
	r.register(globToolDef, handleGlob)
	r.register(webFetchToolDef, handleWebFetch)
	return r
}

// Definitions returns all tool definitions for inclusion in LLM requests.
func (r *Registry) Definitions() []llm.ToolDef {
	return r.defs
}

// Execute runs the named tool with the given input.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	h, ok := r.handlers[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return h(ctx, input)
}

func (r *Registry) register(def llm.ToolDef, h Handler) {
	r.defs = append(r.defs, def)
	r.handlers[def.Name] = h
}
