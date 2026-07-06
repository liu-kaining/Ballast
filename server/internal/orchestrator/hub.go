package orchestrator

import (
	"sync"
)

// Hub 按 sessionID 维护 WebSocket 订阅者，广播 EventEnvelope。
type Hub struct {
	mu     sync.RWMutex
	chans  map[string]map[chan EventEnvelope]struct{}
}

func NewHub() *Hub {
	return &Hub{chans: map[string]map[chan EventEnvelope]struct{}{}}
}

// Subscribe 订阅一个 session 的事件流，返回只读 channel。
func (h *Hub) Subscribe(sessionID string) <-chan EventEnvelope {
	ch := make(chan EventEnvelope, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.chans[sessionID] == nil {
		h.chans[sessionID] = map[chan EventEnvelope]struct{}{}
	}
	h.chans[sessionID][ch] = struct{}{}
	return ch
}

// Unsubscribe 取消订阅。
func (h *Hub) Unsubscribe(sessionID string, ch <-chan EventEnvelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs, ok := h.chans[sessionID]
	if !ok {
		return
	}
	for c := range subs {
		if c == ch {
			close(c)
			delete(subs, c)
		}
	}
}

// Broadcast 向某 session 的所有订阅者广播事件。非阻塞：慢消费者丢弃。
func (h *Hub) Broadcast(sessionID string, ev EventEnvelope) {
	h.mu.RLock()
	subs := h.chans[sessionID]
	h.mu.RUnlock()
	for c := range subs {
		select {
		case c <- ev:
		default:
			// drop on full
		}
	}
}
