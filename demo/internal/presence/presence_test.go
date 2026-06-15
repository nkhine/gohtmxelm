package presence

import (
	"testing"
	"time"
)

func TestTrackerLoginIdleTouchLogout(t *testing.T) {
	tracker := New(time.Minute)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	defer tracker.Logout()

	ch := tracker.Events().Subscribe()
	defer tracker.Events().Unsubscribe(ch)

	tracker.Online("org-admin@customer-a.local")
	assertEvent(t, ch, Online)
	snap := tracker.Snapshot()
	if snap.State != Online || snap.Email != "org-admin@customer-a.local" {
		t.Fatalf("online snapshot = %+v", snap)
	}

	now = now.Add(20 * time.Second)
	tracker.MarkIdle()
	assertEvent(t, ch, Idle)
	if got := tracker.Snapshot().State; got != Idle {
		t.Fatalf("state after idle = %q, want %q", got, Idle)
	}

	now = now.Add(time.Second)
	tracker.Touch()
	assertEvent(t, ch, Online)
	if got := tracker.Snapshot().State; got != Online {
		t.Fatalf("state after touch = %q, want %q", got, Online)
	}

	tracker.Logout()
	assertEvent(t, ch, Offline)
	snap = tracker.Snapshot()
	if snap.State != Offline || snap.Email != "" {
		t.Fatalf("logout snapshot = %+v", snap)
	}
}

func TestTouchIgnoredWhenOffline(t *testing.T) {
	tracker := New(time.Minute)
	ch := tracker.Events().Subscribe()
	defer tracker.Events().Unsubscribe(ch)

	tracker.Touch()

	select {
	case ev := <-ch:
		t.Fatalf("unexpected event = %+v", ev)
	default:
	}
}

func assertEvent(t *testing.T, ch <-chan Snapshot, want State) {
	t.Helper()
	select {
	case got := <-ch:
		if got.State != want {
			t.Fatalf("event state = %q, want %q", got.State, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %q event", want)
	}
}
