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
	enqueueMu sync.Mutex
	stopped   bool
	busy      atomic.Bool
	reserved  atomic.Bool

	key  workerKey
	pool *WorkerPool // back-reference for self-removal
}

const workerIdleTimeout = 30 * time.Minute

func newSessionWorker(key workerKey, pool *WorkerPool) *SessionWorker {
	w := &SessionWorker{
		sessionID:   key.sessionID,
		Broadcaster: NewBroadcaster(),
		inputChan:   make(chan RunRequest, 1),
		stopCh:      make(chan struct{}),
		key:         key,
		pool:        pool,
	}
	w.idleTimer = time.AfterFunc(workerIdleTimeout, w.handleIdleTimeout)
	go w.loop()
	return w
}

// Enqueue reserves the worker for one run request (non-blocking).
// A second request is rejected until the current turn reaches a terminal state.
func (w *SessionWorker) Enqueue(req RunRequest) error {
	w.enqueueMu.Lock()
	defer w.enqueueMu.Unlock()
	if w.stopped {
		return fmt.Errorf("session %s worker stopped", w.sessionID)
	}
	if w.reserved.Swap(true) {
		return fmt.Errorf("session %s is busy", w.sessionID)
	}
	w.Broadcaster.StartGen()
	select {
	case w.inputChan <- req:
		return nil
	default:
		w.reserved.Store(false)
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
		w.enqueueMu.Lock()
		w.stopped = true
		if w.idleTimer != nil {
			w.idleTimer.Stop()
		}
		close(w.stopCh)
		w.enqueueMu.Unlock()
		if w.pool != nil {
			w.pool.remove(w.key)
		}
	})
}

func (w *SessionWorker) handleIdleTimeout() {
	if w.reserved.Load() {
		w.idleTimer.Reset(workerIdleTimeout)
		return
	}
	w.Stop()
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
	defer func() {
		w.busy.Store(false)
		w.reserved.Store(false)
		w.enqueueMu.Lock()
		if !w.stopped {
			w.idleTimer.Reset(workerIdleTimeout)
		}
		w.enqueueMu.Unlock()
	}()

	// Use background context — runner is NOT tied to any HTTP request lifecycle.
	ctx := context.Background()
	var runErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				runErr = fmt.Errorf("session worker panic: %v", recovered)
			}
		}()
		runErr = req.RunFn(ctx, req.SessionID, req.Message, w.Broadcaster)
	}()

	if w.Broadcaster.IsDone() {
		return
	}
	if runErr != nil {
		log.Printf("[worker %s] run error: %v", w.sessionID, runErr)
		errData, _ := json.Marshal(map[string]any{"type": "error", "error": runErr.Error()})
		w.Broadcaster.Publish(BroadcastEvent{Type: "error", Data: errData})
		return
	}
	doneData, _ := json.Marshal(map[string]any{"type": "done", "sessionId": req.SessionID})
	w.Broadcaster.Publish(BroadcastEvent{Type: "done", Data: doneData})
}

// ---------------------------------------------------------------------------
// WorkerPool — manages one SessionWorker per owner/session pair
// ---------------------------------------------------------------------------

// WorkerPool manages a pool of SessionWorkers, one per owner/session pair.
// Workers are created lazily and removed after idle timeout.
type WorkerPool struct {
	mu      sync.Mutex
	workers map[workerKey]*SessionWorker
}

type workerKey struct {
	ownerID   string
	sessionID string
}

// NewWorkerPool creates an empty WorkerPool.
func NewWorkerPool() *WorkerPool {
	return &WorkerPool{
		workers: make(map[workerKey]*SessionWorker),
	}
}

// GetOrCreate returns the worker for an owner/session pair, creating one if necessary.
func (p *WorkerPool) GetOrCreate(ownerID, sessionID string) *SessionWorker {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := workerKey{ownerID: ownerID, sessionID: sessionID}
	if w, ok := p.workers[key]; ok {
		return w
	}
	w := newSessionWorker(key, p)
	p.workers[key] = w
	return w
}

// Get returns the worker for an owner/session pair if it exists, otherwise nil.
func (p *WorkerPool) Get(ownerID, sessionID string) *SessionWorker {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.workers[workerKey{ownerID: ownerID, sessionID: sessionID}]
}

// GetUnique returns a worker only when sessionID identifies exactly one owner.
// It is intended for legacy callbacks that do not yet carry an owner ID and
// fails closed instead of broadcasting across colliding sessions.
func (p *WorkerPool) GetUnique(sessionID string) *SessionWorker {
	p.mu.Lock()
	defer p.mu.Unlock()
	var found *SessionWorker
	for key, worker := range p.workers {
		if key.sessionID != sessionID {
			continue
		}
		if found != nil {
			return nil
		}
		found = worker
	}
	return found
}

// remove is called by the worker itself when it stops.
func (p *WorkerPool) remove(key workerKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.workers, key)
}

// ActiveCount returns (totalWorkers, busyWorkers) for the pool.
//
// totalWorkers is the number of workers currently in the pool (each represents
// one active or recently-active session). busyWorkers is the subset currently
// processing a request. Used by /readyz / /api/status for observability.
func (p *WorkerPool) ActiveCount() (total int, busy int) {
	p.mu.Lock()
	workers := make([]*SessionWorker, 0, len(p.workers))
	for _, w := range p.workers {
		workers = append(workers, w)
	}
	p.mu.Unlock()
	total = len(workers)
	for _, w := range workers {
		if w.IsBusy() {
			busy++
		}
	}
	return total, busy
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
