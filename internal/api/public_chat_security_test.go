package api

import "testing"

func TestPublicChatRegistryExposesNoHostTools(t *testing.T) {
	reg := newPublicToolRegistry(t.TempDir(), "public-agent", "public-session")
	if defs := reg.Definitions(); len(defs) != 0 {
		names := make([]string, 0, len(defs))
		for _, def := range defs {
			names = append(names, def.Name)
		}
		t.Fatalf("public chat must expose no tools, got %v", names)
	}
}
