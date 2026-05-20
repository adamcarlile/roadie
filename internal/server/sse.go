package server

import (
	"sync"

	"roadie/internal/syncer"
)

// hub fans sync events out to all subscribed SSE clients.
type hub struct {
	mu   sync.Mutex
	subs map[chan syncer.Event]struct{}
}

func newHub() *hub {
	return &hub{subs: map[chan syncer.Event]struct{}{}}
}

// subBuf is the per-subscriber channel buffer — enough to ride out a brief
// slow render before publish starts dropping events for that client.
const subBuf = 32

// subscribe returns a new channel that receives published events.
func (h *hub) subscribe() chan syncer.Event {
	ch := make(chan syncer.Event, subBuf)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
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

// publish sends an event to every subscriber, dropping it for any subscriber
// whose buffer is full rather than blocking the run.
func (h *hub) publish(e syncer.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
