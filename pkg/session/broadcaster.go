// Package session provides a fan-out event broadcaster with replay buffer.
//
// Broadcaster decouples the runner (which produces events) from HTTP SSE
// handlers (which consume them). The runner runs in a background goroutine
// and writes to the Broadcaster. Any number of SSE connections can subscribe;
// when a new subscriber joins mid-generation, it first receives all buffered
// events from the current generation, then live events.
//
// Lifecycle:
//
//	generation starts  → runner calls Publish() repeatedly
//	browser disconnects → subscriber goroutine exits; runner is unaffected
//	browser reconnects  → new Subscribe() call replays buffer + continues
//	generation done     → Publish("done", ...) → buffer kept until next StartGen()
package session

import (
	"sync"
)

// BroadcastEvent is a single event in a generation turn.
type BroadcastEvent struct {
	Type string // "text_delta" | "thinking_delta" | "tool_call" | "tool_result" | "error" | "done"
	Data []byte // JSON-encoded payload (same as the SSE data field)
}

// Broadcaster is a thread-safe fan-out broadcaster with a replay buffer.
// One Broadcaster exists per session; it is reused across multiple generation turns.
type Broadcaster struct {
	mu     sync.RWMutex
	subs   map[string]chan BroadcastEvent // subscriber id → channel
	buffer []BroadcastEvent              // events for the current generation
	genID  int                           // monotonically increasing per generation turn
	done   bool                          // true if current generation is finished
}

// NewBroadcaster creates an idle Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subs: make(map[string]chan BroadcastEvent),
	}
}

// StartGen marks the beginning of a new generation turn, clearing the old buffer.
func (b *Broadcaster) StartGen() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buffer = b.buffer[:0]
	b.genID++
	b.done = false
}

// Publish sends an event to all current subscribers and appends it to the buffer.
func (b *Broadcaster) Publish(ev BroadcastEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buffer = append(b.buffer, ev)
	if ev.Type == "done" || ev.Type == "error" {
		b.done = true
	}
	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default:
			// slow subscriber — drop rather than block the runner
		}
	}
}

// Subscribe registers a new subscriber. It immediately receives all buffered
// events from the current generation, then live events going forward.
// Call the returned unsubscribe func when done.
func (b *Broadcaster) Subscribe(id string) (<-chan BroadcastEvent, func()) {
	ch := make(chan BroadcastEvent, 256)

	b.mu.Lock()
	// Replay buffer first (under lock so no events are missed)
	snapshot := make([]BroadcastEvent, len(b.buffer))
	copy(snapshot, b.buffer)
	isDone := b.done
	b.subs[id] = ch
	b.mu.Unlock()

	// Send buffered events without holding the lock to avoid deadlock
	go func() {
		for _, ev := range snapshot {
			ch <- ev
		}
		// If generation already finished, signal and close
		if isDone {
			// The "done" event is already in snapshot; nothing extra needed.
			_ = isDone
		}
	}()

	unsub := func() {
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
		// Drain the channel so the goroutine above doesn't block
		for {
			select {
			case <-ch:
			default:
				return
			}
		}
	}
	return ch, unsub
}

// IsDone reports whether the current generation has finished.
func (b *Broadcaster) IsDone() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.done
}

// BufferLen returns the number of buffered events in the current generation.
func (b *Broadcaster) BufferLen() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.buffer)
}
