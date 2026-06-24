package agentcli

func init() {
	registerCommand(&command{
		name:    "memory",
		summary: "成员记忆树：读取、写入、daily、蒸馏配置",
		actions: []*action{
			{name: "tree", summary: "列出记忆树", usage: "zyhive memory tree <agentId>", run: runMemoryTree},
			{name: "read", summary: "读取记忆文件", usage: "zyhive memory read <agentId> <path>", run: runMemoryRead},
			{name: "write", summary: "写入记忆文件", usage: "zyhive memory write <agentId> <path> <content|-> --yes", run: runMemoryWrite},
			{name: "daily", summary: "追加今日记忆日志", usage: "zyhive memory daily <agentId> <content|-> --yes", run: runMemoryDaily},
			{name: "config", summary: "读取或设置记忆配置", usage: "zyhive memory config <agentId> [--data JSON --yes]", run: runMemoryConfig},
			{name: "consolidate", summary: "立即触发记忆蒸馏", usage: "zyhive memory consolidate <agentId>", run: runMemoryConsolidate},
		},
	})
}

func runMemoryTree(c *ctx, args []string) error {
	agentID := arg(args, 0)
	if agentID == "" {
		return usageErr("用法: zyhive memory tree <agentId>")
	}
	resp, err := c.get("/api/agents/" + agentID + "/memory/tree")
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runMemoryRead(c *ctx, args []string) error {
	agentID, path := arg(args, 0), arg(args, 1)
	if agentID == "" || path == "" {
		return usageErr("用法: zyhive memory read <agentId> <path>")
	}
	resp, err := c.get(slashPath("api/agents", agentID, "memory/file", path))
	if err != nil {
		return err
	}
	return c.result(resp, func(v any) {
		if m := asMap(v); m != nil {
			c.printf("%s", str(m, "content"))
		}
	})
}

func runMemoryWrite(c *ctx, args []string) error {
	agentID, path, content := arg(args, 0), arg(args, 1), arg(args, 2)
	if agentID == "" || path == "" || content == "" {
		return usageErr("用法: zyhive memory write <agentId> <path> <content|-> --yes")
	}
	if err := c.confirm("写入 %s 的记忆文件 %s", agentID, path); err != nil {
		return err
	}
	content, err := stdinIfDash(content)
	if err != nil {
		return err
	}
	resp, err := c.put(slashPath("api/agents", agentID, "memory/file", path), []byte(content))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runMemoryDaily(c *ctx, args []string) error {
	agentID, content := arg(args, 0), arg(args, 1)
	if agentID == "" || content == "" {
		return usageErr("用法: zyhive memory daily <agentId> <content|-> --yes")
	}
	if err := c.confirm("追加 %s 今日记忆", agentID); err != nil {
		return err
	}
	content, err := stdinIfDash(content)
	if err != nil {
		return err
	}
	resp, err := c.post("/api/agents/"+agentID+"/memory/daily", []byte(content))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runMemoryConfig(c *ctx, args []string) error {
	fs := newFlagSet("memory config")
	var dataFlag string
	fs.StringVar(&dataFlag, "data", "", "配置 JSON；不填则读取")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	agentID := arg(pos, 0)
	if agentID == "" {
		return usageErr("用法: zyhive memory config <agentId> [--data JSON --yes]")
	}
	if dataFlag == "" {
		resp, err := c.get("/api/agents/" + agentID + "/memory/config")
		if err != nil {
			return err
		}
		return c.result(resp, nil)
	}
	if err := c.confirm("更新 %s 记忆配置", agentID); err != nil {
		return err
	}
	body, err := bodyFromData(dataFlag, nil)
	if err != nil {
		return err
	}
	resp, err := c.put("/api/agents/"+agentID+"/memory/config", body)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runMemoryConsolidate(c *ctx, args []string) error {
	agentID := arg(args, 0)
	if agentID == "" {
		return usageErr("用法: zyhive memory consolidate <agentId>")
	}
	resp, err := c.post("/api/agents/"+agentID+"/memory/consolidate", nil)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
