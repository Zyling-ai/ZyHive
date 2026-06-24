package agentcli

import "strings"

// The `api` command is an escape hatch: it forwards an arbitrary request to any
// REST endpoint, guaranteeing every server capability is reachable even before
// it gets a dedicated wrapper.
func init() {
	mk := func(method string) *action {
		return &action{
			name:    strings.ToLower(method),
			summary: method + " 任意 API 路径并打印响应",
			usage:   "zyhive api " + method + " /api/<path> [body|-]\n例: zyhive api " + method + " /api/agents",
			run: func(c *ctx, args []string) error {
				return runAPI(c, method, args)
			},
		}
	}
	registerCommand(&command{
		name:    "api",
		summary: "逃生舱：直接调用任意 REST 端点（GET/POST/PUT/PATCH/DELETE）",
		actions: []*action{mk("GET"), mk("POST"), mk("PUT"), mk("PATCH"), mk("DELETE")},
	})
}

func runAPI(c *ctx, method string, args []string) error {
	path := arg(args, 0)
	if path == "" {
		return usageErr("用法: zyhive api %s /api/<path> [body|-]", method)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	var body any
	if raw := arg(args, 1); raw != "" {
		s, err := stdinIfDash(raw)
		if err != nil {
			return err
		}
		body = []byte(s)
	}

	data, err := c.request(method, path, body)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		c.ok("%s %s 完成", method, path)
		return nil
	}
	return c.result(data, nil) // pretty-print JSON (or verbatim if not JSON)
}
