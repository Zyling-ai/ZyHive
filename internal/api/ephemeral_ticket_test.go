package api

import (
	"testing"
	"time"
)

func TestEphemeralTicketIsSingleUse(t *testing.T) {
	store := newEphemeralTicketStore()
	ticket, ok := store.issue(time.Minute)
	if !ok || ticket == "" {
		t.Fatal("failed to issue ticket")
	}
	if !store.consume(ticket) {
		t.Fatal("valid ticket was rejected")
	}
	if store.consume(ticket) {
		t.Fatal("replayed ticket was accepted")
	}
}

func TestEphemeralTicketExpires(t *testing.T) {
	store := newEphemeralTicketStore()
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	ticket, ok := store.issue(time.Minute)
	if !ok {
		t.Fatal("failed to issue ticket")
	}
	now = now.Add(2 * time.Minute)
	if store.consume(ticket) {
		t.Fatal("expired ticket was accepted")
	}
}

func TestWrongEphemeralTicketDoesNotConsumeValidOne(t *testing.T) {
	store := newEphemeralTicketStore()
	ticket, ok := store.issue(time.Minute)
	if !ok {
		t.Fatal("failed to issue ticket")
	}
	if store.consume("wrong") {
		t.Fatal("wrong ticket was accepted")
	}
	if !store.consume(ticket) {
		t.Fatal("valid ticket should remain usable")
	}
}
