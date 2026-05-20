package server

import (
	"testing"
	"time"

	"roadie/internal/syncer"
)

func TestHubBroadcastsToSubscribers(t *testing.T) {
	h := newHub()
	ch, _ := h.subscribe()
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
	slow, _ := h.subscribe()
	defer h.unsubscribe(slow)
	// Fill the slow subscriber's buffer so further sends to it would block.
	for i := 0; i < subBuf; i++ {
		h.publish(syncer.Event{Type: syncer.EvEntryProgress})
	}
	// A fresh subscriber with an empty buffer must still receive promptly.
	fast, _ := h.subscribe()
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
	ch, _ := h.subscribe()
	h.unsubscribe(ch)
	h.unsubscribe(ch) // must not panic
}

func TestHubReplaysSnapshotToLateSubscriber(t *testing.T) {
	h := newHub()
	h.publish(syncer.Event{Type: syncer.EvRunStart})
	h.publish(syncer.Event{Type: syncer.EvEntryStart, Path: "Up (2009).mkv"})
	_, snap := h.subscribe()
	if len(snap) != 2 || snap[1].Path != "Up (2009).mkv" {
		t.Fatalf("snapshot = %+v, want the 2 prior events", snap)
	}
}
