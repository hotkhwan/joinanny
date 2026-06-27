package realtime

import "sync"

// Broadcaster fans one event out to every current subscriber. Publish never
// blocks: a subscriber whose buffer is full drops the event rather than stalling
// the gateway, so one slow web client can never back up the whole stream.
type Broadcaster struct {
	mu     sync.Mutex
	next   int
	subs   map[int]chan Event
	buffer int
}

// NewBroadcaster returns a broadcaster whose per-subscriber buffer holds bufSize
// events before further events to that subscriber are dropped.
func NewBroadcaster(bufSize int) *Broadcaster {
	if bufSize <= 0 {
		bufSize = 16
	}
	return &Broadcaster{subs: make(map[int]chan Event), buffer: bufSize}
}

// Subscribe registers a new subscriber and returns its event channel plus an
// unsubscribe func. The channel is closed when unsubscribe is called, so a range
// over it terminates cleanly. Unsubscribe is idempotent.
func (b *Broadcaster) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.next
	b.next++
	ch := make(chan Event, b.buffer)
	b.subs[id] = ch

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if existing, ok := b.subs[id]; ok {
				delete(b.subs, id)
				close(existing)
			}
		})
	}
	return ch, cancel
}

// Publish delivers event to every subscriber with room in its buffer.
func (b *Broadcaster) Publish(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.subs {
		select {
		case ch <- event:
		default: // drop for this slow subscriber; never block the gateway
		}
	}
}

// SubscriberCount reports how many subscribers are currently attached.
func (b *Broadcaster) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
