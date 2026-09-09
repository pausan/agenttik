package runner

import "sync"

// subscriberBuffer is how many events a slow browser may fall behind before it
// is dropped. Dropping a subscriber never blocks the turn.
const subscriberBuffer = 256

// Hub fans session events out to live subscribers.
type Hub struct {
	mu      sync.Mutex
	nextID  int64
	byTopic map[string]map[int64]*subscriber
}

// subscriber is one channel, which may be registered under several topics. The
// once is what makes that safe: the channel is closed by whichever happens
// first, an unsubscribe or a Publish dropping it for falling behind.
type subscriber struct {
	ch     chan Event
	topics []string
	once   sync.Once
}

func (s *subscriber) close() { s.once.Do(func() { close(s.ch) }) }

func NewHub() *Hub {
	return &Hub{byTopic: make(map[string]map[int64]*subscriber)}
}

// Subscribe returns a channel of events for one topic and a function that
// unsubscribes and drains it.
func (h *Hub) Subscribe(topic string) (<-chan Event, func()) {
	return h.SubscribeMany([]string{topic})
}

// SubscribeMany registers one channel under several topics. A UI with a dozen
// conversations open needs a dozen topics but only one connection, and a
// browser will not hold a dozen open connections to the same origin.
//
// An event published to two of the topics is delivered twice, which is what
// happens to the end of a turn when both the session and its project are being
// watched. Subscribers treat it as a refresh, so a repeat costs nothing.
func (h *Hub) SubscribeMany(topics []string) (<-chan Event, func()) {
	sub := &subscriber{ch: make(chan Event, subscriberBuffer)}

	h.mu.Lock()
	h.nextID++
	id := h.nextID
	seen := make(map[string]bool, len(topics))
	for _, topic := range topics {
		if topic == "" || seen[topic] {
			continue
		}
		seen[topic] = true
		sub.topics = append(sub.topics, topic)
		subs, ok := h.byTopic[topic]
		if !ok {
			subs = make(map[int64]*subscriber)
			h.byTopic[topic] = subs
		}
		subs[id] = sub
	}
	h.mu.Unlock()

	return sub.ch, func() {
		h.mu.Lock()
		h.drop(id, sub)
		h.mu.Unlock()
		sub.close()
	}
}

// drop removes a subscriber from every topic it is registered under. The
// caller holds the lock.
func (h *Hub) drop(id int64, sub *subscriber) {
	for _, topic := range sub.topics {
		subs, ok := h.byTopic[topic]
		if !ok {
			continue
		}
		delete(subs, id)
		if len(subs) == 0 {
			delete(h.byTopic, topic)
		}
	}
}

// Publish delivers ev to every subscriber of the topic. Subscribers that are
// not keeping up are dropped rather than waited on.
func (h *Hub) Publish(topic string, ev Event) {
	h.mu.Lock()
	var slow []*subscriber
	for id, sub := range h.byTopic[topic] {
		select {
		case sub.ch <- ev:
		default:
			h.drop(id, sub)
			slow = append(slow, sub)
		}
	}
	h.mu.Unlock()
	for _, sub := range slow {
		sub.close()
	}
}
