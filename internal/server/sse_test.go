package server

import (
	"testing"
	"time"

	"roadie/internal/syncer"
)

func TestHubBroadcastsToSubscribers(t *testing.T) {
	h := newHub()
	ch := h.subscribe()
	defer h.unsubscribe(ch)

	h.publish(syncer.Event{Type: syncer.EvRunStart})

	select {
	case e := <-ch:
		if e.Type != syncer.EvRunStart {
			t.Fatalf("got event %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber received no event")
	}
}

func TestHubPublishWithNoSubscribers(t *testing.T) {
	newHub().publish(syncer.Event{Type: syncer.EvRunStart}) // must not panic
}

func TestHubSlowSubscriberDoesNotBlock(t *testing.T) {
	h := newHub()
	slow := h.subscribe()
	defer h.unsubscribe(slow)
	// Fill the slow subscriber's buffer so further sends to it would block.
	for i := 0; i < subBuf; i++ {
		h.publish(syncer.Event{Type: syncer.EvEntryProgress})
	}
	// A fresh subscriber with an empty buffer must still receive promptly.
	fast := h.subscribe()
	defer h.unsubscribe(fast)
	h.publish(syncer.Event{Type: syncer.EvRunDone})
	select {
	case e := <-fast:
		if e.Type != syncer.EvRunDone {
			t.Fatalf("got %+v, want run-done", e)
		}
	case <-time.After(time.Second):
		t.Fatal("publish blocked behind the slow, full subscriber")
	}
}

func TestHubDoubleUnsubscribeIsSilent(t *testing.T) {
	h := newHub()
	ch := h.subscribe()
	h.unsubscribe(ch)
	h.unsubscribe(ch) // must not panic
}
