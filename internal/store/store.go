package store

import "sync"

// Event is emitted to subscribers whenever a key is written.
type Event struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version uint64 `json:"version"` // monotonically increases with every write to this key
	Seq     uint64 `json:"seq"`     // global write counter across all keys
}

type entry struct {
	value   string
	version uint64
}

// Store is a thread-safe in-memory key/value store with pub/sub change
// notifications, per-key versioning, and a global sequence counter.
type Store struct {
	mu          sync.RWMutex
	data        map[string]entry
	seq         uint64
	subscribers map[chan Event]struct{}
}

// New returns an initialised, empty Store.
func New() *Store {
	return &Store{
		data:        make(map[string]entry),
		subscribers: make(map[chan Event]struct{}),
	}
}

// Get returns the current value and whether the key exists.
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	return e.value, ok
}

// Version returns the current version of a key (0 if absent).
func (s *Store) Version(key string) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key].version
}

// All returns a snapshot of every key/value pair (values only, for rendering).
func (s *Store) All() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.data))
	for k, e := range s.data {
		out[k] = e.value
	}
	return out
}

// AllStates returns a full snapshot including version and global seq for each
// key, used to hydrate SSE clients on connect.
func (s *Store) AllStates() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Event, 0, len(s.data))
	for k, e := range s.data {
		out = append(out, Event{Key: k, Value: e.value, Version: e.version, Seq: e.version})
	}
	return out
}

// Set writes key=value unconditionally.
func (s *Store) Set(key, value string) {
	s.SetIf(key, value, 0) //nolint:errcheck
}

// SetIf writes key=value only when the caller's wantVersion matches the stored
// version. Pass wantVersion=0 to skip the check.
// Returns (newVersion, true) on success or (currentVersion, false) on conflict.
func (s *Store) SetIf(key, value string, wantVersion uint64) (uint64, bool) {
	s.mu.Lock()
	cur := s.data[key]
	if wantVersion != 0 && cur.version != wantVersion {
		s.mu.Unlock()
		return cur.version, false
	}
	s.seq++
	seq := s.seq
	s.data[key] = entry{value: value, version: seq}
	subs := make([]chan Event, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	e := Event{Key: key, Value: value, Version: seq, Seq: seq}
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			// drop slow consumers; they resync on the next SSE reconnect
		}
	}
	return seq, true
}

// Subscribe returns a buffered channel that receives every subsequent write event.
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
