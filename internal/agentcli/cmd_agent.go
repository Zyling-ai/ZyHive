package agentcli

import "fmt"

func init() {
	registerCommand(&command{
		name:    "agent",
		summary: "成员管理：列出、创建、更新、删除、消息",
		actions: []*action{
			{name: "list", summary: "列出 AI 成员", usage: "zyhive agent list [--json]", run: runAgentList},
			{name: "get", summary: "查看成员详情", usage: "zyhive agent get <agentId> [--json]", run: runAgentGet},
			{name: "create", summary: "创建成员", usage: "zyhive agent create --id ID --name NAME [--model MODEL|--model-id ID] [--description TEXT] [--data JSON] --yes", run: runAgentCreate},
			{name: "update", summary: "更新成员", usage: "zyhive agent update <agentId> [--name NAME] [--description TEXT] [--model MODEL|--model-id ID] [--data JSON] --yes", run: runAgentUpdate},
			{name: "delete", summary: "删除成员", usage: "zyhive agent delete <agentId> --yes", run: runAgentDelete},
			{name: "start", summary: "启动成员（若服务端支持）", usage: "zyhive agent start <agentId>", run: runAgentStart},
			{name: "stop", summary: "停止成员（若服务端支持）", usage: "zyhive agent stop <agentId>", run: runAgentStop},
			{name: "message", summary: "向成员发送同步消息", usage: "zyhive agent message <agentId> <message|- > [--from AGENT]", run: runAgentMessage},
		},
	})
}

func runAgentList(c *ctx, _ []string) error {
	data, err := c.get("/api/agents")
	if err != nil {
		return err
	}
	return c.result(data, func(v any) {
		renderRows(c, v, []string{"id", "name", "status", "model", "description"}, []string{"ID", "Name", "Status", "Model", "Description"})
	})
}

func runAgentGet(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive agent get <agentId>")
	}
	data, err := c.get("/api/agents/" + id)
	if err != nil {
		return err
	}
	return c.result(data, nil)
}

func runAgentCreate(c *ctx, args []string) error {
	fs := newFlagSet("agent create")
	var id, name, desc, model, modelID, toolIDs, skillIDs, avatar, dataFlag string
	fs.StringVar(&id, "id", "", "成员 ID")
	fs.StringVar(&name, "name", "", "成员名称")
	fs.StringVar(&desc, "description", "", "描述")
	fs.StringVar(&model, "model", "", "provider/model")
	fs.StringVar(&modelID, "model-id", "", "模型条目 ID")
	fs.StringVar(&toolIDs, "tool-ids", "", "逗号分隔 tool IDs")
	fs.StringVar(&skillIDs, "skill-ids", "", "逗号分隔 skill IDs")
	fs.StringVar(&avatar, "avatar-color", "", "头像颜色")
	fs.StringVar(&dataFlag, "data", "", "完整 JSON body（可用 - 读 stdin）")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := c.confirm("创建成员"); err != nil {
		return err
	}
	body := map[string]any{}
	addIf(body, "id", id)
	addIf(body, "name", name)
	addIf(body, "description", desc)
	addIf(body, "model", model)
	addIf(body, "modelId", modelID)
	addIf(body, "avatarColor", avatar)
	addIfSlice(body, "toolIds", parseCSV(toolIDs))
	addIfSlice(body, "skillIds", parseCSV(skillIDs))
	if dataFlag == "" && (id == "" || name == "") {
		return usageErr("创建成员需要 --id 和 --name，或提供 --data JSON")
	}
	reqBody, err := bodyFromData(dataFlag, body)
	if err != nil {
		return err
	}
	resp, err := c.post("/api/agents", reqBody)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runAgentUpdate(c *ctx, args []string) error {
	fs := newFlagSet("agent update")
	var name, desc, model, modelID, toolIDs, skillIDs, avatar, dataFlag string
	fs.StringVar(&name, "name", "", "成员名称")
	fs.StringVar(&desc, "description", "", "描述")
	fs.StringVar(&model, "model", "", "provider/model")
	fs.StringVar(&modelID, "model-id", "", "模型条目 ID")
	fs.StringVar(&toolIDs, "tool-ids", "", "逗号分隔 tool IDs")
	fs.StringVar(&skillIDs, "skill-ids", "", "逗号分隔 skill IDs")
	fs.StringVar(&avatar, "avatar-color", "", "头像颜色")
	fs.StringVar(&dataFlag, "data", "", "完整 JSON patch")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id := arg(pos, 0)
	if id == "" {
		return usageErr("用法: zyhive agent update <agentId> [flags]")
	}
	if err := c.confirm("更新成员 %s", id); err != nil {
		return err
	}
	body := map[string]any{}
	addIf(body, "name", name)
	addIf(body, "description", desc)
	addIf(body, "model", model)
	addIf(body, "modelId", modelID)
	addIf(body, "avatarColor", avatar)
	addIfSlice(body, "toolIds", parseCSV(toolIDs))
	addIfSlice(body, "skillIds", parseCSV(skillIDs))
	reqBody, err := bodyFromData(dataFlag, body)
	if err != nil {
		return err
	}
	resp, err := c.patch("/api/agents/"+id, reqBody)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runAgentDelete(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive agent delete <agentId> --yes")
	}
	if err := c.confirm("删除成员 %s", id); err != nil {
		return err
	}
	resp, err := c.del("/api/agents/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runAgentStart(c *ctx, args []string) error {
	return runAgentLifecycle(c, args, "start")
}

func runAgentStop(c *ctx, args []string) error {
	return runAgentLifecycle(c, args, "stop")
}

func runAgentLifecycle(c *ctx, args []string, verb string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive agent %s <agentId>", verb)
	}
	resp, err := c.post(fmt.Sprintf("/api/agents/%s/%s", id, verb), nil)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runAgentMessage(c *ctx, args []string) error {
	fs := newFlagSet("agent message")
	var from string
	fs.StringVar(&from, "from", "", "发送方 agentId")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id := arg(pos, 0)
	msg := arg(pos, 1)
	if id == "" || msg == "" {
		return usageErr("用法: zyhive agent message <agentId> <message|-> [--from AGENT]")
	}
	msg, err = stdinIfDash(msg)
	if err != nil {
		return err
	}
	resp, err := c.post("/api/agents/"+id+"/message", map[string]any{"message": msg, "fromAgentId": from})
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
