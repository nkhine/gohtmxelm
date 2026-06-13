package main

import (
	"testing"
	"time"
)

// testClock is a manually advanced clock for deterministic stopwatch tests.
type testClock struct {
	t time.Time
}

func (c *testClock) now() time.Time { return c.t }

func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// newTestStopwatch returns a stopwatch wired to a controllable clock. Run is
// never started, so tick() is exercised directly where needed.
func newTestStopwatch() (*Stopwatch, *testClock) {
	clock := &testClock{t: time.Unix(0, 0)}
	sw := NewStopwatch()
	sw.now = clock.now
	return sw, clock
}

func TestStartStopAccumulatesAcrossPauses(t *testing.T) {
	sw, clock := newTestStopwatch()

	sw.Start()
	clock.advance(time.Second)
	snap := sw.Stop()
	if snap.Running {
		t.Fatal("should not be running after Stop")
	}
	if snap.ElapsedMs != 1000 {
		t.Fatalf("expected 1000ms after first run, got %d", snap.ElapsedMs)
	}

	// Time passing while stopped must not count.
	clock.advance(5 * time.Second)
	if got := sw.Snapshot().ElapsedMs; got != 1000 {
		t.Fatalf("elapsed must not advance while paused, got %d", got)
	}

	sw.Start()
	clock.advance(2 * time.Second)
	snap = sw.Stop()
	if snap.ElapsedMs != 3000 {
		t.Fatalf("expected 3000ms accumulated across pauses, got %d", snap.ElapsedMs)
	}
}

func TestStartIsIdempotent(t *testing.T) {
	sw, clock := newTestStopwatch()

	sw.Start()
	clock.advance(time.Second)
	sw.Start() // must not reset startedAt
	clock.advance(time.Second)
	snap := sw.Stop()
	if snap.ElapsedMs != 2000 {
		t.Fatalf("repeated Start must not reset the clock, got %d", snap.ElapsedMs)
	}
}

func TestSnapshotElapsedWhileRunning(t *testing.T) {
	sw, clock := newTestStopwatch()
	sw.Start()
	clock.advance(500 * time.Millisecond)
	snap := sw.Snapshot()
	if !snap.Running || snap.ElapsedMs != 500 {
		t.Fatalf("expected running 500ms snapshot, got running=%v elapsed=%d", snap.Running, snap.ElapsedMs)
	}
}

func TestStopWhenNotRunningIsNoOp(t *testing.T) {
	sw, _ := newTestStopwatch()
	snap := sw.Stop()
	if snap.Running || snap.ElapsedMs != 0 {
		t.Fatalf("Stop on fresh stopwatch should be a no-op, got %+v", snap)
	}
}

func TestResetClearsEverything(t *testing.T) {
	sw, clock := newTestStopwatch()
	sw.Start()
	clock.advance(time.Second)
	sw.Lap()
	snap := sw.Reset()
	if snap.Running || snap.ElapsedMs != 0 || len(snap.Laps) != 0 {
		t.Fatalf("Reset should clear running, elapsed, and laps, got %+v", snap)
	}
}

func TestLapNumberingAndCumulativeTimes(t *testing.T) {
	sw, clock := newTestStopwatch()
	sw.Start()

	clock.advance(time.Second)
	sw.Lap() // cumulative 1000ms
	clock.advance(2 * time.Second)
	snap := sw.Lap() // cumulative 3000ms

	if len(snap.Laps) != 2 {
		t.Fatalf("expected 2 laps, got %d", len(snap.Laps))
	}
	// Newest first.
	if snap.Laps[0].Number != 2 || snap.Laps[0].ElapsedMs != 3000 {
		t.Fatalf("newest lap should be #2 at 3000ms, got %+v", snap.Laps[0])
	}
	if snap.Laps[1].Number != 1 || snap.Laps[1].ElapsedMs != 1000 {
		t.Fatalf("oldest lap should be #1 at 1000ms, got %+v", snap.Laps[1])
	}
}

func TestLapAtZeroElapsedIsNoOp(t *testing.T) {
	sw, _ := newTestStopwatch()
	snap := sw.Lap()
	if len(snap.Laps) != 0 {
		t.Fatalf("Lap with zero elapsed should record nothing, got %d laps", len(snap.Laps))
	}
}

func TestSubscriberReceivesStateChange(t *testing.T) {
	sw, _ := newTestStopwatch()
	ch := sw.Subscribe()
	defer sw.Unsubscribe(ch)

	sw.Start()
	select {
	case ev := <-ch:
		if !ev.StateChange {
			t.Fatal("Start must emit a StateChange event")
		}
		if !ev.Snapshot.Running {
			t.Fatal("Start event snapshot should be running")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stopwatch event")
	}
}

func TestTickEmitsNonStateChangeAndStopsWhenPaused(t *testing.T) {
	sw, clock := newTestStopwatch()
	ch := sw.Subscribe()
	defer sw.Unsubscribe(ch)

	sw.Start()
	<-ch // drain the Start state-change event

	clock.advance(100 * time.Millisecond)
	if _, ok := sw.tick(); !ok {
		t.Fatal("tick should report ok while running")
	}
	select {
	case ev := <-ch:
		if ev.StateChange {
			t.Fatal("tick must emit a non-state-change event")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for tick event")
	}

	sw.Stop()
	<-ch // drain Stop event
	if _, ok := sw.tick(); ok {
		t.Fatal("tick must report ok=false when paused so Run can stop the ticker")
	}
}

func TestStopwatchCanLap(t *testing.T) {
	cases := []struct {
		name string
		snap StopwatchSnapshot
		want bool
	}{
		{"running", StopwatchSnapshot{Running: true}, true},
		{"paused with elapsed", StopwatchSnapshot{ElapsedMs: 500}, true},
		{"fresh", StopwatchSnapshot{}, false},
	}
	for _, tc := range cases {
		if got := stopwatchCanLap(tc.snap); got != tc.want {
			t.Errorf("%s: stopwatchCanLap = %v, want %v", tc.name, got, tc.want)
		}
	}
}
