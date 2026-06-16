package presence

import (
	"sync"
	"time"

	"github.com/nkhine/gohtmxelm"
)

// State is the demo-wide authentication presence shown on every card.
type State string

const (
	Offline State = "offline"
	Idle    State = "idle"
	Online  State = "online"
)

// Snapshot is the JSON payload sent over the broker SSE stream.
type Snapshot struct {
	State        State     `json:"state"`
	Email        string    `json:"email,omitempty"`
	ChangedAt    time.Time `json:"changedAt"`
	IdleAfterSec int       `json:"idleAfterSec"`
}

// Tracker owns one global demo presence state. It is intentionally separate
// from the SSO service so auth remains a redirect/session concern.
type Tracker struct {
	mu        sync.Mutex
	state     State
	email     string
	changedAt time.Time
	idleAfter time.Duration
	now       func() time.Time
	timer     *time.Timer
	events    *gohtmxelm.Broadcaster[Snapshot]
}

func New(idleAfter time.Duration) *Tracker {
	if idleAfter <= 0 {
		idleAfter = 30 * time.Second
	}
	return &Tracker{
		state:     Offline,
		changedAt: time.Now().UTC(),
		idleAfter: idleAfter,
		now:       time.Now,
		events:    gohtmxelm.NewBroadcaster[Snapshot](16),
	}
}

func (t *Tracker) Events() *gohtmxelm.Broadcaster[Snapshot] {
	return t.events
}

func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked()
}

func (t *Tracker) Online(email string) {
	t.mu.Lock()
	t.state = Online
	t.email = email
	t.changedAt = t.now().UTC()
	t.resetIdleTimerLocked()
	snap := t.snapshotLocked()
	t.mu.Unlock()

	t.events.Publish(snap)
}

func (t *Tracker) Touch() {
	t.mu.Lock()
	if t.state == Offline {
		t.mu.Unlock()
		return
	}
	wasIdle := t.state == Idle
	t.state = Online
	t.changedAt = t.now().UTC()
	t.resetIdleTimerLocked()
	snap := t.snapshotLocked()
	t.mu.Unlock()

	if wasIdle {
		t.events.Publish(snap)
	}
}

func (t *Tracker) MarkIdle() {
	t.mu.Lock()
	if t.state != Online {
		t.mu.Unlock()
		return
	}
	t.state = Idle
	t.changedAt = t.now().UTC()
	snap := t.snapshotLocked()
	t.mu.Unlock()

	t.events.Publish(snap)
}

func (t *Tracker) Logout() {
	t.mu.Lock()
	t.state = Offline
	t.email = ""
	t.changedAt = t.now().UTC()
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	snap := t.snapshotLocked()
	t.mu.Unlock()

	t.events.Publish(snap)
}

func (t *Tracker) snapshotLocked() Snapshot {
	return Snapshot{
		State:        t.state,
		Email:        t.email,
		ChangedAt:    t.changedAt,
		IdleAfterSec: int(t.idleAfter.Seconds()),
	}
}

func (t *Tracker) resetIdleTimerLocked() {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = time.AfterFunc(t.idleAfter, t.MarkIdle)
}
