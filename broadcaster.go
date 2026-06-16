package gohtmxelm

import "sync"

// Broadcaster is a thread-safe fan-out hub: every subscriber gets its own
// buffered channel, and Publish delivers a value to all of them. Slow
// subscribers whose buffer is full are skipped rather than blocking the
// publisher — they are expected to resync on their next SSE reconnect.
//
// It replaces the per-type pub/sub that an in-memory store and a timer would
// otherwise each reimplement: embed or hold a Broadcaster[T] and the
// Subscribe/Unsubscribe/Publish plumbing comes for free.
type Broadcaster[T any] struct {
	mu     sync.RWMutex
	buffer int
	subs   map[chan T]struct{}
}

// NewBroadcaster returns a ready Broadcaster whose subscriber channels are
// buffered by buffer (use a small value like 16). A non-positive buffer is
// treated as 1.
func NewBroadcaster[T any](buffer int) *Broadcaster[T] {
	if buffer < 1 {
		buffer = 1
	}
	return &Broadcaster[T]{buffer: buffer, subs: make(map[chan T]struct{})}
}

// Subscribe registers and returns a new buffered channel. Call Unsubscribe
// (typically via defer) when the subscriber is done.
func (b *Broadcaster[T]) Subscribe() chan T {
	ch := make(chan T, b.buffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes ch and closes it. Safe to call once per channel.
func (b *Broadcaster[T]) Unsubscribe(ch chan T) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

// Publish delivers v to every current subscriber, skipping any whose buffer is
// full. It never blocks.
func (b *Broadcaster[T]) Publish(v T) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- v:
		default:
		}
	}
}
