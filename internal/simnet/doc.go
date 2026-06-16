// Package simnet is a self-contained, dependency-free deterministic simulation
// harness for the gohtmxelm pattern: Go owns authoritative state, a Broadcaster
// fans changes out over SSE, and the browser broker relays them to HTMX,
// Datastar, and Elm surfaces that must all converge to the same state.
//
// It is PADST in spirit — a single-threaded, seed-reproducible kernel that
// routes explicit messages and checks invariants after every step — but
// scoped to this one library and carrying none of the protocol adapters or
// external dependencies of a general distributed-systems simulator. Its only
// job is to answer one question: under an adversarial network, do all surfaces
// still converge?
//
// # What it models
//
// The kernel mirrors the real [gohtmxelm.Broadcaster] contract exactly:
//
//   - Per-surface delivery is buffered and LOSSY. When a surface's buffer is
//     full the event is dropped, never blocking the publisher — just like
//     Broadcaster.Publish's "select { case ch <- v: default: }".
//   - Convergence is therefore NOT guaranteed by delivery. It is guaranteed by
//     resync: a surface that misses an event reconnects and re-hydrates from
//     the authoritative snapshot, exactly as Serve does (Subscribe, then
//     hydrate). With [Config.Resync] off, the harness demonstrates the
//     divergence that results — proof that the pattern depends on reconnect.
//
// Events come in two flavours, selected by [Config.Semantics]:
//
//   - Snapshot: each event carries the full current state. Idempotent and
//     last-write-wins; tolerant of drops and reorders between resyncs.
//   - Delta: each event carries one changed key and must be applied strictly
//     in version order. A dropped delta opens a gap that only a resync closes.
//
// # Reproducing failures
//
// Every run is fully determined by [Config.Seed]. A [Violation] records the
// seed and step, so a failing scenario reproduces identically — see
// Violation.Reproduce for the one-liner.
//
// # Relationship to the real-code tests
//
// simnet validates the design of the contract. The package's exported
// invariants ([CheckConvergence], [CheckMonotonic]) are the same ones the
// real-code tests assert against the shipped Broadcaster/Stream/Serve using
// testing/synctest and httptest, so model and implementation are held to one
// bar.
package simnet
