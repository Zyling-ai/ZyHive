package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultPublicRequestsPerMinute = 60
	defaultPublicRatePerMinute     = 12
	defaultPublicConcurrent        = 4
	defaultPublicSSEConnections    = 32
	defaultPublicSessions          = 8
	defaultPublicActiveSessions    = 100
	defaultPublicRunTimeout        = 2 * time.Minute
	defaultPublicSessionTTL        = 24 * time.Hour
	defaultPublicActiveSessionTTL  = 30 * time.Minute
	defaultPublicMessageBytes      = 16 * 1024
)

type publicRateWindow struct {
	start time.Time
	count int
}

type publicAccessLimiter struct {
	mu                sync.Mutex
	requestRates      map[string]publicRateWindow
	rates             map[string]publicRateWindow
	sessions          map[string]map[string]time.Time
	globalSessions    map[string]time.Time
	slots             chan struct{}
	sseSlots          chan struct{}
	requestsPerMinute int
	ratePerMinute     int
	sessionsPerSource int
	maxActiveSessions int
	runTimeout        time.Duration
	sessionTTL        time.Duration
	activeSessionTTL  time.Duration
	maxMessageBytes   int
	trustProxyHeader  bool
	lastCleanup       time.Time
}

func newPublicAccessLimiterFromEnv() *publicAccessLimiter {
	return &publicAccessLimiter{
		requestRates:      make(map[string]publicRateWindow),
		rates:             make(map[string]publicRateWindow),
		sessions:          make(map[string]map[string]time.Time),
		globalSessions:    make(map[string]time.Time),
		slots:             make(chan struct{}, envPositiveInt("ZYHIVE_PUBLIC_MAX_CONCURRENT", defaultPublicConcurrent)),
		sseSlots:          make(chan struct{}, envPositiveInt("ZYHIVE_PUBLIC_MAX_SSE", defaultPublicSSEConnections)),
		requestsPerMinute: envPositiveInt("ZYHIVE_PUBLIC_REQUESTS_PER_MINUTE", defaultPublicRequestsPerMinute),
		ratePerMinute:     envPositiveInt("ZYHIVE_PUBLIC_RATE_PER_MINUTE", defaultPublicRatePerMinute),
		sessionsPerSource: envPositiveInt("ZYHIVE_PUBLIC_MAX_SESSIONS", defaultPublicSessions),
		maxActiveSessions: envPositiveInt("ZYHIVE_PUBLIC_MAX_ACTIVE_SESSIONS", defaultPublicActiveSessions),
		runTimeout:        envPositiveDuration("ZYHIVE_PUBLIC_RUN_TIMEOUT", defaultPublicRunTimeout),
		sessionTTL:        envPositiveDuration("ZYHIVE_PUBLIC_SESSION_TTL", defaultPublicSessionTTL),
		activeSessionTTL:  envPositiveDuration("ZYHIVE_PUBLIC_ACTIVE_SESSION_TTL", defaultPublicActiveSessionTTL),
		maxMessageBytes:   envPositiveInt("ZYHIVE_PUBLIC_MAX_MESSAGE_BYTES", defaultPublicMessageBytes),
		trustProxyHeader:  os.Getenv("ZYHIVE_TRUST_PROXY_HEADERS") == "1",
	}
}

func (l *publicAccessLimiter) limitRequest(c *gin.Context) {
	if err := l.allowRequest(l.sourceKey(c), time.Now()); err != nil {
		c.Header("Retry-After", "60")
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		return
	}
	c.Next()
}

func (l *publicAccessLimiter) allowRequest(source string, now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) >= time.Minute {
		l.cleanupLocked(now)
		l.lastCleanup = now
	}
	window := l.requestRates[source]
	if window.start.IsZero() || now.Sub(window.start) >= time.Minute {
		window = publicRateWindow{start: now}
	}
	if window.count >= l.requestsPerMinute {
		return fmt.Errorf("public endpoint rate limit exceeded")
	}
	window.count++
	l.requestRates[source] = window
	return nil
}

func envPositiveInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envPositiveDuration(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func (l *publicAccessLimiter) sourceKey(c *gin.Context) string {
	if l.trustProxyHeader {
		for _, header := range []string{"CF-Connecting-IP", "X-Real-IP"} {
			if ip := net.ParseIP(strings.TrimSpace(c.GetHeader(header))); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if ip := net.ParseIP(c.Request.RemoteAddr); ip != nil {
		return ip.String()
	}
	return "unknown"
}

func (l *publicAccessLimiter) admit(source, sessionID string, now time.Time) (func(), int, error) {
	l.mu.Lock()
	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) >= time.Minute {
		l.cleanupLocked(now)
		l.lastCleanup = now
	}
	window := l.rates[source]
	if window.start.IsZero() || now.Sub(window.start) >= time.Minute {
		window = publicRateWindow{start: now}
	}
	if window.count >= l.ratePerMinute {
		l.mu.Unlock()
		return nil, http.StatusTooManyRequests, fmt.Errorf("public chat rate limit exceeded")
	}
	window.count++
	l.rates[source] = window

	sourceSessions := l.sessions[source]
	if sourceSessions == nil {
		sourceSessions = make(map[string]time.Time)
		l.sessions[source] = sourceSessions
	}
	for id, seenAt := range sourceSessions {
		if now.Sub(seenAt) >= l.sessionTTL {
			delete(sourceSessions, id)
		}
	}
	if _, exists := sourceSessions[sessionID]; !exists && len(sourceSessions) >= l.sessionsPerSource {
		l.mu.Unlock()
		return nil, http.StatusTooManyRequests, fmt.Errorf("public chat session limit exceeded")
	}
	if _, exists := l.globalSessions[sessionID]; !exists && len(l.globalSessions) >= l.maxActiveSessions {
		l.mu.Unlock()
		return nil, http.StatusServiceUnavailable, fmt.Errorf("public chat active session capacity exceeded")
	}

	select {
	case l.slots <- struct{}{}:
		sourceSessions[sessionID] = now
		l.globalSessions[sessionID] = now
		l.mu.Unlock()
		var once sync.Once
		return func() {
			once.Do(func() { <-l.slots })
		}, 0, nil
	default:
		l.mu.Unlock()
		return nil, http.StatusServiceUnavailable, fmt.Errorf("public chat is at capacity")
	}
}

func (l *publicAccessLimiter) cleanupLocked(now time.Time) {
	for source, window := range l.requestRates {
		if now.Sub(window.start) >= 2*time.Minute {
			delete(l.requestRates, source)
		}
	}
	for source, window := range l.rates {
		if now.Sub(window.start) >= 2*time.Minute {
			delete(l.rates, source)
		}
	}
	for source, sourceSessions := range l.sessions {
		for id, seenAt := range sourceSessions {
			if now.Sub(seenAt) >= l.sessionTTL {
				delete(sourceSessions, id)
			}
		}
		if len(sourceSessions) == 0 {
			delete(l.sessions, source)
		}
	}
	for sessionID, seenAt := range l.globalSessions {
		if now.Sub(seenAt) >= l.activeSessionTTL {
			delete(l.globalSessions, sessionID)
		}
	}
}

func (l *publicAccessLimiter) acquireSSE() (func(), error) {
	select {
	case l.sseSlots <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-l.sseSlots })
		}, nil
	default:
		return nil, fmt.Errorf("public stream is at capacity")
	}
}
