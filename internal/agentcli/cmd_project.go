package agentcli

func init() {
	registerCommand(&command{
		name:    "project",
		summary: "共享项目工作区：项目 CRUD、权限、文件",
		actions: []*action{
			{name: "list", summary: "列出项目", usage: "zyhive project list", run: runProjectList},
			{name: "get", summary: "查看项目", usage: "zyhive project get <projectId>", run: runProjectGet},
			{name: "create", summary: "创建项目", usage: "zyhive project create --id ID --name NAME [--description TEXT] [--tags a,b] --yes", run: runProjectCreate},
			{name: "update", summary: "更新项目", usage: "zyhive project update <projectId> [--name NAME] [--description TEXT] [--tags a,b] --yes", run: runProjectUpdate},
			{name: "delete", summary: "删除项目", usage: "zyhive project delete <projectId> --yes", run: runProjectDelete},
			{name: "permissions", summary: "设置编辑权限", usage: "zyhive project permissions <projectId> --editors a,b --yes", run: runProjectPermissions},
			{name: "files", summary: "列出/读取项目文件", usage: "zyhive project files <projectId> [path] [--tree]", run: runProjectFiles},
			{name: "write", summary: "写入项目文件", usage: "zyhive project write <projectId> <path> <content|-> --yes", run: runProjectWrite},
			{name: "delete-file", summary: "删除项目文件", usage: "zyhive project delete-file <projectId> <path> --yes", run: runProjectDeleteFile},
		},
	})
}

func runProjectList(c *ctx, _ []string) error {
	resp, err := c.get("/api/projects")
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runProjectGet(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive project get <projectId>")
	}
	resp, err := c.get("/api/projects/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runProjectCreate(c *ctx, args []string) error {
	fs := newFlagSet("project create")
	var id, name, desc, tags, dataFlag string
	fs.StringVar(&id, "id", "", "项目 ID")
	fs.StringVar(&name, "name", "", "名称")
	fs.StringVar(&desc, "description", "", "描述")
	fs.StringVar(&tags, "tags", "", "逗号分隔标签")
	fs.StringVar(&dataFlag, "data", "", "完整 JSON body")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := c.confirm("创建项目"); err != nil {
		return err
	}
	body := map[string]any{"id": id, "name": name, "description": desc, "tags": parseCSV(tags)}
	if dataFlag == "" && (id == "" || name == "") {
		return usageErr("project create 需要 --id 和 --name，或 --data JSON")
	}
	req, err := bodyFromData(dataFlag, body)
	if err != nil {
		return err
	}
	resp, err := c.post("/api/projects", req)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runProjectUpdate(c *ctx, args []string) error {
	fs := newFlagSet("project update")
	var name, desc, tags, dataFlag string
	fs.StringVar(&name, "name", "", "名称")
	fs.StringVar(&desc, "description", "", "描述")
	fs.StringVar(&tags, "tags", "", "逗号分隔标签")
	fs.StringVar(&dataFlag, "data", "", "完整 JSON body")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id := arg(pos, 0)
	if id == "" {
		return usageErr("用法: zyhive project update <projectId> [flags] --yes")
	}
	if err := c.confirm("更新项目 %s", id); err != nil {
		return err
	}
	body := map[string]any{"name": name, "description": desc}
	if tags != "" {
		body["tags"] = parseCSV(tags)
	}
	req, err := bodyFromData(dataFlag, body)
	if err != nil {
		return err
	}
	resp, err := c.patch("/api/projects/"+id, req)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runProjectDelete(c *ctx, args []string) error {
	id := arg(args, 0)
	if id == "" {
		return usageErr("用法: zyhive project delete <projectId> --yes")
	}
	if err := c.confirm("删除项目 %s", id); err != nil {
		return err
	}
	resp, err := c.del("/api/projects/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runProjectPermissions(c *ctx, args []string) error {
	fs := newFlagSet("project permissions")
	var editors string
	fs.StringVar(&editors, "editors", "", "逗号分隔编辑 agent IDs；留空=全员可写")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id := arg(pos, 0)
	if id == "" {
		return usageErr("用法: zyhive project permissions <projectId> --editors a,b --yes")
	}
	if err := c.confirm("设置项目 %s 权限", id); err != nil {
		return err
	}
	resp, err := c.put("/api/projects/"+id+"/permissions", map[string]any{"editors": parseCSV(editors)})
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runProjectFiles(c *ctx, args []string) error {
	fs := newFlagSet("project files")
	var tree bool
	fs.BoolVar(&tree, "tree", false, "递归树")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id, path := arg(pos, 0), arg(pos, 1)
	if id == "" {
		return usageErr("用法: zyhive project files <projectId> [path] [--tree]")
	}
	if path == "" {
		path = "."
	}
	resp, err := c.get(slashPath("api/projects", id, "files", path) + q(map[string]string{"tree": map[bool]string{true: "true", false: ""}[tree]}))
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

func runProjectWrite(c *ctx, args []string) error {
	id, path, content := arg(args, 0), arg(args, 1), arg(args, 2)
	if id == "" || path == "" || content == "" {
		return usageErr("用法: zyhive project write <projectId> <path> <content|-> --yes")
	}
	if err := c.confirm("写入项目 %s/%s", id, path); err != nil {
		return err
	}
	content, err := stdinIfDash(content)
	if err != nil {
		return err
	}
	resp, err := c.put(slashPath("api/projects", id, "files", path), map[string]any{"content": content})
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runProjectDeleteFile(c *ctx, args []string) error {
	id, path := arg(args, 0), arg(args, 1)
	if id == "" || path == "" {
		return usageErr("用法: zyhive project delete-file <projectId> <path> --yes")
	}
	if err := c.confirm("删除项目文件 %s/%s", id, path); err != nil {
		return err
	}
	resp, err := c.del(slashPath("api/projects", id, "files", path))
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
