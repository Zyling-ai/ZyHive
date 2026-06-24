package agentcli

import (
	"errors"
	"fmt"
	"strings"
)

func init() {
	registerCommand(&command{
		name:    "chat",
		summary: "对话：发送消息、查看/管理成员会话",
		actions: []*action{
			{name: "send", summary: "向成员发送流式对话", usage: "zyhive chat send <agentId> <message|-> [--session SID] [--context TEXT] [--json]", run: runChatSend},
			{name: "sessions", summary: "列出某成员会话", usage: "zyhive chat sessions <agentId>", run: runChatSessions},
			{name: "session", summary: "读取某成员会话", usage: "zyhive chat session <agentId> <sessionId>", run: runChatSessionGet},
			{name: "rename-session", summary: "重命名会话", usage: "zyhive chat rename-session <agentId> <sessionId> --title TITLE --yes", run: runChatRenameSession},
			{name: "delete-session", summary: "删除会话", usage: "zyhive chat delete-session <agentId> <sessionId> --yes", run: runChatDeleteSession},
		},
	})
}

func runChatSend(c *ctx, args []string) error {
	fs := newFlagSet("chat send")
	var sessionID, extraCtx, scenario, skillID, images string
	fs.StringVar(&sessionID, "session", "", "会话 ID（留空自动创建）")
	fs.StringVar(&extraCtx, "context", "", "额外上下文")
	fs.StringVar(&scenario, "scenario", "", "场景")
	fs.StringVar(&skillID, "skill", "", "技能 ID")
	fs.StringVar(&images, "images", "", "逗号分隔图片路径/URL")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	agentID := arg(pos, 0)
	message := arg(pos, 1)
	if agentID == "" || message == "" {
		return usageErr("用法: zyhive chat send <agentId> <message|-> [--session SID]")
	}
	message, err = stdinIfDash(message)
	if err != nil {
		return err
	}

	body := map[string]any{
		"message":   message,
		"sessionId": sessionID,
		"context":   extraCtx,
		"scenario":  scenario,
		"skillId":   skillID,
		"images":    parseCSV(images),
	}

	var events []SSEEvent
	var text strings.Builder
	var finalSession string
	err = c.stream("/api/agents/"+agentID+"/chat", body, func(ev SSEEvent) error {
		events = append(events, ev)
		typ := fmt.Sprintf("%v", ev["type"])
		switch typ {
		case "text_delta":
			if s, ok := ev["text"].(string); ok {
				text.WriteString(s)
				if !c.json {
					fmt.Fprint(c.out, s)
				}
			}
		case "tool_call":
			if !c.json && !c.quiet {
				fmt.Fprint(c.out, "\n[tool_call]\n")
			}
		case "tool_result":
			if !c.json && !c.quiet {
				fmt.Fprint(c.out, "\n[tool_result]\n")
			}
		case "done":
			if s, ok := ev["sessionId"].(string); ok {
				finalSession = s
			}
		case "error":
			if msg, ok := ev["error"].(string); ok {
				return errors.New(msg)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if c.json {
		return c.emitJSON(map[string]any{
			"ok":        true,
			"agentId":   agentID,
			"sessionId": finalSession,
			"text":      text.String(),
			"events":    events,
		})
	}
	fmt.Fprintln(c.out)
	if finalSession != "" && !c.quiet {
		fmt.Fprintf(c.out, "\nSession: %s\n", finalSession)
	}
	return nil
}

func runChatSessions(c *ctx, args []string) error {
	agentID := arg(args, 0)
	if agentID == "" {
		return usageErr("用法: zyhive chat sessions <agentId>")
	}
	resp, err := c.get("/api/agents/" + agentID + "/sessions")
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runChatSessionGet(c *ctx, args []string) error {
	agentID, sid := arg(args, 0), arg(args, 1)
	if agentID == "" || sid == "" {
		return usageErr("用法: zyhive chat session <agentId> <sessionId>")
	}
	resp, err := c.get("/api/agents/" + agentID + "/sessions/" + sid)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runChatRenameSession(c *ctx, args []string) error {
	fs := newFlagSet("chat rename-session")
	var title string
	fs.StringVar(&title, "title", "", "新标题")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	agentID, sid := arg(pos, 0), arg(pos, 1)
	if agentID == "" || sid == "" || title == "" {
		return usageErr("用法: zyhive chat rename-session <agentId> <sessionId> --title TITLE --yes")
	}
	if err := c.confirm("重命名会话 %s/%s", agentID, sid); err != nil {
		return err
	}
	resp, err := c.patch("/api/sessions/"+agentID+"/"+sid, map[string]any{"title": title})
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runChatDeleteSession(c *ctx, args []string) error {
	agentID, sid := arg(args, 0), arg(args, 1)
	if agentID == "" || sid == "" {
		return usageErr("用法: zyhive chat delete-session <agentId> <sessionId> --yes")
	}
	if err := c.confirm("删除会话 %s/%s", agentID, sid); err != nil {
		return err
	}
	resp, err := c.del("/api/sessions/" + agentID + "/" + sid)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
