// Package channel — Telegram message debouncer.
// Buffers rapid consecutive messages from the same chatID and merges them
// into a single handler call after a configurable quiet period (default 300ms).
package channel

import (
	"strings"
	"sync"
	"time"
)

// debouncer collects messages per chatID and fires handler once the quiet period expires.
// If a new message arrives before the timer fires, the timer is reset.
type debouncer struct {
	mu      sync.Mutex
	timers  map[int64]*time.Timer
	buffer  map[int64][]string
	delay   time.Duration
	handler func(chatID int64, msgs []string)
}

// newDebouncer creates a debouncer with the specified delay and message handler.
func newDebouncer(delay time.Duration, handler func(chatID int64, msgs []string)) *debouncer {
	return &debouncer{
		timers:  make(map[int64]*time.Timer),
		buffer:  make(map[int64][]string),
		delay:   delay,
		handler: handler,
	}
}

// Add queues a message for chatID.  If a timer is already pending for that chat,
// it is reset; otherwise a new timer is created.
func (d *debouncer) Add(chatID int64, text string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.buffer[chatID] = append(d.buffer[chatID], text)

	if t, ok := d.timers[chatID]; ok {
		// Stop before Reset to prevent a fired timer from also running.
		// The AfterFunc goroutine may already be waiting for the lock; the
		// len(msgs)==0 guard in the callback handles that race safely.
		t.Stop()
		t.Reset(d.delay)
		return
	}

	d.timers[chatID] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		msgs := d.buffer[chatID]
		delete(d.buffer, chatID)
		delete(d.timers, chatID)
		d.mu.Unlock()

		if len(msgs) == 0 {
			return
		}
		d.handler(chatID, msgs)
	})
}

// mergeMessages joins multiple buffered messages with a newline separator.
func mergeMessages(msgs []string) string {
	return strings.Join(msgs, "\n")
}
