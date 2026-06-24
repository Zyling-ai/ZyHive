package agentcli

func init() {
	registerCommand(&command{
		name:    "task",
		summary: "后台子成员任务：派遣、查看、终止、可派遣对象",
		actions: []*action{
			{name: "list", summary: "列出任务", usage: "zyhive task list [--agent AGENT] [--status running|done|error] [--session SID]", run: runTaskList},
			{name: "spawn", summary: "派遣后台任务", usage: "zyhive task spawn --agent AGENT --task TEXT [--spawned-by AGENT] [--label LABEL] [--model MODEL] --yes", run: runTaskSpawn},
			{name: "get", summary: "查看任务详情", usage: "zyhive task get <taskId>", run: runTaskGet},
			{name: "kill", summary: "终止任务", usage: "zyhive task kill <taskId> --yes", run: runTaskKill},
			{name: "eligible", summary: "查看可派遣对象", usage: "zyhive task eligible --from AGENT [--mode task|report]", run: runTaskEligible},
		},
	})
}

func runTaskList(c *ctx, args []string) error {
	fs := newFlagSet("task list")
	var agentID, status, sessionID string
	fs.StringVar(&agentID, "agent", "", "agentId")
	fs.StringVar(&status, "status", "", "状态")
	fs.StringVar(&sessionID, "session", "", "父会话 ID")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	resp, err := c.get("/api/tasks" + q(map[string]string{"agentId": agentID, "status": status, "sessionId": sessionID}))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runTaskSpawn(c *ctx, args []string) error {
	fs := newFlagSet("task spawn")
	var agentID, task, label, model, spawnedBy, taskType, background, deliverable, priority, projectID, dataFlag string
	fs.StringVar(&agentID, "agent", "", "执行任务的 agentId")
	fs.StringVar(&task, "task", "", "任务描述（可用 - 读 stdin）")
	fs.StringVar(&label, "label", "", "任务标签")
	fs.StringVar(&model, "model", "", "模型覆盖")
	fs.StringVar(&spawnedBy, "spawned-by", "", "派遣方 agentId")
	fs.StringVar(&taskType, "type", "task", "task/report/system")
	fs.StringVar(&background, "background", "", "背景")
	fs.StringVar(&deliverable, "deliverable", "", "交付物")
	fs.StringVar(&priority, "priority", "", "high/normal/low")
	fs.StringVar(&projectID, "project", "", "共享项目 ID")
	fs.StringVar(&dataFlag, "data", "", "完整 JSON body")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := c.confirm("派遣后台任务"); err != nil {
		return err
	}
	taskText, err := stdinIfDash(task)
	if err != nil {
		return err
	}
	body := map[string]any{
		"agentId":         agentID,
		"task":            taskText,
		"label":           label,
		"model":           model,
		"spawnedBy":       spawnedBy,
		"taskType":        taskType,
		"background":      background,
		"deliverable":     deliverable,
		"priority":        priority,
		"sharedProjectId": projectID,
	}
	if dataFlag == "" && (agentID == "" || taskText == "") {
		return usageErr("task spawn 需要 --agent 和 --task，或提供 --data JSON")
	}
	reqBody, err := bodyFromData(dataFlag, body)
	if err != nil {
		return err
	}
	resp, err := c.post("/api/tasks", reqBody)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runTaskGet(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive task get <taskId>")
	}
	resp, err := c.get("/api/tasks/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runTaskKill(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive task kill <taskId> --yes")
	}
	if err := c.confirm("终止任务 %s", id); err != nil {
		return err
	}
	resp, err := c.del("/api/tasks/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runTaskEligible(c *ctx, args []string) error {
	fs := newFlagSet("task eligible")
	var from, mode string
	fs.StringVar(&from, "from", "", "派遣方 agentId")
	fs.StringVar(&mode, "mode", "task", "task 或 report")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if from == "" {
		return usageErr("用法: zyhive task eligible --from AGENT [--mode task|report]")
	}
	resp, err := c.get("/api/tasks/eligible" + q(map[string]string{"from": from, "mode": mode}))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
