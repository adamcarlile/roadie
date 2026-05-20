package server

import (
	"sync"

	"roadie/internal/syncer"
)

// hub fans sync events out to SSE clients and keeps the current run's event
// log so a late joiner can be brought up to date.
type hub struct {
	mu   sync.Mutex
	subs map[chan syncer.Event]struct{}
	// log accumulates the current run's events (reset on EvRunStart) for
	// snapshot replay. It grows for the run's duration; runs are short-lived
	// so this stays bounded in practice — summarising EvEntryProgress events
	// is possible future work for very large runs.
	log []syncer.Event
}

func newHub() *hub {
	return &hub{subs: map[chan syncer.Event]struct{}{}}
}

// subBuf is the per-subscriber channel buffer — enough to ride out a brief
// slow render before publish starts dropping events for that client.
const subBuf = 32

// subscribe registers a new client, returning its channel and a snapshot of
// the current run's events so the client can catch up before live updates.
func (h *hub) subscribe() (chan syncer.Event, []syncer.Event) {
	ch := make(chan syncer.Event, subBuf)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[ch] = struct{}{}
	snap := make([]syncer.Event, len(h.log))
	copy(snap, h.log)
	return ch, snap
}

// unsubscribe removes and closes a subscriber channel.
func (h *hub) unsubscribe(ch chan syncer.Event) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// publish records the event in the run log (resetting it on run-start) and
// fans it out, dropping it for any subscriber whose buffer is full.
func (h *hub) publish(e syncer.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e.Type == syncer.EvRunStart {
		h.log = nil
	}
	h.log = append(h.log, e)
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// snapshot returns a copy of the current run's event log.
func (h *hub) snapshot() []syncer.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]syncer.Event, len(h.log))
	copy(out, h.log)
	return out
}
