package agentcli

func init() {
	registerCommand(&command{
		name:    "skill",
		summary: "全局技能：列表、安装、删除",
		actions: []*action{
			{name: "list", summary: "列出技能", usage: "zyhive skill list", run: func(c *ctx, _ []string) error {
				resp, err := c.get("/api/skills")
				if err != nil {
					return err
				}
				return c.result(resp, nil)
			}},
			{name: "install", summary: "安装技能", usage: "zyhive skill install --data JSON --yes", run: func(c *ctx, args []string) error {
				fs := newFlagSet("skill install")
				var dataFlag string
				fs.StringVar(&dataFlag, "data", "", "install JSON body")
				if _, err := parseFlags(fs, args); err != nil {
					return err
				}
				if dataFlag == "" {
					return usageErr("用法: zyhive skill install --data JSON --yes")
				}
				if err := c.confirm("安装技能"); err != nil {
					return err
				}
				body, err := bodyFromData(dataFlag, nil)
				if err != nil {
					return err
				}
				resp, err := c.post("/api/skills/install", body)
				if err != nil {
					return err
				}
				return c.result(resp, nil)
			}},
			{name: "delete", summary: "删除技能", usage: "zyhive skill delete <id> --yes", run: func(c *ctx, args []string) error {
				id := arg(args, 0)
				if id == "" {
					return usageErr("用法: zyhive skill delete <id> --yes")
				}
				if err := c.confirm("删除技能 %s", id); err != nil {
					return err
				}
				resp, err := c.del("/api/skills/" + id)
				if err != nil {
					return err
				}
				return c.result(resp, nil)
			}},
		},
	})

	registerCommand(&command{
		name:    "usage",
		summary: "Token 用量与费用统计",
		actions: []*action{
			{name: "summary", summary: "汇总", usage: "zyhive usage summary [--agent AGENT] [--from YYYY-MM-DD] [--to YYYY-MM-DD]", run: usageGet("/api/usage/summary")},
			{name: "timeline", summary: "时间线", usage: "zyhive usage timeline [--agent AGENT] [--from DATE] [--to DATE]", run: usageGet("/api/usage/timeline")},
			{name: "records", summary: "明细", usage: "zyhive usage records [--agent AGENT] [--session SID] [--limit N]", run: usageGet("/api/usage/records")},
		},
	})

	registerCommand(&command{
		name:    "system",
		summary: "系统状态、健康检查、统计",
		actions: []*action{
			{name: "status", summary: "详细状态", usage: "zyhive system status", run: simpleGet("/api/status")},
			{name: "stats", summary: "系统统计", usage: "zyhive system stats", run: simpleGet("/api/stats")},
			{name: "health", summary: "鉴权健康检查", usage: "zyhive system health", run: simpleGet("/api/health")},
			{name: "ready", summary: "公开 readiness", usage: "zyhive system ready", run: simpleGet("/readyz")},
		},
	})

	registerCommand(&command{
		name:    "conversations",
		summary: "管理员对话审计日志",
		actions: []*action{
			{name: "global", summary: "全局会话列表", usage: "zyhive conversations global", run: simpleGet("/api/conversations")},
			{name: "list", summary: "某成员会话列表", usage: "zyhive conversations list <agentId>", run: func(c *ctx, args []string) error {
				id := arg(args, 0)
				if id == "" {
					return usageErr("用法: zyhive conversations list <agentId>")
				}
				return simpleGet("/api/agents/"+id+"/conversations")(c, args)
			}},
			{name: "messages", summary: "某渠道消息", usage: "zyhive conversations messages <agentId> <channelId>", run: func(c *ctx, args []string) error {
				id, ch := arg(args, 0), arg(args, 1)
				if id == "" || ch == "" {
					return usageErr("用法: zyhive conversations messages <agentId> <channelId>")
				}
				return simpleGet("/api/agents/"+id+"/conversations/"+ch)(c, args)
			}},
		},
	})

	registerCommand(&command{
		name:    "approval",
		summary: "工具审批：查看 pending、批准、拒绝",
		actions: []*action{
			{name: "pending", summary: "待审批列表", usage: "zyhive approval pending", run: simpleGet("/api/approvals/pending")},
			{name: "approve", summary: "批准", usage: "zyhive approval approve <id> [--reason TEXT] --yes", run: approvalDecision("approve")},
			{name: "deny", summary: "拒绝", usage: "zyhive approval deny <id> [--reason TEXT] --yes", run: approvalDecision("deny")},
		},
	})
}

func simpleGet(path string) func(c *ctx, args []string) error {
	return func(c *ctx, _ []string) error {
		resp, err := c.get(path)
		if err != nil {
			return err
		}
		return c.result(resp, nil)
	}
}

func usageGet(path string) func(c *ctx, args []string) error {
	return func(c *ctx, args []string) error {
		fs := newFlagSet("usage")
		var agent, from, to, session, limit string
		fs.StringVar(&agent, "agent", "", "agentId")
		fs.StringVar(&from, "from", "", "开始日期")
		fs.StringVar(&to, "to", "", "结束日期")
		fs.StringVar(&session, "session", "", "sessionId")
		fs.StringVar(&limit, "limit", "", "限制条数")
		if _, err := parseFlags(fs, args); err != nil {
			return err
		}
		resp, err := c.get(path + q(map[string]string{"agentId": agent, "from": from, "to": to, "sessionId": session, "limit": limit}))
		if err != nil {
			return err
		}
		return c.result(resp, nil)
	}
}

func approvalDecision(decision string) func(c *ctx, args []string) error {
	return func(c *ctx, args []string) error {
		fs := newFlagSet("approval " + decision)
		var reason string
		fs.StringVar(&reason, "reason", "", "理由")
		pos, err := parseFlags(fs, args)
		if err != nil {
			return err
		}
		id := arg(pos, 0)
		if id == "" {
			return usageErr("用法: zyhive approval %s <id> [--reason TEXT] --yes", decision)
		}
		if err := c.confirm("%s 审批 %s", decision, id); err != nil {
			return err
		}
		resp, err := c.post("/api/approvals/"+id+"/"+decision, map[string]any{"reason": reason, "by": "zyhive-cli"})
		if err != nil {
			return err
		}
		return c.result(resp, nil)
	}
}
