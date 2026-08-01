package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMutatesSharedConfig(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodGet, "/api/models", false},
		{http.MethodPost, "/api/models", true},
		{http.MethodPatch, "/api/config", true},
		{http.MethodPost, "/api/providers/p1/test", true},
		{http.MethodPost, "/api/chat/stream", false},
		{http.MethodPost, "/pub/chat/a/stream", false},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		if got := mutatesSharedConfig(request); got != test.want {
			t.Errorf("%s %s = %v, want %v", test.method, test.path, got, test.want)
		}
	}
}

func TestApprovalStreamDoesNotBlockConfigWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	guard := &configAccessGuard{}
	router := gin.New()
	router.Use(guard.middleware)

	streamStarted := make(chan struct{})
	releaseStream := make(chan struct{})
	router.GET("/api/approvals/stream", func(c *gin.Context) {
		close(streamStarted)
		<-releaseStream
		c.Status(http.StatusOK)
	})
	writeDone := make(chan struct{})
	router.PATCH("/api/config", func(c *gin.Context) {
		close(writeDone)
		c.Status(http.StatusOK)
	})

	streamFinished := make(chan struct{})
	go func() {
		defer close(streamFinished)
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/approvals/stream", nil))
	}()
	<-streamStarted
	go router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPatch, "/api/config", nil))

	select {
	case <-writeDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("long-lived approval stream blocked config write")
	}
	close(releaseStream)
	<-streamFinished
}
