package store

import "sync"

// Event is emitted to subscribers whenever a key is written.
type Event struct {
	Key   string
	Value string
}

// Store is a thread-safe in-memory key/value store with pub/sub change notifications.
type Store struct {
	mu          sync.RWMutex
	data        map[string]string
	subscribers map[chan Event]struct{}
}

// New returns an initialised, empty Store.
func New() *Store {
	return &Store{
		data:        make(map[string]string),
		subscribers: make(map[chan Event]struct{}),
	}
}

// Get returns the current value and whether the key exists.
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

// All returns a snapshot of every key/value pair.
func (s *Store) All() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

// Set writes the value and notifies all current subscribers.
// Subscribers whose channels are full are skipped; they will catch up via
// the next SSE hydration on reconnect.
func (s *Store) Set(key, value string) {
	s.mu.Lock()
	s.data[key] = value
	subs := make([]chan Event, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	e := Event{Key: key, Value: value}
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe returns a buffered channel that receives every subsequent Set event.
// Call Unsubscribe when the subscriber is done.
func (s *Store) Subscribe() chan Event {
	ch := make(chan Event, 16)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

// Unsubscribe removes the channel and closes it.
func (s *Store) Unsubscribe(ch chan Event) {
	s.mu.Lock()
	delete(s.subscribers, ch)
	s.mu.Unlock()
	close(ch)
}
