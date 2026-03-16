// pkg/tools/policy.go — tool allow/deny/profile permission system.
// Mirrors OpenClaw's tools.allow / tools.deny / tools.profile config.
package tools

import (
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// ToolPolicy controls which built-in tools are exposed to the model.
// Deny wins over allow. Profile sets a base allowlist before allow/deny are applied.
type ToolPolicy struct {
	Profile string   `json:"profile,omitempty"` // "full"|"coding"|"messaging"|"minimal"
	Allow   []string `json:"allow,omitempty"`   // tool names or group:xxx shorthands
	Deny    []string `json:"deny,omitempty"`    // tool names or group:xxx shorthands
}

// ── Tool groups ──────────────────────────────────────────────────────────────

// toolGroups maps "group:xxx" shorthands to their member tool names.
var toolGroups = map[string][]string{
	"group:fs":       {"read", "write", "edit", "grep", "glob"},
	"group:runtime":  {"exec", "process"},
	"group:web":      {"web_fetch", "web_search"},
	"group:memory":   {"memory_search"},
	"group:ui":       {"browser", "show_image", "image"},
	"group:agent":    {"agent_list", "agent_spawn", "agent_tasks", "agent_kill", "agent_result"},
	"group:sessions": {"sessions_list", "sessions_history", "sessions_send"},
	"group:cron":     {"cron_list", "cron_add", "cron_remove"},
	"group:messaging": {"send_message", "send_file"},
	"group:self":     {"self_list_skills", "self_install_skill", "self_uninstall_skill", "self_rename", "self_update_soul", "self_set_env", "self_delete_env"},
	"group:project":  {"project_list", "project_read", "project_write", "project_create", "project_glob"},
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

// ApplyPolicy filters the registry's registered tools according to the given policy.
// Call this AFTER all With* methods have registered their tools, BEFORE the registry
// is passed to the runner.
//
// Logic:
//  1. Resolve profile base allowlist (nil profile or "full" = all tools allowed by default)
//  2. Apply allow additions
//  3. Apply deny removals (deny wins)
func (r *Registry) ApplyPolicy(policy ToolPolicy) {
	if policy.Profile == "" && len(policy.Allow) == 0 && len(policy.Deny) == 0 {
		return // no policy — keep all tools
	}

	// Step 1: base allowlist from profile
	var baseAllow map[string]bool
	if policy.Profile != "" && policy.Profile != "full" {
		if allowed, ok := profileAllowlists[policy.Profile]; ok && allowed != nil {
			baseAllow = make(map[string]bool)
			for _, name := range allowed {
				baseAllow[name] = true
			}
		}
	}
	// nil baseAllow = all tools allowed at this stage

	// Step 2: explicit allow additions
	extraAllow := expandNames(policy.Allow)

	// Step 3: deny set
	denySet := expandNames(policy.Deny)

	// Build filtered defs + handlers
	var filteredDefs []llm.ToolDef
	filteredHandlers := make(map[string]Handler)

	for _, def := range r.defs {
		name := strings.ToLower(def.Name)

		// Check deny (deny wins everything)
		if denySet["*"] || denySet[name] {
			continue
		}

		// Check allowlist
		allowed := false
		if baseAllow == nil {
			// No profile restriction — allowed by default
			allowed = true
		} else if baseAllow[name] {
			allowed = true
		}
		// Extra allow can add even if not in base profile
		if extraAllow["*"] || extraAllow[name] {
			allowed = true
		}

		if !allowed {
			continue
		}

		filteredDefs = append(filteredDefs, def)
		if h, ok := r.handlers[def.Name]; ok {
			filteredHandlers[def.Name] = h
		}
	}

	r.defs = filteredDefs
	r.handlers = filteredHandlers
}

// MergePolicy merges global and per-agent policies.
// Per-agent policy overrides global when set.
func MergePolicy(global, perAgent *ToolPolicy) ToolPolicy {
	if perAgent == nil {
		if global == nil {
			return ToolPolicy{}
		}
		return *global
	}
	// Per-agent fully overrides profile if set
	merged := ToolPolicy{
		Profile: perAgent.Profile,
		Allow:   perAgent.Allow,
		Deny:    perAgent.Deny,
	}
	if merged.Profile == "" && global != nil {
		merged.Profile = global.Profile
	}
	// Merge deny: both global and per-agent deny lists apply
	if global != nil {
		merged.Deny = append(merged.Deny, global.Deny...)
	}
	return merged
}
