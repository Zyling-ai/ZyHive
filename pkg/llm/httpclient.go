// pkg/llm/httpclient.go — 带超时、重试、错误分类的 HTTP client 工厂。
//
// 设计要点：
//  - 非流式请求：通过标准 http.Client.Timeout 限制整体耗时
//  - 流式请求：不设 Client.Timeout（会截断长响应），改用 context 传入超时
//  - 指数退避重试：最多 3 次，基础间隔 1s，上限 32s，加 jitter
//  - 错误分类：网络错误可重试；429 解析 Retry-After；401/403 不重试；5xx 有限重试
//  - keepalive 心跳：流式 SSE 读取超过 30s 无数据则认为连接断开
package llm

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	// dialTimeout 是 TCP 连接建立超时。
	dialTimeout = 10 * time.Second
	// responseHeaderTimeout 是等待响应头的最长时间。
	responseHeaderTimeout = 30 * time.Second
	// streamKeepaliveTimeout 是流式请求读取中超过此时间无数据则认为断开。
	streamKeepaliveTimeout = 30 * time.Second

	retryMaxAttempts = 3
	retryBaseDelay   = 1 * time.Second
	retryMaxDelay    = 32 * time.Second
)

// newHTTPClientWithTimeout 返回用于非流式请求的 HTTP client。
// 设置了 Timeout 以防止请求卡死，但不适合流式响应。
func newHTTPClientWithTimeout() *http.Client {
	return &http.Client{
		Transport: newTransport(),
		Timeout:   60 * time.Second,
	}
}

// newStreamingHTTPClient 返回用于流式 SSE 请求的 HTTP client。
// 不设置 Client.Timeout（否则会在流读取中途截断响应）。
// 流的生命周期通过传入的 context 控制。
func newStreamingHTTPClient() *http.Client {
	return &http.Client{
		Transport: newTransport(),
		// 故意不设 Timeout，让 context 负责控制流的生命周期
	}
}

// newTransport 返回配置了 dialTimeout 和 responseHeaderTimeout 的 Transport。
func newTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: responseHeaderTimeout,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// retryableError 分类请求错误，返回是否可重试及建议等待时长。
// 返回 (shouldRetry bool, waitDuration time.Duration).
func retryableError(resp *http.Response, err error) (bool, time.Duration) {
	// 网络层错误（DNS 失败、连接被拒、超时等）均可重试
	if err != nil {
		if isNetworkError(err) {
			return true, 0
		}
		// context 被取消不重试
		return false, 0
	}
	if resp == nil {
		return false, 0
	}
	switch resp.StatusCode {
	case 429:
		// 解析 Retry-After header
		wait := parseRetryAfter(resp.Header.Get("Retry-After"))
		return true, wait
	case 401, 403:
		// 认证/授权错误不重试
		return false, 0
	case 500, 502, 503, 504:
		return true, 0
	default:
		return false, 0
	}
}

// isNetworkError 判断是否为网络层错误（可重试）。
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if ok := isErrorType(err, &netErr); ok {
		return true
	}
	// io.EOF / io.ErrUnexpectedEOF 在连接断开时出现，可重试
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return true
	}
	return false
}

// isErrorType 是简单的 errors.As 包装，避免 import。
func isErrorType(err error, target interface{}) bool {
	if err == nil {
		return false
	}
	switch t := target.(type) {
	case *net.Error:
		if v, ok := err.(net.Error); ok {
			*t = v
			return true
		}
	}
	return false
}

// parseRetryAfter 解析 Retry-After header（秒数或 HTTP 日期格式）。
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	// 尝试整数秒
	if secs, err := strconv.Atoi(header); err == nil {
		return time.Duration(secs) * time.Second
	}
	// 尝试 HTTP 日期
	if t, err := http.ParseTime(header); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

// retryDelay 计算第 attempt 次重试的等待时间（指数退避 + jitter）。
// attempt 从 0 开始（第 0 次失败后等待）。
func retryDelay(attempt int, serverHint time.Duration) time.Duration {
	if serverHint > 0 {
		return serverHint
	}
	// 指数退避：base * 2^attempt
	delay := retryBaseDelay * (1 << uint(attempt))
	if delay > retryMaxDelay {
		delay = retryMaxDelay
	}
	// 加 ±25% jitter：最终范围 [base*0.75, base*1.25)
	base := int64(delay)
	jitterRange := base / 4
	if jitterRange < 1 {
		jitterRange = 1
	}
	return time.Duration(base - jitterRange + rand.Int63n(2*jitterRange))
}

// doWithRetry 执行 HTTP 请求，失败时按策略重试。
// makeReq 每次调用返回一个新的 *http.Request（body 已重置）。
// 调用方负责在成功时 close resp.Body。
func doWithRetry(ctx context.Context, client *http.Client, makeReq func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < retryMaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		req, err := makeReq()
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)
		if err != nil {
			shouldRetry, hint := retryableError(nil, err)
			if !shouldRetry {
				return nil, err
			}
			lastErr = err
			wait := retryDelay(attempt, hint)
			log.Printf("[llm] request failed (attempt %d/%d): %v — retrying in %v",
				attempt+1, retryMaxAttempts, err, wait)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}

		shouldRetry, hint := retryableError(resp, nil)
		if !shouldRetry {
			return resp, nil
		}
		// 可重试的 HTTP 状态码
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		lastErr = fmt.Errorf("http status %d: %s", resp.StatusCode, string(errBody))
		wait := retryDelay(attempt, hint)
		log.Printf("[llm] API error (attempt %d/%d): %s — retrying in %v",
			attempt+1, retryMaxAttempts, lastErr, wait)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("all %d attempts failed: %w", retryMaxAttempts, lastErr)
}

// keepaliveReader 包装 io.Reader，超过 idleTimeout 无数据则取消。
// 用于流式 SSE 响应，防止连接假死。
type keepaliveReader struct {
	r           io.Reader
	cancel      context.CancelFunc
	idleTimeout time.Duration
	timer       *time.Timer
}

// newKeepaliveReader 返回一个 keepaliveReader，当超过 idleTimeout 无数据时调用 cancel。
func newKeepaliveReader(r io.Reader, idleTimeout time.Duration, cancel context.CancelFunc) *keepaliveReader {
	kr := &keepaliveReader{
		r:           r,
		cancel:      cancel,
		idleTimeout: idleTimeout,
	}
	kr.timer = time.AfterFunc(idleTimeout, cancel)
	return kr
}

func (kr *keepaliveReader) Read(p []byte) (n int, err error) {
	n, err = kr.r.Read(p)
	if n > 0 {
		// 有数据 → 重置计时器
		kr.timer.Reset(kr.idleTimeout)
	}
	return
}

// Stop 停止心跳计时器（在流结束时调用）。
func (kr *keepaliveReader) Stop() {
	kr.timer.Stop()
}
