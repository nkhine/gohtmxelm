// Package simviz drives the simnet contract harness as a live, watchable
// stream. It records a deterministic run with simnet, then replays it frame by
// frame over a gohtmxelm.Broadcaster — the same fan-out the rest of the demo
// uses — so the simulator card visualises the library's own invariants using
// the library's own pattern. When a run finishes it pauses on the verdict,
// reseeds, and runs again, giving an endless auto-looping demo that the user
// can also drive by hand.
package simviz

import (
	"context"
	"sync"
	"time"

	"github.com/nkhine/gohtmxelm"
	"github.com/nkhine/gohtmxelm/internal/simnet"
)

const (
	frameInterval = 550 * time.Millisecond // wall-clock pace between frames
	holdTicks     = 3                      // ticks to dwell on the final verdict before reseeding
	ledgerMax     = 40                     // completed runs kept in memory
)

// surfaceKinds are the demo's coordinating surfaces, mirroring the message
// workbench: two Elm islands and the HTMX table ride the broker SSE stream;
// the Datastar panel has its own stream that bypasses bridge.js. The viz draws
// each kind's real path; the simulation treats them identically.
var surfaceKinds = []string{"elm", "elm", "htmx", "datastar"}

// RunResult is the durable record of one completed run: its identity, its
// config, and the invariant verdict. Violations carry the failing invariant and
// detail so the seed reproduces the failure for a fix — the reason the ledger
// exists at all, since the on-screen verdict otherwise vanishes on the next
// loop.
type RunResult struct {
	Seed      int64  `json:"seed"`
	Semantics string `json:"semantics"`
	Intensity string `json:"intensity"`
	Resync    bool   `json:"resync"`
	Surfaces  int    `json:"surfaces"`
	Writes    int    `json:"writes"`
	Steps     int    `json:"steps"`
	Converged bool   `json:"converged"`
	Violated  bool   `json:"violated"`
	Invariant string `json:"invariant,omitempty"`
	Detail    string `json:"detail,omitempty"`
	At        string `json:"at"`
	// Deterministic behaviour metrics derived from the trace — reproducible
	// from the seed, unlike wall-clock or host CPU/memory.
	Drops      int `json:"drops"`
	Duplicates int `json:"duplicates"`
	Partitions int `json:"partitions"`
	Reconnects int `json:"reconnects"`
	Gaps       int `json:"gaps"`
	MaxLag     int `json:"maxLag"` // furthest any surface fell behind authoritative
}

// Intensity names a fault profile the card can switch between.
type Intensity string

const (
	Calm   Intensity = "calm"
	Normal Intensity = "normal"
	Storm  Intensity = "storm"
)

// profile maps an Intensity to a simnet fault profile.
func profile(i Intensity) simnet.FaultProfile {
	switch i {
	case Calm:
		return simnet.FaultProfile{
			DropRate: 0.06, DelayRate: 0.12, DelayMax: 3,
			DuplicateRate: 0.04, PartitionRate: 0.03, ReconnectAfter: 3,
		}
	case Storm:
		return simnet.FaultProfile{
			DropRate: 0.35, DelayRate: 0.40, DelayMax: 5,
			DuplicateRate: 0.20, PartitionRate: 0.18, ReconnectAfter: 3,
		}
	default: // Normal
		return simnet.Chaos()
	}
}

// Sim owns the running simulation: the current recorded trace, a playback
// cursor, and the broadcaster that fans each frame out to SSE subscribers.
type Sim struct {
	mu          sync.Mutex
	bc          *gohtmxelm.Broadcaster[simnet.Frame]
	frames      []simnet.Frame
	result      simnet.Result
	cursor      int
	hold        int
	recorded    bool // current run already written to the ledger
	playing     bool
	seed        int64
	intensity   Intensity
	resync      bool
	semantics   simnet.Semantics
	pauseOnFail bool
	ledger      []RunResult     // newest first, capped at ledgerMax
	resultsVer  int             // bumped whenever the ledger changes
	onComplete  func(RunResult) // optional durable sink (e.g. JSONL on disk)
}

// New builds a Sim with sensible demo defaults and records its first run. It
// pauses on the first violation by default so a failure stays on screen.
func New() *Sim {
	s := &Sim{
		bc:          gohtmxelm.NewBroadcaster[simnet.Frame](32),
		playing:     true,
		seed:        1,
		intensity:   Normal,
		resync:      true,
		semantics:   simnet.Delta,
		pauseOnFail: true,
	}
	s.rebuildLocked()
	return s
}

// Events exposes the broadcaster so HTTP handlers can subscribe.
func (s *Sim) Events() *gohtmxelm.Broadcaster[simnet.Frame] { return s.bc }

// Run advances playback on a fixed cadence until ctx is cancelled. It mirrors
// stopwatch.Run: one goroutine, stopped cleanly on shutdown.
func (s *Sim) Run(ctx context.Context) {
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Sim) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.playing {
		return
	}
	if s.cursor >= len(s.frames)-1 {
		// Run complete: record the verdict (may auto-pause on a violation),
		// then dwell on it before starting a fresh scenario.
		s.recordCompletionLocked()
		if !s.playing {
			s.publishLocked()
			return
		}
		s.hold++
		if s.hold >= holdTicks {
			s.seed++
			s.rebuildLocked()
		}
		s.publishLocked()
		return
	}
	s.cursor++
	s.publishLocked()
}

// recordCompletionLocked writes the finished run to the ledger exactly once,
// persists violations through the sink, and pauses on failure when configured.
func (s *Sim) recordCompletionLocked() {
	if s.recorded || len(s.frames) == 0 {
		return
	}
	s.recorded = true
	last := s.frames[len(s.frames)-1]
	rr := RunResult{
		Seed:      s.seed,
		Semantics: s.semantics.String(),
		Intensity: string(s.intensity),
		Resync:    s.resync,
		Surfaces:  len(last.Surfaces),
		Writes:    last.AuthVersion,
		Steps:     last.Step,
		Converged: last.Converged && !last.Violated,
		Violated:  last.Violated,
		At:        time.Now().Format(time.RFC3339),
	}
	if len(s.result.Violations) > 0 {
		rr.Invariant = s.result.Violations[0].Invariant
		rr.Detail = s.result.Violations[0].Detail
	}
	// Tally faults and worst lag across the whole trace — deterministic, so
	// they mean the same thing on every replay of this seed.
	for _, fr := range s.frames {
		switch fr.Action.Kind {
		case simnet.ActDrop:
			rr.Drops++
		case simnet.ActDuplicate:
			rr.Duplicates++
		case simnet.ActPartition:
			rr.Partitions++
		case simnet.ActReconnect:
			rr.Reconnects++
		case simnet.ActGap:
			rr.Gaps++
		}
		minVer := fr.AuthVersion
		for _, su := range fr.Surfaces {
			if su.Version < minVer {
				minVer = su.Version
			}
		}
		if lag := fr.AuthVersion - minVer; lag > rr.MaxLag {
			rr.MaxLag = lag
		}
	}
	s.ledger = append([]RunResult{rr}, s.ledger...)
	if len(s.ledger) > ledgerMax {
		s.ledger = s.ledger[:ledgerMax]
	}
	s.resultsVer++
	if rr.Violated {
		if s.onComplete != nil {
			s.onComplete(rr)
		}
		if s.pauseOnFail {
			s.playing = false
		}
	}
}

// Current returns the frame the playback is sitting on — used to hydrate a
// newly connected stream.
func (s *Sim) Current() simnet.Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentLocked()
}

// Status is the playback-level state the Datastar panel shows alongside the
// frame's own invariant verdicts.
type Status struct {
	Playing     bool
	Seed        int64
	Intensity   string
	Resync      bool
	Semantics   string
	PauseOnFail bool
}

// Status reports the current playback state.
func (s *Sim) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{
		Playing:     s.playing,
		Seed:        s.seed,
		Intensity:   string(s.intensity),
		Resync:      s.resync,
		Semantics:   s.semantics.String(),
		PauseOnFail: s.pauseOnFail,
	}
}

// Results returns the run ledger, newest first.
func (s *Sim) Results() []RunResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]RunResult(nil), s.ledger...)
}

// ResultsVersion changes whenever the ledger does, so a stream can re-render
// the ledger only when it actually changed instead of on every frame.
func (s *Sim) ResultsVersion() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resultsVer
}

// Clear empties the ledger.
func (s *Sim) Clear() { s.set(func() { s.ledger = nil; s.resultsVer++ }) }

// SetPauseOnFail controls whether a violation auto-pauses the loop.
func (s *Sim) SetPauseOnFail(on bool) { s.set(func() { s.pauseOnFail = on }) }

// SetOnComplete registers a durable sink invoked once per violated run (under
// the lock, so it must not call back into the Sim).
func (s *Sim) SetOnComplete(fn func(RunResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onComplete = fn
}

// --- controls -------------------------------------------------------------

func (s *Sim) Play()  { s.set(func() { s.playing = true }) }
func (s *Sim) Pause() { s.set(func() { s.playing = false }) }

// Step advances exactly one frame while paused (no-op while playing).
func (s *Sim) Step() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.playing {
		return
	}
	if s.cursor < len(s.frames)-1 {
		s.cursor++
	}
	if s.cursor >= len(s.frames)-1 {
		s.recordCompletionLocked()
	}
	s.publishLocked()
}

// Reseed jumps to a brand-new scenario immediately.
func (s *Sim) Reseed() { s.set(func() { s.seed++; s.rebuildLocked() }) }

// SetIntensity changes the fault profile and restarts the scenario.
func (s *Sim) SetIntensity(i Intensity) {
	s.set(func() { s.intensity = i; s.rebuildLocked() })
}

// SetResync toggles the reconnect-and-rehydrate recovery. Off demonstrates the
// divergence a purely lossy stream produces.
func (s *Sim) SetResync(on bool) { s.set(func() { s.resync = on; s.rebuildLocked() }) }

// SetSemantics selects snapshot or delta event carriage and restarts the run.
func (s *Sim) SetSemantics(sem simnet.Semantics) {
	s.set(func() { s.semantics = sem; s.rebuildLocked() })
}

// Replay loads an exact past run — its seed and every knob — paused at the
// first frame, so a ledger row can reproduce a failure deterministically and be
// stepped or played through again.
func (s *Sim) Replay(seed int64, semantics simnet.Semantics, intensity Intensity, resync bool) {
	s.set(func() {
		s.seed = seed
		s.semantics = semantics
		s.intensity = intensity
		s.resync = resync
		s.playing = false // loaded paused at frame 0; user presses ▶ or Step
		s.rebuildLocked()
	})
}

// LoadHistory seeds the ledger from previously persisted runs so failures
// survive a restart. rs is in file (oldest-first) order; the ledger is
// newest-first and capped.
func (s *Sim) LoadHistory(rs []RunResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RunResult, 0, len(rs))
	for i := len(rs) - 1; i >= 0 && len(out) < ledgerMax; i-- {
		out = append(out, rs[i])
	}
	s.ledger = out
	s.resultsVer++
}

// set runs mutate under the lock then publishes the resulting frame.
func (s *Sim) set(mutate func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mutate()
	s.publishLocked()
}

// --- internals ------------------------------------------------------------

func (s *Sim) rebuildLocked() {
	cfg := simnet.Config{
		Seed:      s.seed,
		Surfaces:  len(surfaceKinds),
		Buffer:    6,
		Writes:    22,
		Keyspace:  5,
		Semantics: s.semantics,
		Faults:    profile(s.intensity),
		Resync:    s.resync,
		Kinds:     surfaceKinds,
	}
	tr := cfg.Record()
	s.frames = tr.Frames
	s.result = tr.Result
	s.cursor = 0
	s.hold = 0
	s.recorded = false
}

func (s *Sim) currentLocked() simnet.Frame {
	if len(s.frames) == 0 {
		return simnet.Frame{}
	}
	if s.cursor >= len(s.frames) {
		s.cursor = len(s.frames) - 1
	}
	return s.frames[s.cursor]
}

func (s *Sim) publishLocked() { s.bc.Publish(s.currentLocked()) }
