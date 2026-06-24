package agentcli

func init() {
	registerCommand(&command{
		name:    "session",
		summary: "全局会话：跨成员列表、查看、重命名、删除",
		actions: []*action{
			{name: "list", summary: "列出全局会话", usage: "zyhive session list [--agent AGENT] [--limit N]", run: runSessionList},
			{name: "get", summary: "查看会话", usage: "zyhive session get <agentId> <sessionId>", run: runSessionGet},
			{name: "delete", summary: "删除会话", usage: "zyhive session delete <agentId> <sessionId> --yes", run: runSessionDelete},
			{name: "patch", summary: "更新会话元数据", usage: "zyhive session patch <agentId> <sessionId> --title TITLE --yes", run: runSessionPatch},
		},
	})
}

func runSessionList(c *ctx, args []string) error {
	fs := newFlagSet("session list")
	var agentID, limit string
	fs.StringVar(&agentID, "agent", "", "按 agent 过滤")
	fs.StringVar(&limit, "limit", "", "最大条数")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	resp, err := c.get("/api/sessions" + q(map[string]string{"agentId": agentID, "limit": limit}))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runSessionGet(c *ctx, args []string) error {
	agentID, sid := arg(args, 0), arg(args, 1)
	if agentID == "" || sid == "" {
		return usageErr("用法: zyhive session get <agentId> <sessionId>")
	}
	resp, err := c.get("/api/sessions/" + agentID + "/" + sid)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runSessionDelete(c *ctx, args []string) error {
	agentID, sid := arg(args, 0), arg(args, 1)
	if agentID == "" || sid == "" {
		return usageErr("用法: zyhive session delete <agentId> <sessionId> --yes")
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

func runSessionPatch(c *ctx, args []string) error {
	fs := newFlagSet("session patch")
	var title, dataFlag string
	fs.StringVar(&title, "title", "", "新标题")
	fs.StringVar(&dataFlag, "data", "", "JSON patch")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	agentID, sid := arg(pos, 0), arg(pos, 1)
	if agentID == "" || sid == "" {
		return usageErr("用法: zyhive session patch <agentId> <sessionId> --title TITLE --yes")
	}
	if err := c.confirm("更新会话 %s/%s", agentID, sid); err != nil {
		return err
	}
	body := map[string]any{"title": title}
	req, err := bodyFromData(dataFlag, body)
	if err != nil {
		return err
	}
	resp, err := c.patch("/api/sessions/"+agentID+"/"+sid, req)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
