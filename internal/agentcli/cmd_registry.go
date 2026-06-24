package agentcli

func init() {
	registerRegistry("model", "/api/models", "模型注册表", "PATCH")
	registerRegistry("provider", "/api/providers", "Provider API Key 注册表", "PUT")
	registerRegistry("channel", "/api/channels", "全局渠道注册表", "PATCH")
	registerRegistry("tool", "/api/tools", "工具能力注册表", "PATCH")
	registerRegistry("acp", "/api/acp", "ACP 编程代理注册表", "PATCH")
}

func registerRegistry(name, base, summary, updateMethod string) {
	registerCommand(&command{
		name:    name,
		summary: summary + "：list/create/update/delete/test",
		actions: []*action{
			{name: "list", summary: "列出", usage: "zyhive " + name + " list", run: func(c *ctx, args []string) error {
				resp, err := c.get(base)
				if err != nil {
					return err
				}
				return c.result(resp, nil)
			}},
			{name: "create", summary: "创建", usage: "zyhive " + name + " create --data JSON --yes", run: func(c *ctx, args []string) error {
				fs := newFlagSet(name + " create")
				var dataFlag string
				fs.StringVar(&dataFlag, "data", "", "JSON body")
				if _, err := parseFlags(fs, args); err != nil {
					return err
				}
				if dataFlag == "" {
					return usageErr("用法: zyhive %s create --data JSON --yes", name)
				}
				if err := c.confirm("创建 %s", name); err != nil {
					return err
				}
				body, err := bodyFromData(dataFlag, nil)
				if err != nil {
					return err
				}
				resp, err := c.post(base, body)
				if err != nil {
					return err
				}
				return c.result(resp, nil)
			}},
			{name: "update", summary: "更新", usage: "zyhive " + name + " update <id> --data JSON --yes", run: func(c *ctx, args []string) error {
				fs := newFlagSet(name + " update")
				var dataFlag string
				fs.StringVar(&dataFlag, "data", "", "JSON patch/body")
				pos, err := parseFlags(fs, args)
				if err != nil {
					return err
				}
				id := arg(pos, 0)
				if id == "" || dataFlag == "" {
					return usageErr("用法: zyhive %s update <id> --data JSON --yes", name)
				}
				if err := c.confirm("更新 %s %s", name, id); err != nil {
					return err
				}
				body, err := bodyFromData(dataFlag, nil)
				if err != nil {
					return err
				}
				resp, err := c.request(updateMethod, base+"/"+id, body)
				if err != nil {
					return err
				}
				return c.result(resp, nil)
			}},
			{name: "delete", summary: "删除", usage: "zyhive " + name + " delete <id> --yes", run: func(c *ctx, args []string) error {
				id := arg(args, 0)
				if id == "" {
					return usageErr("用法: zyhive %s delete <id> --yes", name)
				}
				if err := c.confirm("删除 %s %s", name, id); err != nil {
					return err
				}
				resp, err := c.del(base + "/" + id)
				if err != nil {
					return err
				}
				return c.result(resp, nil)
			}},
			{name: "test", summary: "测试", usage: "zyhive " + name + " test <id>", run: func(c *ctx, args []string) error {
				id := arg(args, 0)
				if id == "" {
					return usageErr("用法: zyhive %s test <id>", name)
				}
				resp, err := c.post(base+"/"+id+"/test", nil)
				if err != nil {
					return err
				}
				return c.result(resp, nil)
			}},
		},
	})
}
