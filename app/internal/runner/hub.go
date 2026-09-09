package runner

import "sync"

// subscriberBuffer is how many events a slow browser may fall behind before it
// is dropped. Dropping a subscriber never blocks the turn.
const subscriberBuffer = 256

// Hub fans session events out to live subscribers.
type Hub struct {
	mu      sync.Mutex
	nextID  int64
	byTopic map[string]map[int64]chan Event
}

func NewHub() *Hub {
	return &Hub{byTopic: make(map[string]map[int64]chan Event)}
}

// Subscribe returns a channel of events for one session and a function that
// unsubscribes and drains it.
func (h *Hub) Subscribe(topic string) (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)

	h.mu.Lock()
	h.nextID++
	id := h.nextID
	subs, ok := h.byTopic[topic]
	if !ok {
		subs = make(map[int64]chan Event)
		h.byTopic[topic] = subs
	}
	subs[id] = ch
	h.mu.Unlock()

	return ch, func() { h.unsubscribe(topic, id) }
}

func (h *Hub) unsubscribe(topic string, id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs, ok := h.byTopic[topic]
	if !ok {
		return
	}
	ch, ok := subs[id]
	if !ok {
		return
	}
	delete(subs, id)
	if len(subs) == 0 {
		delete(h.byTopic, topic)
	}
	close(ch)
}

// Publish delivers ev to every subscriber of the topic. Subscribers that are
// not keeping up are dropped rather than waited on.
func (h *Hub) Publish(topic string, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.byTopic[topic]
	for id, ch := range subs {
		select {
		case ch <- ev:
		default:
			delete(subs, id)
			close(ch)
		}
	}
	if len(subs) == 0 {
		delete(h.byTopic, topic)
	}
}
