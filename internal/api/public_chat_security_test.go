package api

import (
	"encoding/json"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/session"
)

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

func TestPublicWorkerOwnerSeparatesAgentsAndChannels(t *testing.T) {
	first := publicWorkerOwner("agent-a", "channel")
	if first == publicWorkerOwner("agent-b", "channel") {
		t.Fatal("different agents must have different public worker owners")
	}
	if first == publicWorkerOwner("agent-a", "other-channel") {
		t.Fatal("different channels must have different public worker owners")
	}
}

func TestPublicTerminalDataPreservesErrorAndAddsSession(t *testing.T) {
	data := publicTerminalData(session.BroadcastEvent{
		Type: "error",
		Data: []byte(`{"type":"error","error":"deadline exceeded"}`),
	}, "web-session")
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["error"] != "deadline exceeded" || payload["sessionId"] != "web-session" {
		t.Fatalf("unexpected terminal payload: %s", data)
	}
}
