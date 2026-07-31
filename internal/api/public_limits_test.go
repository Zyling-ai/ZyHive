package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newTestPublicLimiter(rate, concurrent, sessions int) *publicAccessLimiter {
	return &publicAccessLimiter{
		requestRates:      make(map[string]publicRateWindow),
		rates:             make(map[string]publicRateWindow),
		sessions:          make(map[string]map[string]time.Time),
		globalSessions:    make(map[string]time.Time),
		slots:             make(chan struct{}, concurrent),
		sseSlots:          make(chan struct{}, concurrent),
		requestsPerMinute: rate,
		ratePerMinute:     rate,
		sessionsPerSource: sessions,
		maxActiveSessions: 100,
		runTimeout:        time.Second,
		sessionTTL:        time.Hour,
		activeSessionTTL:  time.Hour,
		maxMessageBytes:   1024,
	}
}

func TestPublicEndpointRateLimit(t *testing.T) {
	limiter := newTestPublicLimiter(2, 1, 1)
	now := time.Now()
	if err := limiter.allowRequest("source", now); err != nil {
		t.Fatal(err)
	}
	if err := limiter.allowRequest("source", now); err != nil {
		t.Fatal(err)
	}
	if err := limiter.allowRequest("source", now); err == nil {
		t.Fatal("expected public endpoint rate limit")
	}
}

func TestPublicLimiterRateLimit(t *testing.T) {
	limiter := newTestPublicLimiter(2, 2, 2)
	now := time.Now()
	for range 2 {
		release, status, err := limiter.admit("source", "session", now)
		if err != nil || status != 0 {
			t.Fatalf("unexpected admission failure: status=%d err=%v", status, err)
		}
		release()
	}
	if _, status, err := limiter.admit("source", "session", now); err == nil || status != http.StatusTooManyRequests {
		t.Fatalf("expected rate limit, status=%d err=%v", status, err)
	}
}

func TestPublicLimiterGlobalConcurrency(t *testing.T) {
	limiter := newTestPublicLimiter(10, 1, 10)
	release, _, err := limiter.admit("source-a", "session-a", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	if _, status, err := limiter.admit("source-b", "session-b", time.Now()); err == nil || status != http.StatusServiceUnavailable {
		t.Fatalf("expected capacity rejection, status=%d err=%v", status, err)
	}
}

func TestPublicLimiterSSEConcurrency(t *testing.T) {
	limiter := newTestPublicLimiter(10, 1, 10)
	release, err := limiter.acquireSSE()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := limiter.acquireSSE(); err == nil {
		t.Fatal("expected SSE capacity rejection")
	}
}

func TestPublicLimiterSessionCapAndExpiry(t *testing.T) {
	limiter := newTestPublicLimiter(10, 1, 1)
	now := time.Now()
	release, _, err := limiter.admit("source", "session-a", now)
	if err != nil {
		t.Fatal(err)
	}
	release()

	if _, status, err := limiter.admit("source", "session-b", now); err == nil || status != http.StatusTooManyRequests {
		t.Fatalf("expected session cap, status=%d err=%v", status, err)
	}

	limiter.sessionTTL = time.Minute
	release, _, err = limiter.admit("source", "session-b", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("expired session should be pruned: %v", err)
	}
	release()
}

func TestPublicLimiterGlobalActiveSessionCapAndExpiry(t *testing.T) {
	limiter := newTestPublicLimiter(10, 1, 10)
	limiter.maxActiveSessions = 1
	now := time.Now()
	release, _, err := limiter.admit("source-a", "session-a", now)
	if err != nil {
		t.Fatal(err)
	}
	release()

	if _, status, err := limiter.admit("source-b", "session-b", now); err == nil || status != http.StatusServiceUnavailable {
		t.Fatalf("expected global session capacity, status=%d err=%v", status, err)
	}

	limiter.activeSessionTTL = time.Minute
	release, _, err = limiter.admit("source-b", "session-b", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("expired global session should be pruned: %v", err)
	}
	release()
}

func TestPublicLimiterSourceKeyDoesNotTrustHeadersByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "203.0.113.10:4321"
	req.Header.Set("X-Real-IP", "198.51.100.20")
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = req

	limiter := newTestPublicLimiter(1, 1, 1)
	if got := limiter.sourceKey(ctx); got != "203.0.113.10" {
		t.Fatalf("sourceKey=%q, want remote address", got)
	}

	limiter.trustProxyHeader = true
	if got := limiter.sourceKey(ctx); got != "198.51.100.20" {
		t.Fatalf("trusted sourceKey=%q, want proxy header", got)
	}
}

func TestPublicLimiterEnvironmentDefaultsAndOverrides(t *testing.T) {
	t.Setenv("ZYHIVE_PUBLIC_MAX_CONCURRENT", "2")
	t.Setenv("ZYHIVE_PUBLIC_MAX_SSE", "4")
	t.Setenv("ZYHIVE_PUBLIC_REQUESTS_PER_MINUTE", "30")
	t.Setenv("ZYHIVE_PUBLIC_RATE_PER_MINUTE", "7")
	t.Setenv("ZYHIVE_PUBLIC_MAX_SESSIONS", "3")
	t.Setenv("ZYHIVE_PUBLIC_MAX_ACTIVE_SESSIONS", "9")
	t.Setenv("ZYHIVE_PUBLIC_RUN_TIMEOUT", "5s")
	t.Setenv("ZYHIVE_PUBLIC_ACTIVE_SESSION_TTL", "10m")
	t.Setenv("ZYHIVE_PUBLIC_MAX_MESSAGE_BYTES", "2048")

	limiter := newPublicAccessLimiterFromEnv()
	if cap(limiter.slots) != 2 || cap(limiter.sseSlots) != 4 ||
		limiter.requestsPerMinute != 30 || limiter.ratePerMinute != 7 ||
		limiter.sessionsPerSource != 3 || limiter.maxActiveSessions != 9 {
		t.Fatalf("unexpected limits: %+v", limiter)
	}
	if limiter.runTimeout != 5*time.Second || limiter.activeSessionTTL != 10*time.Minute ||
		limiter.maxMessageBytes != 2048 {
		t.Fatalf("unexpected timeout/message limits: %+v", limiter)
	}
}
