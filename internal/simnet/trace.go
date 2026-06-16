package simnet

// This file adds an observable trace on top of the kernel: a run can be
// recorded frame-by-frame so a UI can replay it (the demo's radial simulator
// card streams these frames over the library's own Broadcaster→SSE→Elm path).
// The data here is deliberately lean and JSON-friendly — no maps of state, just
// the verdicts and per-surface status a visualiser needs.

// ActionKind classifies what happened in a frame, so a renderer can choose an
// animation (a packet flying, a drop splatting, a node dimming).
type ActionKind string

const (
	ActHydrate   ActionKind = "hydrate"   // surface connected and loaded the snapshot
	ActWrite     ActionKind = "write"     // authoritative state changed
	ActDeliver   ActionKind = "deliver"   // surface applied an event
	ActDrop      ActionKind = "drop"      // event dropped (explicit or buffer-full)
	ActDuplicate ActionKind = "duplicate" // event scheduled to arrive twice
	ActGap       ActionKind = "gap"       // delta arrived out of order; surface must resync
	ActPartition ActionKind = "partition" // surface went dark
	ActReconnect ActionKind = "reconnect" // surface reconnected and re-hydrated
)

// Action is one discrete event in the simulation timeline.
type Action struct {
	Kind    ActionKind `json:"kind"`
	Surface int        `json:"surface"` // index of the affected surface, or -1 for server-wide
	Version int        `json:"version"`
	Label   string     `json:"label"`
}

// SurfaceState is a lean per-surface snapshot for the visualiser.
type SurfaceState struct {
	Label      string `json:"label"`
	Kind       string `json:"kind"`   // transport tag from Config.Kinds (elm|htmx|datastar|"")
	Status     string `json:"status"` // synced | behind | gap | partitioned
	Version    int    `json:"version"`
	BufferUsed int    `json:"bufferUsed"`
	BufferCap  int    `json:"bufferCap"`
}

// Frame is the full observable state at one step: what just happened, the
// authoritative version, every surface's status, and the live invariant
// verdicts.
type Frame struct {
	Seed        int64          `json:"seed"`
	Step        int            `json:"step"`
	Total       int            `json:"total"` // total frames in the run (0 until known)
	Action      Action         `json:"action"`
	AuthVersion int            `json:"authVersion"`
	AuthKeys    int            `json:"authKeys"`
	Surfaces    []SurfaceState `json:"surfaces"`
	Semantics   string         `json:"semantics"`
	Resync      bool           `json:"resync"`
	// Converged is the LIVE verdict: every surface presents authoritative state
	// right now. It breathes false as surfaces fall behind and true as they
	// catch up — the heartbeat of the contract.
	Converged bool `json:"converged"`
	// Final is true on the terminal frame; Violated then reports whether the
	// run ended in a contract breach (the seed reproduces it).
	Final     bool   `json:"final"`
	Violated  bool   `json:"violated"`
	Violation string `json:"violation,omitempty"`
}

// Trace is a fully recorded run: every frame plus the final invariant result.
type Trace struct {
	Seed   int64   `json:"seed"`
	Config Config  `json:"-"`
	Frames []Frame `json:"frames"`
	Result Result  `json:"-"`
}

// Record runs cfg to completion while capturing a Frame for every action, then
// stamps each frame with the total count and the terminal verdict. The
// recording shares the exact kernel the test suite drives, so what you watch is
// what the contract tests assert.
func (cfg Config) Record() Trace {
	k := New(cfg)
	var frames []Frame
	k.hook = func(a Action) { frames = append(frames, k.frameFor(a)) }
	res := k.Run()

	total := len(frames)
	for i := range frames {
		frames[i].Total = total
	}
	if n := len(frames); n > 0 {
		frames[n-1].Final = true
		frames[n-1].Violated = !res.OK()
		if !res.OK() {
			frames[n-1].Violation = res.Violations[0].Detail
		}
	}
	return Trace{Seed: cfg.Seed, Config: k.cfg, Frames: frames, Result: res}
}

// frameFor snapshots the kernel right after action a.
func (k *Kernel) frameFor(a Action) Frame {
	surfaces := make([]SurfaceState, len(k.surf))
	converged := true
	for i, s := range k.surf {
		st := statusOf(s, k.srvVer)
		if st != "synced" {
			converged = false
		}
		kind := ""
		if i < len(k.cfg.Kinds) {
			kind = k.cfg.Kinds[i]
		}
		surfaces[i] = SurfaceState{
			Label:      s.label,
			Kind:       kind,
			Status:     st,
			Version:    s.version,
			BufferUsed: s.buf,
			BufferCap:  k.cfg.Buffer,
		}
	}
	return Frame{
		Seed:        k.cfg.Seed,
		Step:        len(k.log), // monotonic frame counter
		Action:      a,
		AuthVersion: k.srvVer,
		AuthKeys:    len(k.srvData),
		Surfaces:    surfaces,
		Semantics:   k.cfg.Semantics.String(),
		Resync:      k.cfg.Resync,
		Converged:   converged,
	}
}

func statusOf(s *surface, authVer int) string {
	switch {
	case !s.connected:
		return "partitioned"
	case s.gap:
		return "gap"
	case s.version < authVer:
		return "behind"
	default:
		return "synced"
	}
}
