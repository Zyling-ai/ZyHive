package agentcli

import "strconv"

func init() {
	registerCommand(&command{
		name:    "goal",
		summary: "目标规划：目标 CRUD、进度、里程碑、定期检查",
		actions: []*action{
			{name: "list", summary: "列出目标", usage: "zyhive goal list [--agent AGENT]", run: runGoalList},
			{name: "get", summary: "查看目标", usage: "zyhive goal get <goalId>", run: runGoalGet},
			{name: "create", summary: "创建目标", usage: "zyhive goal create --data JSON --yes", run: runGoalCreate},
			{name: "update", summary: "更新目标", usage: "zyhive goal update <goalId> --data JSON --yes", run: runGoalUpdate},
			{name: "delete", summary: "删除目标", usage: "zyhive goal delete <goalId> --yes", run: runGoalDelete},
			{name: "progress", summary: "更新进度", usage: "zyhive goal progress <goalId> <0-100> --yes", run: runGoalProgress},
			{name: "milestone", summary: "设置里程碑完成状态", usage: "zyhive goal milestone <goalId> <milestoneId> [--done|--not-done] --yes", run: runGoalMilestone},
			{name: "checks", summary: "列出检查计划", usage: "zyhive goal checks <goalId>", run: runGoalChecks},
			{name: "check-add", summary: "新增检查计划", usage: "zyhive goal check-add <goalId> --data JSON --yes", run: runGoalCheckAdd},
			{name: "check-update", summary: "更新检查计划", usage: "zyhive goal check-update <goalId> <checkId> --data JSON --yes", run: runGoalCheckUpdate},
			{name: "check-remove", summary: "删除检查计划", usage: "zyhive goal check-remove <goalId> <checkId> --yes", run: runGoalCheckRemove},
			{name: "check-run", summary: "立即运行检查", usage: "zyhive goal check-run <goalId> <checkId>", run: runGoalCheckRun},
			{name: "records", summary: "查看检查记录", usage: "zyhive goal records <goalId>", run: runGoalRecords},
		},
	})
}

func runGoalList(c *ctx, args []string) error {
	fs := newFlagSet("goal list")
	var agentID string
	fs.StringVar(&agentID, "agent", "", "按 agent 过滤")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	resp, err := c.get("/api/goals" + q(map[string]string{"agentId": agentID}))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalGet(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive goal get <goalId>")
	}
	resp, err := c.get("/api/goals/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalCreate(c *ctx, args []string) error {
	fs := newFlagSet("goal create")
	var dataFlag string
	fs.StringVar(&dataFlag, "data", "", "goal.Goal JSON")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if dataFlag == "" {
		return usageErr("用法: zyhive goal create --data JSON --yes")
	}
	if err := c.confirm("创建目标"); err != nil {
		return err
	}
	body, err := bodyFromData(dataFlag, nil)
	if err != nil {
		return err
	}
	resp, err := c.post("/api/goals", body)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalUpdate(c *ctx, args []string) error {
	fs := newFlagSet("goal update")
	var dataFlag string
	fs.StringVar(&dataFlag, "data", "", "goal.Goal JSON patch")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id := arg(pos, 0)
	if id == "" || dataFlag == "" {
		return usageErr("用法: zyhive goal update <goalId> --data JSON --yes")
	}
	if err := c.confirm("更新目标 %s", id); err != nil {
		return err
	}
	body, err := bodyFromData(dataFlag, nil)
	if err != nil {
		return err
	}
	resp, err := c.patch("/api/goals/"+id, body)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalDelete(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive goal delete <goalId> --yes")
	}
	if err := c.confirm("删除目标 %s", id); err != nil {
		return err
	}
	resp, err := c.del("/api/goals/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalProgress(c *ctx, args []string) error {
	id, raw := arg(args, 0), arg(args, 1)
	if id == "" || raw == "" {
		return usageErr("用法: zyhive goal progress <goalId> <0-100> --yes")
	}
	progress, err := strconv.Atoi(raw)
	if err != nil {
		return usageErr("progress 必须是数字")
	}
	if err := c.confirm("更新目标 %s 进度为 %d", id, progress); err != nil {
		return err
	}
	resp, err := c.patch("/api/goals/"+id+"/progress", map[string]any{"progress": progress})
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalMilestone(c *ctx, args []string) error {
	fs := newFlagSet("goal milestone")
	done := true
	fs.BoolVar(&done, "done", true, "标记完成")
	notDone := fs.Bool("not-done", false, "标记未完成")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if *notDone {
		done = false
	}
	goalID, mid := arg(pos, 0), arg(pos, 1)
	if goalID == "" || mid == "" {
		return usageErr("用法: zyhive goal milestone <goalId> <milestoneId> [--done|--not-done] --yes")
	}
	if err := c.confirm("更新目标 %s 里程碑 %s", goalID, mid); err != nil {
		return err
	}
	resp, err := c.patch("/api/goals/"+goalID+"/milestones/"+mid, map[string]any{"done": done})
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalChecks(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive goal checks <goalId>")
	}
	resp, err := c.get("/api/goals/" + id + "/checks")
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalCheckAdd(c *ctx, args []string) error {
	return goalCheckData(c, args, "POST", false)
}

func runGoalCheckUpdate(c *ctx, args []string) error {
	return goalCheckData(c, args, "PATCH", true)
}

func goalCheckData(c *ctx, args []string, method string, needCheckID bool) error {
	fs := newFlagSet("goal check")
	var dataFlag string
	fs.StringVar(&dataFlag, "data", "", "GoalCheck JSON")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	goalID, checkID := arg(pos, 0), arg(pos, 1)
	if goalID == "" || dataFlag == "" || (needCheckID && checkID == "") {
		return usageErr("用法: zyhive goal check-%s <goalId> [checkId] --data JSON --yes", map[bool]string{true: "update", false: "add"}[needCheckID])
	}
	if err := c.confirm("更新目标 %s 检查计划", goalID); err != nil {
		return err
	}
	body, err := bodyFromData(dataFlag, nil)
	if err != nil {
		return err
	}
	path := "/api/goals/" + goalID + "/checks"
	if needCheckID {
		path += "/" + checkID
	}
	resp, err := c.request(method, path, body)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalCheckRemove(c *ctx, args []string) error {
	goalID, checkID := arg(args, 0), arg(args, 1)
	if goalID == "" || checkID == "" {
		return usageErr("用法: zyhive goal check-remove <goalId> <checkId> --yes")
	}
	if err := c.confirm("删除目标 %s 的检查计划 %s", goalID, checkID); err != nil {
		return err
	}
	resp, err := c.del("/api/goals/" + goalID + "/checks/" + checkID)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalCheckRun(c *ctx, args []string) error {
	goalID, checkID := arg(args, 0), arg(args, 1)
	if goalID == "" || checkID == "" {
		return usageErr("用法: zyhive goal check-run <goalId> <checkId>")
	}
	resp, err := c.post("/api/goals/"+goalID+"/checks/"+checkID+"/run", nil)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runGoalRecords(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive goal records <goalId>")
	}
	resp, err := c.get("/api/goals/" + id + "/check-records")
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
