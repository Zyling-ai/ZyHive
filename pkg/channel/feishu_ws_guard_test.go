package channel

import (
	"context"
	"errors"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/netguard"
)

func TestValidateFeishuWebSocketURL(t *testing.T) {
	ctx := context.Background()
	for _, host := range []string{
		"msg-frontier.feishu.cn",
		"msg-frontier.larksuite.com",
	} {
		if !isAllowedFeishuWebSocketHost(host) {
			t.Fatalf("official Feishu/Lark host rejected: %s", host)
		}
	}

	// Host allowlisting is deterministic; network resolution is covered by
	// pkg/netguard tests and must not make this unit test depend on public DNS.
	for _, target := range []string{
		"ws://127.0.0.1/ws",
		"https://msg-frontier.feishu.cn/ws",
		"wss://203.0.113.10/ws",
	} {
		if err := validateFeishuWebSocketURL(ctx, target); !errors.Is(err, netguard.ErrBlocked) {
			t.Fatalf("%s: expected ErrBlocked, got %v", target, err)
		}
	}
}
