package agentcli

func init() {
	registerCommand(&command{
		name:    "file",
		summary: "成员 workspace 文件：读、写、删",
		actions: []*action{
			{name: "read", summary: "读取文件或目录", usage: "zyhive file read <agentId> <path> [--tree]", run: runFileRead},
			{name: "write", summary: "写入文件", usage: "zyhive file write <agentId> <path> <content|-> --yes", run: runFileWrite},
			{name: "delete", summary: "删除文件/目录", usage: "zyhive file delete <agentId> <path> --yes", run: runFileDelete},
		},
	})
}

func runFileRead(c *ctx, args []string) error {
	fs := newFlagSet("file read")
	var tree bool
	fs.BoolVar(&tree, "tree", false, "递归树")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	agentID, path := arg(pos, 0), arg(pos, 1)
	if agentID == "" {
		return usageErr("用法: zyhive file read <agentId> <path> [--tree]")
	}
	if path == "" {
		path = "."
	}
	resp, err := c.get(slashPath("api/agents", agentID, "files", path) + q(map[string]string{"tree": map[bool]string{true: "true", false: ""}[tree]}))
	if err != nil {
		return err
	}
	return c.result(resp, func(v any) {
		if m := asMap(v); m != nil && str(m, "content") != "" {
			c.printf("%s", str(m, "content"))
			return
		}
		_ = c.emitJSON(v)
	})
}

func runFileWrite(c *ctx, args []string) error {
	agentID, path, content := arg(args, 0), arg(args, 1), arg(args, 2)
	if agentID == "" || path == "" || content == "" {
		return usageErr("用法: zyhive file write <agentId> <path> <content|-> --yes")
	}
	if err := c.confirm("写入 %s:%s", agentID, path); err != nil {
		return err
	}
	content, err := stdinIfDash(content)
	if err != nil {
		return err
	}
	resp, err := c.put(slashPath("api/agents", agentID, "files", path), map[string]any{"content": content})
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runFileDelete(c *ctx, args []string) error {
	agentID, path := arg(args, 0), arg(args, 1)
	if agentID == "" || path == "" {
		return usageErr("用法: zyhive file delete <agentId> <path> --yes")
	}
	if err := c.confirm("删除 %s:%s", agentID, path); err != nil {
		return err
	}
	resp, err := c.del(slashPath("api/agents", agentID, "files", path))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
