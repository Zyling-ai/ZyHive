package agentcli

func init() {
	registerCommand(&command{
		name:    "cron",
		summary: "定时任务：CRUD、立即运行、历史记录",
		actions: []*action{
			{name: "list", summary: "列出定时任务", usage: "zyhive cron list [--agent AGENT|--global]", run: runCronList},
			{name: "add", summary: "创建定时任务", usage: "zyhive cron add --agent AGENT --name NAME --expr CRON --message TEXT [--tz Asia/Shanghai] --yes", run: runCronAdd},
			{name: "update", summary: "更新定时任务", usage: "zyhive cron update <jobId> --data JSON --yes", run: runCronUpdate},
			{name: "remove", summary: "删除定时任务", usage: "zyhive cron remove <jobId> --yes", run: runCronRemove},
			{name: "run", summary: "立即触发定时任务", usage: "zyhive cron run <jobId>", run: runCronRun},
			{name: "runs", summary: "查看运行历史", usage: "zyhive cron runs <jobId>", run: runCronRuns},
			{name: "enable", summary: "重新启用任务", usage: "zyhive cron enable <jobId>", run: runCronEnable},
		},
	})
}

func runCronList(c *ctx, args []string) error {
	fs := newFlagSet("cron list")
	var agentID string
	var global bool
	fs.StringVar(&agentID, "agent", "", "按 agentId 过滤")
	fs.BoolVar(&global, "global", false, "仅列出全局任务")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if global {
		agentID = "__global__"
	}
	resp, err := c.get("/api/cron" + q(map[string]string{"agentId": agentID}))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runCronAdd(c *ctx, args []string) error {
	fs := newFlagSet("cron add")
	var id, agentID, name, expr, tz, message, model, delivery, dataFlag string
	var enabled bool
	fs.StringVar(&id, "id", "", "任务 ID（可选）")
	fs.StringVar(&agentID, "agent", "", "执行成员 agentId")
	fs.StringVar(&name, "name", "", "任务名称")
	fs.StringVar(&expr, "expr", "", "cron 表达式")
	fs.StringVar(&tz, "tz", "Asia/Shanghai", "时区")
	fs.StringVar(&message, "message", "", "发送给 agent 的 prompt（可用 - 读 stdin）")
	fs.StringVar(&model, "model", "", "覆盖模型（可选）")
	fs.StringVar(&delivery, "delivery", "announce", "announce 或 none")
	fs.StringVar(&dataFlag, "data", "", "完整 cron.Job JSON")
	fs.BoolVar(&enabled, "enabled", true, "是否启用")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := c.confirm("创建 cron 任务"); err != nil {
		return err
	}
	msg, err := stdinIfDash(message)
	if err != nil {
		return err
	}
	body := map[string]any{
		"id":      id,
		"name":    name,
		"enabled": enabled,
		"agentId": agentID,
		"schedule": map[string]any{
			"kind": "cron",
			"expr": expr,
			"tz":   tz,
		},
		"payload": map[string]any{
			"kind":    "agentTurn",
			"message": msg,
			"model":   model,
		},
		"delivery": map[string]any{"mode": delivery},
	}
	if dataFlag == "" && (agentID == "" || name == "" || expr == "" || msg == "") {
		return usageErr("cron add 需要 --agent/--name/--expr/--message，或提供 --data JSON")
	}
	reqBody, err := bodyFromData(dataFlag, body)
	if err != nil {
		return err
	}
	resp, err := c.post("/api/cron", reqBody)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runCronUpdate(c *ctx, args []string) error {
	fs := newFlagSet("cron update")
	var dataFlag string
	fs.StringVar(&dataFlag, "data", "", "cron.Job JSON patch（可用 -）")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id := arg(pos, 0)
	if id == "" || dataFlag == "" {
		return usageErr("用法: zyhive cron update <jobId> --data JSON --yes")
	}
	if err := c.confirm("更新 cron 任务 %s", id); err != nil {
		return err
	}
	body, err := bodyFromData(dataFlag, nil)
	if err != nil {
		return err
	}
	resp, err := c.patch("/api/cron/"+id, body)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runCronRemove(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive cron remove <jobId> --yes")
	}
	if err := c.confirm("删除 cron 任务 %s", id); err != nil {
		return err
	}
	resp, err := c.del("/api/cron/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runCronRun(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive cron run <jobId>")
	}
	resp, err := c.post("/api/cron/"+id+"/run", nil)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runCronRuns(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive cron runs <jobId>")
	}
	resp, err := c.get("/api/cron/" + id + "/runs")
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runCronEnable(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive cron enable <jobId>")
	}
	resp, err := c.put("/api/cron/"+id+"/enable", nil)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
