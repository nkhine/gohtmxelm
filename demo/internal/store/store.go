package store

import (
	"sort"
	"sync"
)

// Event is emitted to subscribers whenever a key is written or deleted.
type Event struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Source  string `json:"source"`            // which surface wrote: htmx | datastar | elm-a | elm-b | go
	Deleted bool   `json:"deleted,omitempty"` // true when the key was removed
	Version uint64 `json:"version"`           // monotonically increases with every write to this key
	Seq     uint64 `json:"seq"`               // global write counter across all keys
}

// Entry is a render-ready snapshot of one key, including attribution.
type Entry struct {
	Key     string
	Value   string
	Source  string
	Version uint64
}

type entry struct {
	value   string
	version uint64
	source  string
}

// Store is a thread-safe in-memory key/value store with pub/sub change
// notifications, per-key versioning, write attribution, and a global
// sequence counter.
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

// Seq returns the global write counter (total writes and deletes so far).
func (s *Store) Seq() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seq
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

// Entries returns a key-sorted snapshot with attribution and versions,
// for rendering tables deterministically.
func (s *Store) Entries() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.data))
	for k, e := range s.data {
		out = append(out, Entry{Key: k, Value: e.value, Source: e.source, Version: e.version})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// AllStates returns a full snapshot including version and global seq for each
// key, used to hydrate SSE clients on connect.
func (s *Store) AllStates() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Event, 0, len(s.data))
	for k, e := range s.data {
		out = append(out, Event{Key: k, Value: e.value, Source: e.source, Version: e.version, Seq: e.version})
	}
	return out
}

// Set writes key=value unconditionally, attributed to "go".
func (s *Store) Set(key, value string) {
	s.SetIf(key, value, "go", 0) //nolint:errcheck
}

// SetIf writes key=value attributed to source, only when the caller's
// wantVersion matches the stored version. Pass wantVersion=0 to skip the check.
// Returns (newVersion, true) on success or (currentVersion, false) on conflict.
func (s *Store) SetIf(key, value, source string, wantVersion uint64) (uint64, bool) {
	s.mu.Lock()
	cur := s.data[key]
	if wantVersion != 0 && cur.version != wantVersion {
		s.mu.Unlock()
		return cur.version, false
	}
	s.seq++
	seq := s.seq
	s.data[key] = entry{value: value, version: seq, source: source}
	subs := s.subscriberList()
	s.mu.Unlock()

	s.notify(subs, Event{Key: key, Value: value, Source: source, Version: seq, Seq: seq})
	return seq, true
}

// Delete removes a key, attributed to source. Returns false when the key
// does not exist. Subscribers receive an Event with Deleted=true.
func (s *Store) Delete(key, source string) bool {
	s.mu.Lock()
	if _, ok := s.data[key]; !ok {
		s.mu.Unlock()
		return false
	}
	delete(s.data, key)
	s.seq++
	seq := s.seq
	subs := s.subscriberList()
	s.mu.Unlock()

	s.notify(subs, Event{Key: key, Source: source, Deleted: true, Version: seq, Seq: seq})
	return true
}

// subscriberList must be called with s.mu held.
func (s *Store) subscriberList() []chan Event {
	subs := make([]chan Event, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	return subs
}

func (s *Store) notify(subs []chan Event, e Event) {
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			// drop slow consumers; they resync on the next SSE reconnect
		}
	}
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
