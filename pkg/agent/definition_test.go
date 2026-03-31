package agent

import "testing"

func TestGetBuiltInAgent_Found(t *testing.T) {
	for _, def := range BuiltInAgentDefinitions {
		got := GetBuiltInAgent(def.AgentType)
		if got == nil {
			t.Errorf("GetBuiltInAgent(%q) returned nil", def.AgentType)
		}
	}
}

func TestGetBuiltInAgent_NotFound(t *testing.T) {
	if GetBuiltInAgent("nonexistent") != nil {
		t.Error("GetBuiltInAgent(nonexistent) should return nil")
	}
}

func TestIsReadOnlyAgent(t *testing.T) {
	cases := map[string]bool{
		"explore":         true,
		"plan":            true,
		"general-purpose": false,
		"verification":    false,
		"coordinator":     false,
	}
	for agentType, want := range cases {
		def := GetBuiltInAgent(agentType)
		if def == nil {
			t.Errorf("agent %q not found", agentType)
			continue
		}
		if got := def.IsReadOnlyAgent(); got != want {
			t.Errorf("IsReadOnlyAgent(%q) = %v, want %v", agentType, got, want)
		}
	}
}

func TestEffectiveModel(t *testing.T) {
	parent := "parent-model"
	cases := []struct {
		model string
		want  string
	}{
		{"", parent},
		{"inherit", parent},
		{"anthropic/claude-opus-4-5", "anthropic/claude-opus-4-5"},
	}
	for _, c := range cases {
		def := &AgentDefinition{Model: c.model}
		if got := def.EffectiveModel(parent); got != c.want {
			t.Errorf("EffectiveModel(%q) = %q, want %q", c.model, got, c.want)
		}
	}
}

func TestAllBuiltInAgentsHaveRequiredFields(t *testing.T) {
	for _, def := range BuiltInAgentDefinitions {
		if def.AgentType == "" {
			t.Error("built-in agent has empty AgentType")
		}
		if def.WhenToUse == "" {
			t.Errorf("agent %q has empty WhenToUse", def.AgentType)
		}
		if def.Source != "built-in" {
			t.Errorf("agent %q source = %q, want built-in", def.AgentType, def.Source)
		}
		if len(def.Tools) == 0 {
			t.Errorf("agent %q has no tools defined", def.AgentType)
		}
	}
}
