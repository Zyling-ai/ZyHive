package agentcli

func init() {
	registerCommand(&command{
		name:    "network",
		summary: "通讯录/群档案：本地与全局视图、更新、合并",
		actions: []*action{
			{name: "contacts", summary: "列出联系人", usage: "zyhive network contacts <agentId> | --global", run: runNetworkContacts},
			{name: "contact", summary: "查看联系人", usage: "zyhive network contact <agentId> <contactId>", run: runNetworkContactGet},
			{name: "contact-update", summary: "更新联系人", usage: "zyhive network contact-update <agentId> <contactId> --data JSON --yes", run: runNetworkContactUpdate},
			{name: "contact-delete", summary: "删除联系人", usage: "zyhive network contact-delete <agentId> <contactId> --yes", run: runNetworkContactDelete},
			{name: "contact-merge", summary: "合并联系人", usage: "zyhive network contact-merge <agentId> <primaryId> --alias ALIAS --yes", run: runNetworkContactMerge},
			{name: "chats", summary: "列出群档案", usage: "zyhive network chats <agentId> | --global", run: runNetworkChats},
			{name: "chat", summary: "查看群档案", usage: "zyhive network chat <agentId> <chatId>", run: runNetworkChatGet},
			{name: "chat-update", summary: "更新群档案", usage: "zyhive network chat-update <agentId> <chatId> --data JSON --yes", run: runNetworkChatUpdate},
			{name: "chat-delete", summary: "删除群档案", usage: "zyhive network chat-delete <agentId> <chatId> --yes", run: runNetworkChatDelete},
			{name: "refresh", summary: "重建通讯录索引", usage: "zyhive network refresh <agentId>", run: runNetworkRefresh},
		},
	})
}

func runNetworkContacts(c *ctx, args []string) error {
	fs := newFlagSet("network contacts")
	var global bool
	fs.BoolVar(&global, "global", false, "全局聚合")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	var resp []byte
	if global {
		resp, err = c.get("/api/network/contacts")
	} else {
		agentID := arg(pos, 0)
		if agentID == "" {
			return usageErr("用法: zyhive network contacts <agentId> 或 --global")
		}
		resp, err = c.get("/api/agents/" + agentID + "/network/contacts")
	}
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runNetworkContactGet(c *ctx, args []string) error {
	return networkGet(c, args, "contacts")
}

func runNetworkContactUpdate(c *ctx, args []string) error {
	return networkPatch(c, args, "contacts")
}

func runNetworkContactDelete(c *ctx, args []string) error {
	return networkDelete(c, args, "contacts")
}

func runNetworkContactMerge(c *ctx, args []string) error {
	fs := newFlagSet("network contact-merge")
	var alias string
	fs.StringVar(&alias, "alias", "", "要合入的 aliasId")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	agentID, cid := arg(pos, 0), arg(pos, 1)
	if agentID == "" || cid == "" || alias == "" {
		return usageErr("用法: zyhive network contact-merge <agentId> <primaryId> --alias ALIAS --yes")
	}
	if err := c.confirm("合并联系人 %s <- %s", cid, alias); err != nil {
		return err
	}
	resp, err := c.post("/api/agents/"+agentID+"/network/contacts/"+cid+"/merge", map[string]any{"aliasId": alias})
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runNetworkChats(c *ctx, args []string) error {
	fs := newFlagSet("network chats")
	var global bool
	fs.BoolVar(&global, "global", false, "全局聚合")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	var resp []byte
	if global {
		resp, err = c.get("/api/network/chats")
	} else {
		agentID := arg(pos, 0)
		if agentID == "" {
			return usageErr("用法: zyhive network chats <agentId> 或 --global")
		}
		resp, err = c.get("/api/agents/" + agentID + "/network/chats")
	}
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func runNetworkChatGet(c *ctx, args []string) error {
	return networkGet(c, args, "chats")
}

func runNetworkChatUpdate(c *ctx, args []string) error {
	return networkPatch(c, args, "chats")
}

func runNetworkChatDelete(c *ctx, args []string) error {
	return networkDelete(c, args, "chats")
}

func runNetworkRefresh(c *ctx, args []string) error {
	agentID := arg(args, 0)
	if agentID == "" {
		return usageErr("用法: zyhive network refresh <agentId>")
	}
	resp, err := c.post("/api/agents/"+agentID+"/network/refresh", nil)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func networkGet(c *ctx, args []string, kind string) error {
	agentID, id := arg(args, 0), arg(args, 1)
	if agentID == "" || id == "" {
		return usageErr("用法: zyhive network %s <agentId> <id>", kind[:len(kind)-1])
	}
	resp, err := c.get("/api/agents/" + agentID + "/network/" + kind + "/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func networkPatch(c *ctx, args []string, kind string) error {
	fs := newFlagSet("network patch")
	var dataFlag string
	fs.StringVar(&dataFlag, "data", "", "JSON patch")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	agentID, id := arg(pos, 0), arg(pos, 1)
	if agentID == "" || id == "" || dataFlag == "" {
		return usageErr("用法: zyhive network update <agentId> <id> --data JSON --yes")
	}
	if err := c.confirm("更新 network/%s/%s", kind, id); err != nil {
		return err
	}
	body, err := bodyFromData(dataFlag, nil)
	if err != nil {
		return err
	}
	resp, err := c.patch("/api/agents/"+agentID+"/network/"+kind+"/"+id, body)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}

func networkDelete(c *ctx, args []string, kind string) error {
	agentID, id := arg(args, 0), arg(args, 1)
	if agentID == "" || id == "" {
		return usageErr("用法: zyhive network delete <agentId> <id> --yes")
	}
	if err := c.confirm("删除 network/%s/%s", kind, id); err != nil {
		return err
	}
	resp, err := c.del("/api/agents/" + agentID + "/network/" + kind + "/" + id)
	if err != nil {
		return err
	}
	return c.result(resp, nil)
}
