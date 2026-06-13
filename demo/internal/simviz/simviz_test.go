package simviz

import "testing"

// stepToEnd advances a paused sim to its final frame.
func stepToEnd(t *testing.T, s *Sim) {
	t.Helper()
	for i := 0; i < 2000; i++ {
		if s.Current().Final {
			return
		}
		s.Step()
	}
	t.Fatal("never reached final frame")
}

// TestLedgerRecordsCompletion: a finished run lands in the ledger exactly once,
// stamped with the seed that reproduces it, and the verdict matches the final
// frame.
func TestLedgerRecordsCompletion(t *testing.T) {
	s := New()
	s.Pause()
	stepToEnd(t, s)

	got := s.Results()
	if len(got) != 1 {
		t.Fatalf("ledger has %d entries, want 1", len(got))
	}
	rr := got[0]
	if rr.Seed != s.Status().Seed {
		t.Errorf("ledger seed %d, want %d", rr.Seed, s.Status().Seed)
	}
	if rr.Violated != s.Current().Violated {
		t.Errorf("ledger violated=%v, final frame violated=%v", rr.Violated, s.Current().Violated)
	}
	// Stepping past the end must not double-record.
	s.Step()
	if n := len(s.Results()); n != 1 {
		t.Errorf("re-stepping recorded again: %d entries", n)
	}
}

// TestPauseOnFailCapturesViolation: with recovery off, a violated run is
// recorded with its failing invariant AND the loop auto-pauses so the failure
// stays put. Also verifies the durable sink fires.
func TestPauseOnFailCapturesViolation(t *testing.T) {
	s := New()
	var sunk []RunResult
	s.SetOnComplete(func(rr RunResult) { sunk = append(sunk, rr) })
	s.Pause()
	s.SetResync(false) // lossy stream with no recovery -> divergence

	found := false
	for attempt := 0; attempt < 250 && !found; attempt++ {
		stepToEnd(t, s)
		if last := s.Results()[0]; last.Violated {
			found = true
			if last.Invariant == "" || last.Detail == "" {
				t.Errorf("violated entry missing invariant/detail: %+v", last)
			}
			if s.Status().Playing {
				t.Error("loop did not auto-pause on violation")
			}
			break
		}
		s.Reseed() // rebuild keeps playing=false; try the next scenario
	}
	if !found {
		t.Fatal("no violation recorded across 250 seeds with resync off")
	}
	if len(sunk) == 0 {
		t.Error("durable sink never fired for a violation")
	}
}

// TestClear empties the ledger and bumps the version so streams re-render.
func TestClear(t *testing.T) {
	s := New()
	s.Pause()
	stepToEnd(t, s)
	if len(s.Results()) == 0 {
		t.Fatal("expected a recorded run before clear")
	}
	before := s.ResultsVersion()
	s.Clear()
	if len(s.Results()) != 0 {
		t.Error("ledger not empty after Clear")
	}
	if s.ResultsVersion() == before {
		t.Error("ResultsVersion did not change on Clear")
	}
}
