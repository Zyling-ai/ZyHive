package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
