// Package stopwatch is the demo's server-authoritative timer domain: a single
// Go-owned stopwatch whose state every front-end surface (HTMX controls,
// Datastar readout, Elm analytics) observes over SSE. It holds no rendering or
// transport concerns.
package stopwatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nkhine/gohtmxelm"
)

// Snapshot is a render-ready view of the stopwatch at one instant.
type Snapshot struct {
	ElapsedMs int64 `json:"elapsedMs"`
	Running   bool  `json:"running"`
	Laps      []Lap `json:"laps"`
}

// Lap is one recorded split, numbered newest-first in a Snapshot.
type Lap struct {
	Number    int   `json:"number"`
	ElapsedMs int64 `json:"elapsedMs"`
}

// CanLap reports whether a Lap action is meaningful: the timer is running, or
// it is paused with time already on the clock.
func (s Snapshot) CanLap() bool {
	return s.Running || s.ElapsedMs > 0
}

// Event is what subscribers receive. StateChange distinguishes a discrete user
// action (start/stop/lap/reset) from a periodic tick, so consumers that only
// care about control state can ignore the 10/sec tick stream.
type Event struct {
	Snapshot    Snapshot
	StateChange bool
}

// Stopwatch is a single Go-owned timer with pub/sub change notifications.
type Stopwatch struct {
	mu        sync.RWMutex
	elapsed   time.Duration
	startedAt time.Time
	running   bool
	laps      []time.Duration
	events    *gohtmxelm.Broadcaster[Event]
	now       func() time.Time // injectable clock for deterministic tests
	wake      chan struct{}    // signals Run that the timer started ticking
}

// New returns an idle stopwatch.
func New() *Stopwatch {
	return &Stopwatch{
		events: gohtmxelm.NewBroadcaster[Event](16),
		now:    time.Now,
		wake:   make(chan struct{}, 1),
	}
}

// Events exposes the broadcaster so SSE handlers can stream with gohtmxelm.Serve.
func (s *Stopwatch) Events() *gohtmxelm.Broadcaster[Event] { return s.events }

// Run drives the periodic tick loop. The ticker only runs while the stopwatch
// is running: when paused it stops the ticker and blocks on wake, so an idle
// stopwatch does no work. Start pokes wake to resume ticking.
func (s *Stopwatch) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	ticker.Stop()
	defer ticker.Stop()

	running := false
	for {
		if running {
			select {
			case <-ticker.C:
				if _, ok := s.tick(); !ok {
					ticker.Stop()
					running = false
				}
			case <-s.wake:
				// already ticking; nothing to do
			case <-ctx.Done():
				return
			}
		} else {
			select {
			case <-s.wake:
				ticker.Reset(100 * time.Millisecond)
				running = true
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *Stopwatch) Start() Snapshot {
	s.mu.Lock()
	if !s.running {
		s.running = true
		s.startedAt = s.now()
	}
	snap := s.snapshotLocked()
	s.mu.Unlock()
	s.events.Publish(Event{Snapshot: snap, StateChange: true})
	// Wake the tick loop without blocking if a poke is already pending.
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return snap
}

func (s *Stopwatch) Stop() Snapshot {
	s.mu.Lock()
	if s.running {
		s.elapsed += s.now().Sub(s.startedAt)
		s.running = false
	}
	snap := s.snapshotLocked()
	s.mu.Unlock()
	s.events.Publish(Event{Snapshot: snap, StateChange: true})
	return snap
}

func (s *Stopwatch) Reset() Snapshot {
	s.mu.Lock()
	s.elapsed = 0
	s.startedAt = s.now()
	s.running = false
	s.laps = nil
	snap := s.snapshotLocked()
	s.mu.Unlock()
	s.events.Publish(Event{Snapshot: snap, StateChange: true})
	return snap
}

func (s *Stopwatch) Lap() Snapshot {
	s.mu.Lock()
	elapsed := s.elapsedLocked()
	if elapsed > 0 {
		s.laps = append([]time.Duration{elapsed}, s.laps...)
	}
	snap := s.snapshotLocked()
	s.mu.Unlock()
	s.events.Publish(Event{Snapshot: snap, StateChange: true})
	return snap
}

func (s *Stopwatch) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

// tick emits a periodic (non-state-change) update. It returns ok=false when
// the stopwatch is paused, which tells Run to stop the ticker.
func (s *Stopwatch) tick() (Snapshot, bool) {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return Snapshot{}, false
	}
	snap := s.snapshotLocked()
	s.mu.Unlock()
	s.events.Publish(Event{Snapshot: snap, StateChange: false})
	return snap, true
}

func (s *Stopwatch) snapshotLocked() Snapshot {
	elapsed := s.elapsedLocked()
	laps := make([]Lap, 0, len(s.laps))
	total := len(s.laps)
	for i, lap := range s.laps {
		laps = append(laps, Lap{Number: total - i, ElapsedMs: lap.Milliseconds()})
	}
	return Snapshot{ElapsedMs: elapsed.Milliseconds(), Running: s.running, Laps: laps}
}

func (s *Stopwatch) elapsedLocked() time.Duration {
	elapsed := s.elapsed
	if s.running {
		elapsed += s.now().Sub(s.startedAt)
	}
	return elapsed
}

// FormatElapsed formats milliseconds as HH:MM:SS:mmm.
func FormatElapsed(ms int64) string {
	hours := ms / 3600000
	minutes := (ms / 60000) % 60
	seconds := (ms / 1000) % 60
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d:%03d", hours, minutes, seconds, millis)
}

// Dials returns the fill angle (degrees, 0–360) for each of the four dial
// rings. Each ring shows how far the elapsed time has progressed through its
// own unit: the subsecond ring completes once per second, the second ring once
// per minute, the minute ring once per hour, and the hour ring once per
// 12-hour cycle. Using the full millisecond value makes each ring sweep
// smoothly rather than stepping.
func Dials(ms int64) (sub, sec, min, hour float64) {
	const deg = 360.0
	sub = float64(ms%1000) / 1000.0 * deg
	sec = float64(ms%60000) / 60000.0 * deg
	min = float64(ms%3600000) / 3600000.0 * deg
	hour = float64(ms%43200000) / 43200000.0 * deg
	return sub, sec, min, hour
}
