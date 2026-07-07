package orchestrator

import "sync"

const eventHistoryLimit = 256

// Hub retains a bounded per-session history and broadcasts live events.
// Replaying history prevents users from missing early sandbox events while
// navigating from the session list to the workspace.
type Hub struct {
	mu      sync.Mutex
	chans   map[string]map[chan EventEnvelope]struct{}
	history map[string][]EventEnvelope
}

func NewHub() *Hub {
	return &Hub{
		chans:   make(map[string]map[chan EventEnvelope]struct{}),
		history: make(map[string][]EventEnvelope),
	}
}

func (h *Hub) Subscribe(sessionID string) <-chan EventEnvelope {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan EventEnvelope, eventHistoryLimit+64)
	if h.chans[sessionID] == nil {
		h.chans[sessionID] = make(map[chan EventEnvelope]struct{})
	}
	h.chans[sessionID][ch] = struct{}{}
	for _, event := range h.history[sessionID] {
		ch <- event
	}
	return ch
}

func (h *Hub) Unsubscribe(sessionID string, ch <-chan EventEnvelope) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for subscriber := range h.chans[sessionID] {
		if subscriber == ch {
			delete(h.chans[sessionID], subscriber)
			close(subscriber)
			break
		}
	}
	if len(h.chans[sessionID]) == 0 {
		delete(h.chans, sessionID)
	}
}

// Broadcast is non-blocking for live subscribers. History remains available
// even when a browser has not connected yet.
func (h *Hub) Broadcast(sessionID string, event EventEnvelope) {
	h.mu.Lock()
	defer h.mu.Unlock()

	history := append(h.history[sessionID], event)
	if len(history) > eventHistoryLimit {
		history = append([]EventEnvelope(nil), history[len(history)-eventHistoryLimit:]...)
	}
	h.history[sessionID] = history

	for subscriber := range h.chans[sessionID] {
		select {
		case subscriber <- event:
		default:
			// A slow browser can reconnect and recover from retained history.
		}
	}
}
