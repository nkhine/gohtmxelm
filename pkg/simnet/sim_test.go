package simnet

import (
	"testing"
)

// TestContract_ConvergesUnderChaos is the headline claim: with the library's
// reconnect-and-rehydrate recovery in place (Resync: true), every surface
// converges to authoritative state under a hostile network — drops, delays,
// reorders, duplicates, and partitions — for BOTH snapshot and delta
// semantics, across many seeds. Any failure prints the seed to reproduce it.
func TestContract_ConvergesUnderChaos(t *testing.T) {
	for _, sem := range []Semantics{Snapshot, Delta} {
		sem := sem
		t.Run(sem.String(), func(t *testing.T) {
			for seed := int64(0); seed < 200; seed++ {
				cfg := Config{
					Seed:      seed,
					Surfaces:  4,
					Buffer:    8,
					Writes:    40,
					Keyspace:  5,
					Semantics: sem,
					Faults:    Chaos(),
					Resync:    true,
				}
				res := New(cfg).Run()
				if !res.OK() {
					for _, v := range res.Violations {
						t.Errorf("%v\n  reproduce: %s\n  recent events:\n    %s",
							v, v.Reproduce(), joinIndent(v.Events))
					}
				}
			}
		})
	}
}

// TestContract_TinyBufferStillConverges stresses the lossy path: a buffer of 1
// guarantees buffer-full drops, so convergence here is owed entirely to
// resync, not to delivery luck.
func TestContract_TinyBufferStillConverges(t *testing.T) {
	for seed := int64(0); seed < 100; seed++ {
		cfg := Config{
			Seed:      seed,
			Surfaces:  6,
			Buffer:    1,
			Writes:    50,
			Keyspace:  4,
			Semantics: Delta,
			Faults:    Chaos(),
			Resync:    true,
		}
		res := New(cfg).Run()
		if !res.OK() {
			t.Fatalf("seed %d diverged with buffer=1: %v (reproduce: Config.Seed=%d)",
				seed, res.Violations[0], seed)
		}
	}
}

// TestContract_DivergesWithoutResync proves the dependency the design rests on:
// turn reconnect-and-rehydrate OFF and a lossy stream cannot keep surfaces
// converged. This MUST find a violation — if it ever stops finding one the
// fault model has gone toothless and the convergence tests are no longer
// meaningful.
func TestContract_DivergesWithoutResync(t *testing.T) {
	for _, sem := range []Semantics{Snapshot, Delta} {
		sem := sem
		t.Run(sem.String(), func(t *testing.T) {
			found := false
			var sawSeed int64
			for seed := int64(0); seed < 200 && !found; seed++ {
				cfg := Config{
					Seed:      seed,
					Surfaces:  4,
					Buffer:    4,
					Writes:    40,
					Keyspace:  5,
					Semantics: sem,
					Faults:    Chaos(),
					Resync:    false, // no recovery
				}
				res := New(cfg).Run()
				if !res.OK() {
					found = true
					sawSeed = seed
				}
			}
			if !found {
				t.Fatalf("expected divergence without resync for %s semantics, but every run converged — fault model may be toothless", sem)
			}
			t.Logf("%s: divergence without resync first seen at seed %d (as designed)", sem, sawSeed)
		})
	}
}

// TestDeterminism verifies the kernel is reproducible: identical Config yields
// an identical event log and result, the property that makes Violation.Seed a
// reliable repro handle.
func TestDeterminism(t *testing.T) {
	cfg := Config{
		Seed: 1234, Surfaces: 5, Buffer: 4, Writes: 60, Keyspace: 6,
		Semantics: Delta, Faults: Chaos(), Resync: true,
	}
	a := New(cfg).Run()
	b := New(cfg).Run()

	if len(a.Log) != len(b.Log) {
		t.Fatalf("log length differs: %d vs %d", len(a.Log), len(b.Log))
	}
	for i := range a.Log {
		if a.Log[i] != b.Log[i] {
			t.Fatalf("log diverged at line %d:\n  a: %s\n  b: %s", i, a.Log[i], b.Log[i])
		}
	}
	if a.Auth.Version != b.Auth.Version || a.Steps != b.Steps {
		t.Fatalf("results differ: a(ver=%d,steps=%d) b(ver=%d,steps=%d)",
			a.Auth.Version, a.Steps, b.Auth.Version, b.Steps)
	}
}

// TestNoFaultsIsExact sanity-checks the kernel itself: with no faults and ample
// buffer, every surface should track every write exactly via the stream alone,
// no resync required.
func TestNoFaultsIsExact(t *testing.T) {
	cfg := Config{
		Seed: 7, Surfaces: 3, Buffer: 64, Writes: 30, Keyspace: 5,
		Semantics: Delta, Resync: false,
	}
	res := New(cfg).Run()
	if !res.OK() {
		t.Fatalf("clean network should converge without resync: %v", res.Violations[0])
	}
	if res.Auth.Version != 30 {
		t.Fatalf("expected authoritative version 30, got %d", res.Auth.Version)
	}
}

// TestInvariants_DirectUnit exercises the shared invariants in isolation so the
// real-code tests can rely on them too.
func TestInvariants_DirectUnit(t *testing.T) {
	auth := Authoritative{Data: map[string]string{"k0": "v2", "k1": "v1"}, Version: 2}

	ok := []View{{Label: "s", Data: map[string]string{"k0": "v2", "k1": "v1"}, Version: 2}}
	if err := CheckConvergence(auth, ok); err != nil {
		t.Errorf("converged view rejected: %v", err)
	}

	staleVer := []View{{Label: "s", Data: map[string]string{"k0": "v2", "k1": "v1"}, Version: 1}}
	if err := CheckConvergence(auth, staleVer); err == nil {
		t.Error("stale version accepted as converged")
	}

	wrongVal := []View{{Label: "s", Data: map[string]string{"k0": "v1", "k1": "v1"}, Version: 2}}
	if err := CheckConvergence(auth, wrongVal); err == nil {
		t.Error("wrong value accepted as converged")
	}

	if err := CheckMonotonic("s", []int{0, 1, 1, 2, 5}); err != nil {
		t.Errorf("non-decreasing history rejected: %v", err)
	}
	if err := CheckMonotonic("s", []int{0, 1, 3, 2}); err == nil {
		t.Error("regressing history accepted")
	}
}

// TestRecord_ProducesReplayableTrace checks the trace API the simulator card
// streams: frames are emitted, every frame knows the run total, the terminal
// frame is flagged, and a diverging run surfaces the violation on that frame.
func TestRecord_ProducesReplayableTrace(t *testing.T) {
	healthy := Config{
		Seed: 3, Surfaces: 4, Buffer: 6, Writes: 20, Keyspace: 5,
		Semantics: Delta, Faults: Chaos(), Resync: true,
	}.Record()

	if len(healthy.Frames) == 0 {
		t.Fatal("no frames recorded")
	}
	last := healthy.Frames[len(healthy.Frames)-1]
	if !last.Final {
		t.Error("terminal frame not flagged Final")
	}
	if last.Violated {
		t.Errorf("healthy run reported a violation: %s", last.Violation)
	}
	for i, f := range healthy.Frames {
		if f.Total != len(healthy.Frames) {
			t.Fatalf("frame %d Total=%d, want %d", i, f.Total, len(healthy.Frames))
		}
		if len(f.Surfaces) != healthy.Config.Surfaces {
			t.Fatalf("frame %d has %d surfaces, want %d", i, len(f.Surfaces), healthy.Config.Surfaces)
		}
	}

	// A run with recovery disabled must end Violated on the terminal frame for
	// at least one seed.
	broken := false
	for seed := int64(0); seed < 200 && !broken; seed++ {
		tr := Config{
			Seed: seed, Surfaces: 4, Buffer: 4, Writes: 30, Keyspace: 5,
			Semantics: Delta, Faults: Chaos(), Resync: false,
		}.Record()
		if last := tr.Frames[len(tr.Frames)-1]; last.Violated {
			broken = true
			if last.Violation == "" {
				t.Error("violated terminal frame carries no detail")
			}
		}
	}
	if !broken {
		t.Error("no diverging trace found without resync")
	}
}

func joinIndent(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n    "
		}
		out += l
	}
	return out
}
