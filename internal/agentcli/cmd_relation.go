package agentcli

func init() {
	registerCommand(&command{
		name:    "relation",
		summary: "团队关系图：读写成员关系、增删边",
		actions: []*action{
			{name: "get", summary: "读取成员关系文件", usage: "zyhive relation get <agentId>", run: runRelationGet},
			{name: "set", summary: "覆盖成员关系文件", usage: "zyhive relation set <agentId> <markdown|-> --yes", run: runRelationSet},
			{name: "graph", summary: "查看团队图谱", usage: "zyhive relation graph", run: runRelationGraph},
			{name: "edge-add", summary: "添加/更新关系边", usage: "zyhive relation edge-add --from A --to B --type 平级协作 [--strength strong] --yes", run: runRelationEdgeAdd},
			{name: "edge-delete", summary: "删除关系边", usage: "zyhive relation edge-delete --from A --to B --yes", run: runRelationEdgeDelete},
			{name: "clear", summary: "清空所有关系", usage: "zyhive relation clear --yes", run: runRelationClear},
		},
	})
}

func runRelationGet(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive relation get <agentId>")
	}
	resp, err := c.get("/api/agents/" + id + "/relations")
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runRelationSet(c *ctx, args []string) error {
	id, content := arg(args, 0), arg(args, 1)
	if id == "" || content == "" {
		return usageErr("用法: zyhive relation set <agentId> <markdown|-> --yes")
	}
	if err := c.confirm("覆盖 %s 关系文件", id); err != nil {
		return err
	}
	content, err := stdinIfDash(content)
	if err != nil {
		return err
	}
	resp, err := c.put("/api/agents/"+id+"/relations", []byte(content))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runRelationGraph(c *ctx, _ []string) error {
	resp, err := c.get("/api/team/graph")
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runRelationEdgeAdd(c *ctx, args []string) error {
	fs := newFlagSet("relation edge-add")
	var from, to, typ, strength, desc string
	fs.StringVar(&from, "from", "", "源 agent")
	fs.StringVar(&to, "to", "", "目标 agent")
	fs.StringVar(&typ, "type", "平级协作", "关系类型")
	fs.StringVar(&strength, "strength", "medium", "强度")
	fs.StringVar(&desc, "desc", "", "说明")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if from == "" || to == "" {
		return usageErr("用法: zyhive relation edge-add --from A --to B --type 平级协作 --yes")
	}
	if err := c.confirm("添加关系 %s -> %s", from, to); err != nil {
		return err
	}
	resp, err := c.put("/api/team/relations/edge", map[string]any{"from": from, "to": to, "type": typ, "strength": strength, "desc": desc})
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runRelationEdgeDelete(c *ctx, args []string) error {
	fs := newFlagSet("relation edge-delete")
	var from, to string
	fs.StringVar(&from, "from", "", "源 agent")
	fs.StringVar(&to, "to", "", "目标 agent")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if from == "" || to == "" {
		return usageErr("用法: zyhive relation edge-delete --from A --to B --yes")
	}
	if err := c.confirm("删除关系 %s -> %s", from, to); err != nil {
		return err
	}
	resp, err := c.del("/api/team/relations/edge" + q(map[string]string{"from": from, "to": to}))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runRelationClear(c *ctx, _ []string) error {
	if err := c.confirm("清空所有团队关系"); err != nil {
		return err
	}
	resp, err := c.del("/api/team/relations")
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
