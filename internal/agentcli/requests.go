package agentcli

import (
	"context"
	"fmt"
)

// Convenience request helpers on ctx so command handlers stay terse. Each uses
// a per-call timeout (except streams) and returns the response body or an error
// already carrying the right exit code.

func (c *ctx) get(path string) ([]byte, error) {
	cx, cancel := shortTimeout(context.Background())
	defer cancel()
	return c.client.Request(cx, "GET", path, nil)
}

func (c *ctx) post(path string, body any) ([]byte, error) {
	cx, cancel := shortTimeout(context.Background())
	defer cancel()
	return c.client.Request(cx, "POST", path, body)
}

func (c *ctx) patch(path string, body any) ([]byte, error) {
	cx, cancel := shortTimeout(context.Background())
	defer cancel()
	return c.client.Request(cx, "PATCH", path, body)
}

func (c *ctx) put(path string, body any) ([]byte, error) {
	cx, cancel := shortTimeout(context.Background())
	defer cancel()
	return c.client.Request(cx, "PUT", path, body)
}

func (c *ctx) del(path string) ([]byte, error) {
	cx, cancel := shortTimeout(context.Background())
	defer cancel()
	return c.client.Request(cx, "DELETE", path, nil)
}

// request issues an arbitrary method (used by the `api` escape hatch).
func (c *ctx) request(method, path string, body any) ([]byte, error) {
	cx, cancel := shortTimeout(context.Background())
	defer cancel()
	return c.client.Request(cx, method, path, body)
}

// stream POSTs and consumes an SSE stream (no timeout — long-running).
func (c *ctx) stream(path string, body any, onEvent func(SSEEvent) error) error {
	return c.client.StreamSSE(context.Background(), "POST", path, body, onEvent)
}

// confirm enforces the write-safety policy: returns nil if --yes was passed,
// otherwise refuses in non-interactive contexts. (We default to refusing rather
// than prompting, since the primary caller is an agent via exec.)
func (c *ctx) confirm(whatFormat string, a ...any) error {
	if c.yes {
		return nil
	}
	return usageErr("该操作有副作用（%s），请加 --yes 确认执行", fmt.Sprintf(whatFormat, a...))
}
