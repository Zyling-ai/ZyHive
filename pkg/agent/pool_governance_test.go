package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/tools"
)

func TestPoolFinalizesGlobalApprovalWithOwnerContext(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	broker := tools.NewBroker(nil)
	pool := &Pool{
		cfg: &config.Config{
			ToolPolicyRaw: json.RawMessage(`{"ask":["read"]}`),
		},
		approvalBroker: broker,
	}
	agent := &Agent{
		ID:           "agent-1",
		WorkspaceDir: workspace,
	}
	registry := tools.New(workspace, t.TempDir(), agent.ID)
	pool.finalizeToolRegistry(registry, agent, "session-1")

	done := make(chan error, 1)
	go func() {
		_, err := registry.Execute(
			context.Background(),
			"read",
			json.RawMessage(`{"file_path":"note.txt"}`),
		)
		done <- err
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		pending := broker.ListPending("")
		if len(pending) == 1 {
			if pending[0].AgentID != agent.ID || pending[0].SessionID != "session-1" {
				t.Fatalf("approval owner mismatch: %+v", pending[0])
			}
			if err := broker.Decide(pending[0].ID, tools.ApprovalDecision{Approved: false}); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("approval request was not created")
}

func TestPoolInvalidPolicyRemovesAllTools(t *testing.T) {
	pool := &Pool{
		cfg: &config.Config{
			ToolPolicyRaw: json.RawMessage(`{"profile":"invalid"}`),
		},
	}
	agent := &Agent{ID: "agent-1", WorkspaceDir: t.TempDir()}
	registry := tools.New(agent.WorkspaceDir, t.TempDir(), agent.ID)
	pool.finalizeToolRegistry(registry, agent, "session-1")
	if len(registry.Definitions()) != 0 {
		t.Fatal("invalid Pool policy did not fail closed")
	}
}
