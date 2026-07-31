package browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/netguard"
)

type proxyDialFunc func(context.Context, string, string) (net.Conn, error)
type proxyValidateFunc func(context.Context, string) error

type safeProxy struct {
	listener  net.Listener
	server    *http.Server
	transport *http.Transport
	forward   *httputil.ReverseProxy
	dial      proxyDialFunc
	validate  proxyValidateFunc
	closeOnce sync.Once
}

func newSafeProxy() (*safeProxy, error) {
	transport := netguard.NewSafeTransport()
	return startSafeProxy(transport, netguard.DialContext, netguard.ValidateURL)
}

func startSafeProxy(
	transport *http.Transport,
	dial proxyDialFunc,
	validate proxyValidateFunc,
) (*safeProxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start browser safety proxy: %w", err)
	}
	proxy := &safeProxy{
		listener:  listener,
		transport: transport,
		dial:      dial,
		validate:  validate,
	}
	proxy.forward = &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			target := *request.In.URL
			if target.Scheme == "" {
				target.Scheme = "http"
			}
			if target.Host == "" {
				target.Host = request.In.Host
			}
			request.Out.URL = &target
			request.Out.Host = target.Host
			request.Out.Header.Del("Proxy-Authorization")
		},
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "browser request blocked", http.StatusBadGateway)
		},
	}
	proxy.server = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		_ = proxy.server.Serve(listener)
	}()
	return proxy, nil
}

func (p *safeProxy) URL() string {
	return "http://" + p.listener.Addr().String()
}

func (p *safeProxy) Close() {
	p.closeOnce.Do(func() {
		_ = p.server.Close()
		_ = p.listener.Close()
		p.transport.CloseIdleConnections()
	})
}

func (p *safeProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	target := absoluteProxyURL(r)
	if target == nil || (target.Scheme != "http" && target.Scheme != "ws") ||
		target.Hostname() == "" || target.User != nil {
		http.Error(w, "invalid browser proxy target", http.StatusForbidden)
		return
	}
	if target.Scheme == "ws" {
		target.Scheme = "http"
	}
	if err := p.validate(r.Context(), target.String()); err != nil {
		http.Error(w, "browser request blocked", http.StatusForbidden)
		return
	}
	r.URL = target
	p.forward.ServeHTTP(w, r)
}

func absoluteProxyURL(r *http.Request) *url.URL {
	if r.URL == nil {
		return nil
	}
	target := *r.URL
	if target.Scheme == "" {
		target.Scheme = "http"
	}
	if target.Host == "" {
		target.Host = r.Host
	}
	return &target
}

func (p *safeProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	address := r.Host
	if address == "" && r.URL != nil {
		address = r.URL.Host
	}
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		http.Error(w, "invalid CONNECT target", http.StatusBadRequest)
		return
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "invalid CONNECT port", http.StatusBadRequest)
		return
	}

	targetConn, err := p.dial(r.Context(), "tcp", address)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, netguard.ErrBlocked) {
			status = http.StatusForbidden
		}
		http.Error(w, "browser CONNECT blocked", status)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = targetConn.Close()
		http.Error(w, "CONNECT unsupported", http.StatusInternalServerError)
		return
	}
	clientConn, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = targetConn.Close()
		return
	}
	defer clientConn.Close()
	defer targetConn.Close()

	_, _ = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	if err := buffered.Flush(); err != nil {
		return
	}

	copyDone := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(targetConn, buffered.Reader)
		copyDone <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(clientConn, targetConn)
		copyDone <- struct{}{}
	}()
	<-copyDone
}
