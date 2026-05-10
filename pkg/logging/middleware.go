package logging

import (
	"github.com/gin-gonic/gin"
)

// HeaderTraceID is the response/request header name for the trace identifier.
// Inbound: when present, used as-is (lets external systems thread their own
// trace ids through). Outbound: always set so the client knows which id to
// reference when reporting issues.
const HeaderTraceID = "X-Trace-Id"

// TraceMiddleware attaches a trace_id to every request's context (via the
// gin Request.Context()) and echoes it on the response.
//
// Use BEFORE any handler that wants to log with logging.FromContext(ctx).
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderTraceID)
		if id == "" {
			id = NewTraceID()
		}
		c.Header(HeaderTraceID, id)

		// Replace the gin context's underlying request with one carrying our
		// trace_id, so any handler that takes ctx via c.Request.Context() sees it.
		req := c.Request
		ctx := WithTraceID(req.Context(), id)
		c.Request = req.WithContext(ctx)

		// Mirror into gin's own value bag so handlers using c.Get("trace_id")
		// can read it without going through the request context.
		c.Set("trace_id", id)
		c.Next()
	}
}
