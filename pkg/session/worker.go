// Package session provides a persistent session worker.
//
// SessionWorker is a background goroutine that processes chat messages for one
// session. It is completely decoupled from HTTP connections: the runner runs to
// completion even if every browser tab is closed. SSE handlers simply subscribe
// to the Broadcaster and unsubscribe when they disconnect.
//
// WorkerPool maintains one SessionWorker per session (lazy creation).
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// RunFnType is the signature for a runner factory function.
// Called by the worker goroutine with ctx=context.Background() (independent of HTTP).
// The function must publish all events (including "done"/"error") to bc.
type RunFnType = func(ctx context.Context, sessionID string, message string, bc *Broadcaster) error

// RunRequest is a single conversation turn to be processed by the worker.
type RunRequest struct {
	AgentID   string
	SessionID string
	Message   string

	// RunFn is called by the worker goroutine to execute the runner.
	// Using a factory keeps worker.go free of runner/llm import cycles.
	RunFn RunFnType
}

// SessionWorker processes run requests for a single session sequentially.
// It is safe to create and forget — it shuts itself down after IdleTimeout.
type SessionWorker struct {
	sessionID   string
	Broadcaster *Broadcaster

	inputChan chan RunRequest
	idleTimer *time.Timer
	stopOnce  sync.Once
	stopCh    chan struct{}
	busy      atomic.Bool

	pool *WorkerPool // back-reference for self-removal
}

const workerIdleTimeout = 30 * time.Minute

func newSessionWorker(sessionID string, pool *WorkerPool) *SessionWorker {
	w := &SessionWorker{
		sessionID:   sessionID,
		Broadcaster: NewBroadcaster(),
		inputChan:   make(chan RunRequest, 8),
		stopCh:      make(chan struct{}),
		pool:        pool,
	}
	w.idleTimer = time.AfterFunc(workerIdleTimeout, w.Stop)
	go w.loop()
	return w
}

// Enqueue adds a run request to the worker's queue (non-blocking).
// If the queue is full, the request is dropped and an error is returned.
func (w *SessionWorker) Enqueue(req RunRequest) error {
	select {
	case w.inputChan <- req:
		return nil
	default:
		return fmt.Errorf("session %s worker queue full", w.sessionID)
	}
}

// IsBusy returns true if the worker is currently processing a request.
func (w *SessionWorker) IsBusy() bool {
	return w.busy.Load()
}

// Stop shuts down the worker goroutine (idempotent).
func (w *SessionWorker) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		if w.pool != nil {
			w.pool.remove(w.sessionID)
		}
	})
}

func (w *SessionWorker) loop() {
	for {
		select {
		case <-w.stopCh:
			return
		case req, ok := <-w.inputChan:
			if !ok {
				return
			}
			// Reset idle timer each time a message arrives
			w.idleTimer.Reset(workerIdleTimeout)
			w.process(req)
		}
	}
}

func (w *SessionWorker) process(req RunRequest) {
	w.busy.Store(true)
	defer w.busy.Store(false)

	// Signal start of a new generation (clears replay buffer)
	w.Broadcaster.StartGen()

	// Use background context — runner is NOT tied to any HTTP request lifecycle.
	ctx := context.Background()

	if err := req.RunFn(ctx, req.SessionID, req.Message, w.Broadcaster); err != nil {
		log.Printf("[worker %s] run error: %v", w.sessionID, err)
		errData, _ := json.Marshal(map[string]any{"type": "error", "error": err.Error()})
		w.Broadcaster.Publish(BroadcastEvent{Type: "error", Data: errData})
	}
}

// ---------------------------------------------------------------------------
// WorkerPool — manages one SessionWorker per session
// ---------------------------------------------------------------------------

// WorkerPool manages a pool of SessionWorkers, one per session.
// Workers are created lazily and removed after idle timeout.
type WorkerPool struct {
	mu      sync.Mutex
	workers map[string]*SessionWorker
}

// NewWorkerPool creates an empty WorkerPool.
func NewWorkerPool() *WorkerPool {
	return &WorkerPool{
		workers: make(map[string]*SessionWorker),
	}
}

// GetOrCreate returns the worker for sessionID, creating one if necessary.
func (p *WorkerPool) GetOrCreate(sessionID string) *SessionWorker {
	p.mu.Lock()
	defer p.mu.Unlock()
	if w, ok := p.workers[sessionID]; ok {
		return w
	}
	w := newSessionWorker(sessionID, p)
	p.workers[sessionID] = w
	return w
}

// Get returns the worker for sessionID if it exists, otherwise nil.
func (p *WorkerPool) Get(sessionID string) *SessionWorker {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.workers[sessionID]
}

// remove is called by the worker itself when it stops.
func (p *WorkerPool) remove(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.workers, sessionID)
}

// StopAll stops all workers (used on server shutdown).
func (p *WorkerPool) StopAll() {
	p.mu.Lock()
	list := make([]*SessionWorker, 0, len(p.workers))
	for _, w := range p.workers {
		list = append(list, w)
	}
	p.mu.Unlock()
	for _, w := range list {
		w.Stop()
	}
}
