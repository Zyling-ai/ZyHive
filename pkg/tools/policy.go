// pkg/tools/policy.go — tool allow/deny/profile permission system.
package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// ToolPolicy controls which built-in tools are exposed to the model.
// Deny wins over allow. Profile sets a base allowlist before allow/deny are applied.
type ToolPolicy struct {
	Profile string   `json:"profile,omitempty"` // "full"|"coding"|"messaging"|"minimal"
	Allow   []string `json:"allow,omitempty"`   // tool names or group:xxx shorthands
	Deny    []string `json:"deny,omitempty"`    // tool names or group:xxx shorthands
	// Ask 在 F-01 (26.5.12v1) 引入：tools whose names appear here go through
	// the approval Broker before executing. Supports group:xxx shorthands.
	Ask []string `json:"ask,omitempty"`
}

// ── Tool groups ──────────────────────────────────────────────────────────────

// toolGroups maps "group:xxx" shorthands to their member tool names.
var toolGroups = map[string][]string{
	"group:fs":      {"read", "write", "edit", "grep", "glob"},
	"group:runtime": {"exec", "process"},
	"group:web":     {"web_fetch", "web_search"},
	"group:memory":  {"memory_search"},
	"group:ui": {
		"browser_navigate", "browser_snapshot", "browser_screenshot",
		"browser_click", "browser_type", "browser_fill", "browser_press",
		"browser_hover", "browser_scroll", "browser_select", "browser_eval",
		"browser_wait", "browser_tabs", "browser_new_tab",
		"browser_switch_tab", "browser_close_tab", "show_image", "image",
	},
	"group:agent": {
		"agent_list", "agent_spawn", "agent_tasks", "agent_kill", "agent_result",
		"report_result", "report_to_parent",
	},
	"group:sessions":  {"sessions_list", "sessions_history", "sessions_send", "session_rename"},
	"group:cron":      {"cron_list", "cron_add", "cron_remove", "self_schedule"},
	"group:messaging": {"send_message", "send_file"},
	"group:self":      {"self_list_skills", "self_install_skill", "self_uninstall_skill", "self_rename", "self_update_soul", "self_set_env", "self_delete_env", "wish_add", "wish_list"},
	"group:project":   {"project_list", "project_read", "project_write", "project_create", "project_glob"},
	"group:network":   {"network_note", "chat_note"},
}

// profileAllowlists maps profile name → allowed tool names (nil = all).
var profileAllowlists = map[string][]string{
	"minimal": {
		"send_message", "memory_search",
	},
	"coding": flatten(
		toolGroups["group:fs"],
		toolGroups["group:runtime"],
		toolGroups["group:agent"],
		toolGroups["group:memory"],
		[]string{"image", "web_fetch", "web_search"},
	),
	"messaging": flatten(
		toolGroups["group:messaging"],
		toolGroups["group:sessions"],
		[]string{"memory_search"},
	),
	"full": nil, // nil = no restriction
}

func flatten(slices ...[]string) []string {
	var out []string
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

// ── Policy resolution ────────────────────────────────────────────────────────

// expandNames expands group shorthands (e.g. "group:fs") to individual tool names.
// "*" expands to the special sentinel "*" (matches everything).
func expandNames(patterns []string) map[string]bool {
	result := make(map[string]bool)
	for _, p := range patterns {
		if p == "*" {
			result["*"] = true
			continue
		}
		if members, ok := toolGroups[p]; ok {
			for _, m := range members {
				result[m] = true
			}
		} else {
			result[strings.ToLower(p)] = true
		}
	}
	return result
}

// DecodeToolPolicy parses one optional policy layer. Invalid JSON and unknown
// profiles are rejected so callers can fail closed instead of silently granting
// the full tool set.
func DecodeToolPolicy(raw json.RawMessage) (*ToolPolicy, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var policy ToolPolicy
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return nil, fmt.Errorf("decode tool policy: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode tool policy: trailing JSON value")
	}
	if policy.Profile != "" {
		if _, ok := profileAllowlists[policy.Profile]; !ok {
			return nil, fmt.Errorf("unknown tool policy profile %q", policy.Profile)
		}
	}
	for field, patterns := range map[string][]string{
		"allow": policy.Allow,
		"deny":  policy.Deny,
		"ask":   policy.Ask,
	} {
		for _, pattern := range patterns {
			if strings.HasPrefix(pattern, "group:") {
				if _, ok := toolGroups[pattern]; !ok {
					return nil, fmt.Errorf("unknown tool policy %s group %q", field, pattern)
				}
			}
		}
	}
	return &policy, nil
}

// ApplyPolicy filters the registry's registered tools according to the given policy.
// Call this AFTER all With* methods have registered their tools, BEFORE the registry
// is passed to the runner.
//
// Logic:
//  1. Resolve profile base allowlist (nil profile or "full" = all tools allowed by default)
//  2. Apply allow additions
//  3. Apply deny removals (deny wins)
func (r *Registry) ApplyPolicy(policy ToolPolicy) {
	r.ApplyPolicyLayers(&policy)
}

// ApplyPolicyLayers applies every non-nil policy as an independent security
// boundary. A tool must be allowed by every layer; deny therefore always wins,
// and a per-agent policy cannot broaden the global policy.
// It returns the union of all approval patterns.
func (r *Registry) ApplyPolicyLayers(policies ...*ToolPolicy) []string {
	active := make([]*ToolPolicy, 0, len(policies))
	var ask []string
	for _, policy := range policies {
		if policy == nil {
			continue
		}
		snapshot := *policy
		snapshot.Allow = append([]string(nil), policy.Allow...)
		snapshot.Deny = append([]string(nil), policy.Deny...)
		snapshot.Ask = append([]string(nil), policy.Ask...)
		active = append(active, &snapshot)
		ask = append(ask, policy.Ask...)
	}
	r.policyLayers = active
	r.governanceConfigured = true
	if len(active) == 0 {
		return nil
	}
	var filteredDefs []llm.ToolDef
	filteredHandlers := make(map[string]Handler)
	for _, def := range r.defs {
		name := strings.ToLower(def.Name)
		if !allowsAllPolicyLayers(active, name) {
			continue
		}
		filteredDefs = append(filteredDefs, def)
		if h, ok := r.handlers[def.Name]; ok {
			filteredHandlers[def.Name] = h
		}
	}
	r.defs = filteredDefs
	r.handlers = filteredHandlers
	return ask
}

func allowsAllPolicyLayers(policies []*ToolPolicy, name string) bool {
	name = strings.ToLower(name)
	for _, policy := range policies {
		if !policyAllowsTool(*policy, name) {
			return false
		}
	}
	return true
}

func policyAllowsTool(policy ToolPolicy, name string) bool {
	deny := expandNames(policy.Deny)
	if deny["*"] || deny[name] {
		return false
	}
	allowed := true
	if policy.Profile != "" && policy.Profile != "full" {
		allowed = false
		for _, candidate := range profileAllowlists[policy.Profile] {
			if candidate == name {
				allowed = true
				break
			}
		}
	}
	extra := expandNames(policy.Allow)
	return allowed || extra["*"] || extra[name]
}

// ConfigureGovernance is the single finalization point for registry policy and
// approval. It must run after every dynamic With* registration.
func (r *Registry) ConfigureGovernance(
	globalRaw, agentRaw json.RawMessage,
	broker *Broker,
	timeout time.Duration,
) error {
	globalPolicy, err := DecodeToolPolicy(globalRaw)
	if err != nil {
		r.ApplyPolicy(ToolPolicy{Deny: []string{"*"}})
		r.WithApprovalBroker(nil, nil, timeout)
		return err
	}
	agentPolicy, err := DecodeToolPolicy(agentRaw)
	if err != nil {
		r.ApplyPolicy(ToolPolicy{Deny: []string{"*"}})
		r.WithApprovalBroker(nil, nil, timeout)
		return err
	}
	ask := r.ApplyPolicyLayers(globalPolicy, agentPolicy)
	r.WithApprovalBroker(broker, ask, timeout)
	return nil
}
