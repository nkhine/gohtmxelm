package simnet

import (
	"fmt"
	"math/rand/v2"
)

// Semantics selects how a state change is carried to surfaces.
type Semantics int

const (
	// Snapshot events carry the full current state (idempotent, last-write-wins).
	Snapshot Semantics = iota
	// Delta events carry a single changed key and must apply in version order.
	Delta
)

func (s Semantics) String() string {
	if s == Delta {
		return "delta"
	}
	return "snapshot"
}

// FaultProfile is the adversarial network. Every rate is a per-event (or
// per-surface, for Partition) probability in [0,1], sampled from the seeded
// RNG so a given Seed always produces the same faults.
type FaultProfile struct {
	// DropRate drops an event outright before it is even buffered.
	DropRate float64
	// DelayRate defers an event by DelayMax (1..) steps, which also reorders it
	// relative to events that were not delayed.
	DelayRate float64
	// DelayMax is the largest delay in steps (>=1 when DelayRate>0).
	DelayMax int
	// DuplicateRate delivers an event twice.
	DuplicateRate float64
	// PartitionRate disconnects a surface for ReconnectAfter steps, dropping its
	// buffered events. On reconnect it re-hydrates (if Resync is set).
	PartitionRate float64
	// ReconnectAfter is how many steps a partitioned surface stays dark.
	ReconnectAfter int
}

// Chaos is a representative hostile profile: frequent drops, reordering delays,
// duplicates, and partitions. Useful as a default for stress matrices.
func Chaos() FaultProfile {
	return FaultProfile{
		DropRate:       0.25,
		DelayRate:      0.30,
		DelayMax:       4,
		DuplicateRate:  0.15,
		PartitionRate:  0.10,
		ReconnectAfter: 3,
	}
}

// Config fully determines a run. Same Config (and thus same Seed) => same
// execution, same result.
type Config struct {
	Seed      int64
	Surfaces  int          // number of HTMX/Datastar/Elm-style surfaces
	Buffer    int          // per-surface SSE buffer depth (mirrors Broadcaster buffer)
	Writes    int          // authoritative writes performed during the run
	Keyspace  int          // distinct keys writes target (>=1)
	Semantics Semantics    // Snapshot or Delta event carriage
	Faults    FaultProfile // the adversarial network
	// Kinds optionally tags each surface with its transport (e.g. "elm",
	// "htmx", "datastar") purely for visualisation; it does not affect the
	// simulation. Index i tags surface i; missing entries default to "".
	Kinds []string
	// Resync models the library's reconnect-and-rehydrate recovery. When true,
	// any surface that drops/gaps/partitions schedules a hydrate that restores
	// it from the authoritative snapshot. When false, no recovery happens and
	// the run exposes the divergence a purely lossy stream produces.
	Resync bool
}

func (c Config) withDefaults() Config {
	if c.Surfaces < 1 {
		c.Surfaces = 1
	}
	if c.Buffer < 1 {
		c.Buffer = 1
	}
	if c.Keyspace < 1 {
		c.Keyspace = 1
	}
	if c.Faults.DelayRate > 0 && c.Faults.DelayMax < 1 {
		c.Faults.DelayMax = 1
	}
	if c.Faults.PartitionRate > 0 && c.Faults.ReconnectAfter < 1 {
		c.Faults.ReconnectAfter = 1
	}
	return c
}

// Violation is a reproducible contract failure: an invariant that did not hold,
// stamped with the seed and step needed to replay it.
type Violation struct {
	Seed      int64
	Step      int
	Invariant string
	Detail    string
	Events    []string // tail of the event log leading up to the failure
}

func (v Violation) Error() string {
	return fmt.Sprintf("simnet: invariant %q violated at step %d (seed %d): %s",
		v.Invariant, v.Step, v.Seed, v.Detail)
}

// Reproduce returns a one-line hint for replaying the exact failing run.
func (v Violation) Reproduce() string {
	return fmt.Sprintf("re-run with Config.Seed = %d", v.Seed)
}

// Result is the outcome of a run.
type Result struct {
	Seed       int64
	Steps      int
	Auth       Authoritative
	Views      []View
	Violations []Violation
	Log        []string
}

// OK reports whether the run satisfied every invariant.
func (r Result) OK() bool { return len(r.Violations) == 0 }

// event is one state change in flight toward a surface.
type event struct {
	version  int
	isSnap   bool
	key, val string            // delta payload
	snapshot map[string]string // snapshot payload
}

// delivery is a scheduled arrival at a surface (or a scheduled reconnect).
type delivery struct {
	at        int // step it becomes due
	seq       int // global ordering tiebreak — keeps the kernel deterministic
	surface   int
	ev        event
	reconnect bool
}

type surface struct {
	label     string
	connected bool
	buf       int               // current buffer occupancy
	data      map[string]string // applied state
	version   int               // authoritative version `data` reflects
	gap       bool              // missed an in-order delta; stuck until resync
	history   []int             // applied-version history (for CheckMonotonic)
}

// Kernel is the single-threaded simulation engine.
type Kernel struct {
	cfg     Config
	rng     *rand.Rand
	step    int
	seq     int
	srvData map[string]string
	srvVer  int
	surf    []*surface
	queue   []delivery
	log     []string
	hook    func(Action) // optional observer; set by Record to capture frames
}

// New builds a kernel for cfg.
func New(cfg Config) *Kernel {
	cfg = cfg.withDefaults()
	k := &Kernel{
		cfg:     cfg,
		rng:     rand.New(rand.NewPCG(uint64(cfg.Seed), uint64(cfg.Seed)^0x9e3779b9)),
		srvData: map[string]string{},
		surf:    make([]*surface, cfg.Surfaces),
	}
	for i := range k.surf {
		k.surf[i] = &surface{
			label:     fmt.Sprintf("surface-%d", i),
			connected: true,
			data:      map[string]string{},
		}
	}
	return k
}

// Run executes the scenario to quiescence and checks the contract, returning
// the result. It connects every surface (initial hydrate), performs Writes
// authoritative changes interleaved with delivery steps under the fault
// profile, drains the network, then asserts convergence.
func (k *Kernel) Run() Result {
	// Step 0: every surface connects and hydrates the empty initial snapshot.
	for i := range k.surf {
		k.hydrate(i)
		k.emit(Action{Kind: ActHydrate, Surface: i, Version: k.srvVer,
			Label: fmt.Sprintf("%s connect+hydrate v%d", k.surf[i].label, k.srvVer)})
	}

	// One write per step for the first Writes steps; deliveries fan out from
	// there and may be delayed past the last write.
	for w := 0; w < k.cfg.Writes; w++ {
		k.step++
		k.applyWrite(w)
		k.drainDue()
	}

	// Drain everything still in flight (delayed events, scheduled reconnects).
	// Termination is guaranteed: writes are finite, each miss schedules at most
	// one reconnect, and a reconnect fully converges a surface without
	// producing new misses.
	for len(k.queue) > 0 {
		k.step++
		k.drainDue()
	}

	// Quiesce: if resync is enabled, any surface still behind reconnects now —
	// the eventual reconnect the lossy contract promises.
	if k.cfg.Resync {
		for i, s := range k.surf {
			if !s.connected || s.gap || s.version != k.srvVer {
				k.step++
				k.reconnect(i)
			}
		}
	}

	return k.finish()
}

// applyWrite mutates authoritative state and fans the change out to surfaces.
func (k *Kernel) applyWrite(w int) {
	key := fmt.Sprintf("k%d", k.rng.IntN(k.cfg.Keyspace))
	k.srvVer++
	val := fmt.Sprintf("v%d", k.srvVer)
	k.srvData[key] = val
	k.emit(Action{Kind: ActWrite, Surface: -1, Version: k.srvVer,
		Label: fmt.Sprintf("write v%d %s=%s", k.srvVer, key, val)})

	ev := event{version: k.srvVer}
	if k.cfg.Semantics == Snapshot {
		ev.isSnap = true
		ev.snapshot = cloneMap(k.srvData)
	} else {
		ev.key, ev.val = key, val
	}

	for i := range k.surf {
		k.fanOut(i, ev)
	}
}

// fanOut applies the fault model to one surface for one event.
func (k *Kernel) fanOut(i int, ev event) {
	s := k.surf[i]
	f := k.cfg.Faults

	// Partition: surface goes dark, its buffer is discarded, reconnect later.
	if s.connected && f.PartitionRate > 0 && k.chance(f.PartitionRate) {
		s.connected = false
		s.buf = 0
		s.gap = true
		k.emit(Action{Kind: ActPartition, Surface: i, Version: ev.version,
			Label: fmt.Sprintf("%s partitioned", s.label)})
		if k.cfg.Resync {
			k.schedule(delivery{at: k.step + f.ReconnectAfter, surface: i, reconnect: true})
		}
		return
	}
	if !s.connected {
		return // dark: event lost, recovery is the scheduled reconnect
	}

	// Explicit drop.
	if f.DropRate > 0 && k.chance(f.DropRate) {
		k.emit(Action{Kind: ActDrop, Surface: i, Version: ev.version,
			Label: fmt.Sprintf("%s drop v%d", s.label, ev.version)})
		k.miss(i)
		return
	}

	// Buffer-full drop — the real Broadcaster's non-blocking skip.
	if s.buf >= k.cfg.Buffer {
		k.emit(Action{Kind: ActDrop, Surface: i, Version: ev.version,
			Label: fmt.Sprintf("%s buffer-full drop v%d", s.label, ev.version)})
		k.miss(i)
		return
	}
	s.buf++

	at := k.step
	if f.DelayRate > 0 && k.chance(f.DelayRate) {
		at += 1 + k.rng.IntN(f.DelayMax)
	}
	k.schedule(delivery{at: at, surface: i, ev: ev})
	if f.DuplicateRate > 0 && k.chance(f.DuplicateRate) {
		k.schedule(delivery{at: at, surface: i, ev: ev})
		k.emit(Action{Kind: ActDuplicate, Surface: i, Version: ev.version,
			Label: fmt.Sprintf("%s duplicate v%d", s.label, ev.version)})
	}
}

// miss records that a surface lost an event and, under Resync, schedules its
// reconnect.
func (k *Kernel) miss(i int) {
	k.surf[i].gap = true
	if k.cfg.Resync {
		k.schedule(delivery{at: k.step + 1 + k.cfg.Faults.ReconnectAfter, surface: i, reconnect: true})
	}
}

// drainDue processes every delivery due at the current step, in deterministic
// (seq) order.
func (k *Kernel) drainDue() {
	for {
		idx := -1
		for j, d := range k.queue {
			if d.at <= k.step {
				if idx == -1 || d.seq < k.queue[idx].seq {
					idx = j
				}
			}
		}
		if idx == -1 {
			return
		}
		d := k.queue[idx]
		k.queue = append(k.queue[:idx], k.queue[idx+1:]...)
		if d.reconnect {
			k.reconnect(d.surface)
		} else {
			k.deliver(d.surface, d.ev)
		}
	}
}

// deliver applies an event to a surface per the active semantics, enforcing
// version ordering.
func (k *Kernel) deliver(i int, ev event) {
	s := k.surf[i]
	if s.buf > 0 {
		s.buf--
	}
	if !s.connected {
		return
	}

	if ev.isSnap {
		// Snapshot: idempotent last-write-wins. Older snapshots are ignored, so
		// reorders and duplicates can't regress the surface.
		if ev.version > s.version {
			s.data = cloneMap(ev.snapshot)
			s.version = ev.version
			s.gap = false
			s.history = append(s.history, s.version)
			k.emit(Action{Kind: ActDeliver, Surface: i, Version: ev.version,
				Label: fmt.Sprintf("%s apply snapshot v%d", s.label, ev.version)})
		}
		return
	}

	// Delta: strict in-order application.
	switch {
	case ev.version <= s.version:
		// stale or duplicate — ignore.
	case ev.version == s.version+1 && !s.gap:
		s.data[ev.key] = ev.val
		s.version = ev.version
		s.history = append(s.history, s.version)
		k.emit(Action{Kind: ActDeliver, Surface: i, Version: ev.version,
			Label: fmt.Sprintf("%s apply delta v%d", s.label, ev.version)})
	default:
		// Gap: a delta is missing. The surface cannot safely apply ahead; it
		// must resync. Recovery is the scheduled reconnect.
		s.gap = true
		k.emit(Action{Kind: ActGap, Surface: i, Version: ev.version,
			Label: fmt.Sprintf("%s gap at v%d (have v%d)", s.label, ev.version, s.version)})
		k.miss(i)
	}
}

// hydrate connects a surface and loads the full authoritative snapshot.
func (k *Kernel) hydrate(i int) {
	s := k.surf[i]
	s.connected = true
	s.buf = 0
	s.data = cloneMap(k.srvData)
	s.version = k.srvVer
	s.gap = false
	s.history = append(s.history, s.version)
}

func (k *Kernel) reconnect(i int) {
	k.hydrate(i)
	k.emit(Action{Kind: ActReconnect, Surface: i, Version: k.srvVer,
		Label: fmt.Sprintf("%s reconnect+hydrate v%d", k.surf[i].label, k.srvVer)})
}

func (k *Kernel) schedule(d delivery) {
	d.seq = k.seq
	k.seq++
	k.queue = append(k.queue, d)
}

func (k *Kernel) chance(p float64) bool { return k.rng.Float64() < p }

// emit records an action: it appends a deterministic log line (used by the
// determinism test) and, when a trace is being recorded, hands a full frame to
// the observer.
func (k *Kernel) emit(a Action) {
	k.log = append(k.log, fmt.Sprintf("step %d: %s", k.step, a.Label))
	if k.hook != nil {
		k.hook(a)
	}
}

// finish runs the invariants and assembles the result.
func (k *Kernel) finish() Result {
	auth := Authoritative{Data: cloneMap(k.srvData), Version: k.srvVer}
	views := make([]View, len(k.surf))
	for i, s := range k.surf {
		views[i] = View{Label: s.label, Data: cloneMap(s.data), Version: s.version}
	}

	res := Result{Seed: k.cfg.Seed, Steps: k.step, Auth: auth, Views: views, Log: k.log}

	// Ordering: each surface's applied-version history must be non-decreasing.
	for _, s := range k.surf {
		if err := CheckMonotonic(s.label, s.history); err != nil {
			res.Violations = append(res.Violations, k.violation("monotonic-version", err))
		}
	}
	// Convergence: every surface presents the authoritative state.
	if err := CheckConvergence(auth, views); err != nil {
		res.Violations = append(res.Violations, k.violation("convergence", err))
	}
	return res
}

func (k *Kernel) violation(name string, err error) Violation {
	return Violation{
		Seed:      k.cfg.Seed,
		Step:      k.step,
		Invariant: name,
		Detail:    err.Error(),
		Events:    tail(k.log, 12),
	}
}

func cloneMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func tail(s []string, n int) []string {
	if len(s) <= n {
		return append([]string(nil), s...)
	}
	return append([]string(nil), s[len(s)-n:]...)
}
