package channel

import (
	"context"
	"errors"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/netguard"
)

func TestValidateFeishuWebSocketURL(t *testing.T) {
	ctx := context.Background()
	if err := validateFeishuWebSocketURL(ctx, "wss://msg-frontier.feishu.cn/ws/v2?x=1"); err != nil {
		t.Fatalf("official Feishu host rejected: %v", err)
	}
	if err := validateFeishuWebSocketURL(ctx, "wss://msg-frontier.larksuite.com/ws/v2"); err != nil {
		t.Fatalf("official Lark host rejected: %v", err)
	}

	// Loopback / wrong scheme are blocked by netguard before host allowlisting.
	for _, target := range []string{
		"ws://127.0.0.1/ws",
		"https://msg-frontier.feishu.cn/ws",
	} {
		if err := validateFeishuWebSocketURL(ctx, target); !errors.Is(err, netguard.ErrBlocked) {
			t.Fatalf("%s: expected ErrBlocked, got %v", target, err)
		}
	}

	// Public but non-Feishu hosts must fail closed after netguard accepts the URL shape.
	if err := validateFeishuWebSocketURL(ctx, "wss://203.0.113.10/ws"); !errors.Is(err, netguard.ErrBlocked) {
		t.Fatalf("non-Feishu public IP: expected ErrBlocked, got %v", err)
	}
}
