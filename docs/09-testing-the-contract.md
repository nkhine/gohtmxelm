# 9. Testing the contract: convergence under an adversarial network

> The fusion pattern makes one load-bearing promise: **convergence**. After
> writes stop and the network settles, every surface — HTMX table, Datastar
> region, Elm island — presents the same state Go owns. Chapter 6 argued *why*
> that should be true. This chapter shows how to *verify* it, why it is harder
> than it looks, and how the library tests both the design and the shipped code.

---

## 9.1 The contract, stated precisely

Two properties, checked together:

- **Convergence** — once the system quiesces, every connected surface presents
  exactly the authoritative state (same data, same version).
- **Ordering (monotonicity)** — a surface never applies an older version over a
  newer one; its view of the version only moves forward.

These are the two invariants the whole test strategy is built around, and they
live in one place — `simnet/invariants.go` — as pure functions:

```go
simnet.CheckConvergence(auth, views) // every view == authoritative, or error
simnet.CheckMonotonic(label, history) // applied versions never regress
```

Everything below asserts *these same functions*, so the model and the
implementation are held to an identical bar.

---

## 9.2 The catch: delivery is lossy by design

It is tempting to assume convergence falls out of "the server sends every change
to everyone." It does not, because `Broadcaster` is deliberately **lossy**:

```go
// Publish delivers v to every current subscriber, skipping any whose buffer is
// full. It never blocks.
func (b *Broadcaster[T]) Publish(v T) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- v:
		default: // buffer full — drop, do not block the publisher
		}
	}
}
```

A slow or briefly-disconnected subscriber simply *misses* events. So the stream
on its own cannot guarantee convergence. What rescues it is **resync**:
`Serve` subscribes and then **hydrates** the full current snapshot, and the
browser reconnects automatically when its `EventSource` drops:

```go
func Serve[T any](s *Stream, b *Broadcaster[T], hydrate ..., each ...) error {
	ch := b.Subscribe()        // 1. subscribe first
	defer b.Unsubscribe(ch)
	if hydrate != nil {
		hydrate(s)             // 2. then send the full current snapshot
	}
	for { /* 3. forward each subsequent change */ }
}
```

So the real guarantee is:

> Convergence holds **iff** events are idempotent enough to survive loss
> (snapshot semantics), **or** a missed event is recovered by reconnecting and
> re-hydrating. A purely lossy stream with neither will diverge.

That single sentence is the thing worth testing — and the thing the simulator
makes visible.

---

## 9.3 Two layers, one set of invariants

| Layer | What it exercises | Tool |
|-------|-------------------|------|
| `simnet` | a deterministic *model* of the contract under faults | a custom single-threaded kernel |
| `pkg/*_test.go` | the *shipped* `Broadcaster` / `Stream` / `Serve` code | `testing/synctest` + `httptest` + `-race` |

The model proves the *design* is sound under adversarial conditions; the
real-code tests prove the *implementation* matches. Neither replaces the other —
the model cannot exercise real goroutines, and the goroutine tests cannot sweep
hundreds of fault schedules cheaply.

---

## 9.4 `simnet` — deterministic simulation

`simnet` is a small harness in the spirit of **PADST** (protocol-aware
deterministic simulation testing): a single-threaded, seed-reproducible kernel
that routes explicit messages and checks invariants after every step. It carries
no external dependencies and is scoped to this one pattern.

A run is fully described by a `Config`:

```go
res := simnet.Config{
	Seed:      42,
	Surfaces:  5,
	Buffer:    6,             // per-surface SSE buffer (the lossy point)
	Writes:    40,
	Keyspace:  5,
	Semantics: simnet.Delta,  // or simnet.Snapshot
	Faults:    simnet.Chaos(),// drop / delay / duplicate / reorder / partition
	Resync:    true,          // reconnect-and-rehydrate recovery
}.Run()

if !res.OK() {
	// res.Violations carries the failing invariant, the step, and the Seed.
}
```

The kernel mirrors the real semantics precisely: per-surface delivery is buffered
and dropped when full; a surface that misses an event (drop, full buffer, gap, or
partition) only recovers if `Resync` is on; `Snapshot` events are idempotent
last-write-wins while `Delta` events must apply strictly in order.

The headline tests (`simnet/sim_test.go`):

- **converges under chaos** — with `Resync: true`, every surface converges
  across 200 seeds, for both snapshot and delta semantics;
- **diverges without resync** — with `Resync: false`, a violation *must* appear
  (and the test fails if it cannot find one, which would mean the fault model
  went toothless);
- **determinism** — the same `Seed` yields an identical event log, which is what
  makes `Violation.Seed` a reliable reproduction handle.

`Config.Record()` additionally captures the run frame-by-frame (`Trace.Frames`)
so it can be replayed — that is what the simulator card consumes.

---

## 9.5 Real-code tests — `synctest`, `httptest`, `-race`

The model is single-threaded by construction, so it cannot catch a data race or
a channel-handling bug in the actual code. Go 1.25's `testing/synctest` fills the
gap: it runs the *real* goroutines, channels, and timers in a controlled bubble,
deterministically.

- `broadcaster_synctest_test.go` drives the real `Broadcaster`: fan-out
  ordering, the non-blocking drop-on-full behaviour, unsubscribe-closes, and a
  concurrent subscribe/publish/unsubscribe hammer (whose value is under `-race`).
- `serve_contract_test.go` drives `Stream` / `Serve`: the
  subscribe→hydrate→fan-out lifecycle, an end-to-end check over a real
  `httptest.Server` socket, and a reconnect-rehydrate convergence test that
  asserts the same `simnet.CheckConvergence` the model uses.

```sh
go test ./... -race          # both layers, race detector on
go test ./simnet/        # the contract model only
```

---

## 9.6 The simulator: watching the contract hold (or break)

The **Contract simulator** card (`/examples/simulator`) is the harness made
visible — and it dogfoods the library to do it. `demo/internal/simviz` records a
`simnet` run and replays it frame-by-frame over the library's own `Broadcaster`;
an Elm island renders a radial network while Datastar renders the verdict. The
frames travel the very path they depict.

It draws each transport's **real path**, because they differ:

- **Elm** rides the broker SSE → `bridge.js` → island `brokerIn` port (a push).
- **HTMX** rides the *same* broker stream, but the event only *triggers* an
  `hx-get` that re-pulls a fragment (`htmx.trigger → GET → swap`).
- **Datastar** runs its **own** SSE stream and patches the DOM directly — it
  bypasses `bridge.js` entirely (drawn as its own spoke).

A per-frame pipeline strip lights the function in play — a drop at the
`Broadcaster`, a duplicate on the SSE wire, a partition/reconnect at `bridge.js`,
a gap at the island port, a deliver at the surface.

What makes it a *tool* and not just an animation:

- **auto-loop + controls** — it runs new seeds continuously, or you drive it
  (play / pause / step / reseed, fault intensity, snapshot⇄delta, and **break
  it** to turn resync off and watch divergence);
- **a run ledger** — every completed run is recorded with deterministic metrics
  (drops, duplicates, partitions, reconnects, max lag, steps) and a
  plain-language explanation of any violation;
- **durable failures** — violations are appended to `sim-violations.jsonl`
  (override with `SIM_LOG=...`) and reloaded into the ledger on startup, so a
  failure is never lost to the next loop;
- **replay** — any ledger row reloads its *exact* seed and knobs, paused at frame
  zero, so a failure can be stepped through deterministically (and reproduced in
  a test via the same `simnet.Config{…}.Record()`).

---

## 9.7 What the simulator is **not**

It is a model of *correctness under faults*, not a performance or load test. The
`simnet` kernel does no real I/O — no sockets, no DOM, no goroutines — so its CPU
and memory are the *simulator's*, not production's. The metrics it keeps are
deliberately **deterministic** (faults weathered, max lag, steps), reproducible
from the seed. Wall-clock duration is not kept, because in the sim it is only the
playback tempo.

Production cost — server memory under many concurrent SSE clients, browser
render time — is a separate concern measured against the *running* app
(`net/http/pprof`, benchmarks on `Broadcaster`/`Serve`, the browser Performance
API), and should be kept distinct so the two are never conflated.

---

## 9.8 Summary

- The pattern's promise is **convergence + ordering**, expressed once as
  `simnet.CheckConvergence` / `simnet.CheckMonotonic`.
- Convergence is **not** a consequence of delivery — the `Broadcaster` is lossy
  on purpose. It depends on **reconnect-and-rehydrate** (and/or snapshot
  idempotence).
- `simnet` proves the *design* holds under drop/delay/duplicate/reorder/
  partition, seed-reproducibly; `synctest`/`httptest`/`-race` tests prove the
  *shipped code* holds, against the same invariants.
- The simulator card runs that harness live, visualises every transport's real
  path, and turns each failure into a durable, replayable, seed-stamped record.
