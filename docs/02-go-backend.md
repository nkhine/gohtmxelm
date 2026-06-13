# 2. Go: the backend and the single source of truth

> The front-end frameworks get the attention, but the most important design
> decision in the reference demo is on the server: **the server owns all the truth, and it
> can hold thousands of streaming connections open cheaply.** Go makes both of
> those natural rather than heroic.

This document explains why Go fits streaming servers so well, then walks the
three pieces of state machinery: the **key-value store** with pub/sub, the
**stopwatch** with its gated tick loop, and the **graceful shutdown** that
connects back to the chunked-encoding lesson from doc 1.

---

## 2.1 Why Go for this

A server that holds many long-lived SSE connections has an unusual shape: most of
its goroutines spend most of their time *blocked, waiting* — for a store change,
for a tick, for the client to disconnect. Two language features make this cheap
and readable in Go:

### Goroutines are not OS threads

Each SSE connection is handled by its own goroutine. A goroutine starts at ~2 KB
of stack and is multiplexed onto a small pool of OS threads by the Go runtime.
You can have tens of thousands of them blocked on a channel receive at almost no
cost. The same workload modelled with one OS thread per connection would exhaust
memory; modelled with an event loop and callbacks (the Node.js approach) it would
work but invert your control flow into a tangle of `.then()` and state machines.

Go lets you write the **blocking, sequential** version — which reads like the
problem — and the runtime makes it scale:

```go
for {
	select {
	case e := <-ch:          // block until the store changes...
		writeSSE(w, "store-change", e)
		flusher.Flush()
	case <-r.Context().Done(): // ...or the client goes away
		return
	}
}
```

That is the whole SSE handler. No callbacks, no reactor, no async colouring of
functions. It blocks, and that's fine.

### Channels are the synchronization primitive

A channel is a typed, concurrency-safe queue. The store uses channels to fan a
single write out to every connected client. The famous Go proverb applies:

> *Don't communicate by sharing memory; share memory by communicating.*

Instead of every SSE handler reaching into shared store state under a lock, the
store **sends** each change down a channel that the handler **receives** from.
The handler never touches shared state directly; it just drains its own channel.

---

## 2.2 The key-value store: one source of truth, with pub/sub

[`demo/internal/store/store.go`](../demo/internal/store/store.go) is the heart of the app.
It is a thread-safe map with three powers layered on top: **versioning**,
**attribution**, and **change notification**.

### The data model

```go
type Event struct {
	Key     string
	Value   string
	Source  string // who wrote it: htmx | datastar | app-a | app-b | go
	Deleted bool
	Version uint64 // per-key, increments on every write to that key
	Seq     uint64 // global, increments on every write to ANY key
}
```

Two counters, doing two different jobs — this distinction is worth internalizing:

- **`Version` (per key)** enables *optimistic concurrency control*. A client that
  read version 4 of `message` can say "write this, but only if it's still
  version 4." If someone else wrote in the meantime, the version won't match and
  the write is rejected. No locks held across the network; conflicts are detected
  rather than prevented.
- **`Seq` (global)** gives every change across the whole store a monotonic order.
  A client that has processed up to `seq=12` can safely **discard** any event it
  later receives with `seq <= 12` — those are stale redeliveries from a flaky
  reconnect. This is how the broker stays correct across dropped connections.

### Optimistic locking in one function

```go
func (s *Store) SetIf(key, value, source string, wantVersion uint64) (uint64, bool) {
	s.mu.Lock()
	cur := s.data[key]
	if wantVersion != 0 && cur.version != wantVersion {
		s.mu.Unlock()
		return cur.version, false // conflict — caller's view was stale
	}
	s.seq++
	seq := s.seq
	s.data[key] = entry{value: value, version: seq, source: source}
	subs := s.subscriberList()
	s.mu.Unlock()

	s.notify(subs, Event{Key: key, Value: value, Source: source, Version: seq, Seq: seq})
	return seq, true
}
```

Read the locking carefully, because it embodies a pattern you should copy:

1. **The lock is held only for the map mutation.** We grab the subscriber list
   while locked, then *release the lock before notifying.* Sending on channels
   while holding the lock would be a recipe for deadlock and latency spikes — a
   slow consumer would stall every other writer. Collect-then-release-then-notify
   is the safe shape.
2. **`wantVersion == 0` means "don't check"** — an escape hatch for unconditional
   writes (the HTMX form path uses it; the broker's optimistic path passes the
   real version).
3. The HTTP layer translates a `false` return into **HTTP 409 Conflict**, and the
   client's reaction is *do nothing* — because the SSE stream will deliver the
   winning value momentarily anyway. The conflict resolves itself.

### Fan-out without blocking the writer

```go
func (s *Store) Subscribe() chan Event {
	ch := make(chan Event, 16) // buffered
	// ... register ch ...
	return ch
}

func (s *Store) notify(subs []chan Event, e Event) {
	for _, ch := range subs {
		select {
		case ch <- e:          // deliver if there's room
		default:               // otherwise DROP for this subscriber
		}
	}
}
```

The `select { case ch <- e: default: }` is a **non-blocking send**. If a
subscriber's buffer is full (a slow or stalled client), we *drop the event for
that subscriber rather than block the writer*. This is the correct trade-off for
this system, and the reasoning is important:

- A writer must never be held hostage by the slowest reader. One stuck client
  cannot be allowed to freeze the whole server.
- Dropping is **safe here** because of the `seq`/`version` design: when the slow
  client reconnects, the SSE stream re-hydrates it with the full current state
  (see §2.3). It doesn't need every intermediate delta — it needs the latest
  truth, and it will get it.

This is "prefer liveness over completeness, and make completeness recoverable" —
a deep pattern in distributed systems. You don't guarantee every message is
delivered; you guarantee the client can always **resync to the truth**.

---

## 2.3 Hydrate-then-stream: making reconnection trivial

Each SSE endpoint follows the same two-phase shape. On connect it sends the
**entire current state**, then it streams **deltas**. From the events handler:

```go
// Phase 1: hydrate — full snapshot, applied unconditionally by the client
for _, state := range kv.AllStates() {
	writeSSE(w, "store-hydrate", state)
}
flusher.Flush()

// Phase 2: stream — deltas, which the client may discard if stale by seq
for {
	select {
	case e := <-ch:
		writeSSE(w, "store-change", e)
		flusher.Flush()
	case <-r.Context().Done():
		return
	}
}
```

The two distinct event names (`store-hydrate` vs `store-change`) let the client
treat them differently: hydration events are **always** applied (they *are* the
truth as of connect time); change events are checked against the last-seen `seq`
and dropped if old. Because every connection begins with a full hydrate, a client
that drops and reconnects automatically catches up — no special "give me what I
missed" logic, no server-side per-client buffers. **Reconnection is just
connection.**

This is why the drop-on-full strategy in §2.2 is safe: completeness is recovered
by the next hydrate, so the stream is allowed to be lossy under pressure.

---

## 2.4 The stopwatch: a server-owned clock that does no idle work

The stopwatch (in [`demo/main.go`](../demo/main.go)) is the app's other piece of live
state. It teaches a few more server patterns.

### Inject the clock; never call `time.Now()` directly in logic

```go
type Stopwatch struct {
	// ...
	now func() time.Time // injectable clock
}

func NewStopwatch() *Stopwatch {
	return &Stopwatch{ /* ... */ now: time.Now }
}
```

Every place that needs the time calls `s.now()` instead of `time.Now()`. In
production `now` *is* `time.Now`. In tests we substitute a clock we control:

```go
clock := &testClock{t: time.Unix(0, 0)}
sw := NewStopwatch()
sw.now = clock.now
sw.Start()
clock.advance(time.Second)
sw.Stop()           // elapsed is EXACTLY 1000ms — no sleeps, no flakiness
```

Time and randomness are the two classic sources of untestable, flaky code.
**Inject them** and they become ordinary inputs. The stopwatch's whole
accumulate-across-pause behaviour is verified deterministically because of this
one design choice.

### The gated tick loop: don't burn a timer when paused

A naive timer ticks forever. This one **parks itself** when the stopwatch isn't
running, and is woken by `Start()`:

```go
func (s *Stopwatch) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	ticker.Stop() // start parked
	running := false
	for {
		if running {
			select {
			case <-ticker.C:
				if _, ok := s.tick(); !ok { // tick reports "I'm paused now"
					ticker.Stop()
					running = false
				}
			case <-s.wake:           // already running; ignore
			case <-ctx.Done():
				return
			}
		} else {
			select {
			case <-s.wake:           // Start() poked us
				ticker.Reset(100 * time.Millisecond)
				running = true
			case <-ctx.Done():
				return
			}
		}
	}
}
```

Notice `tick()` returns `(snapshot, ok)`, and `ok == false` means "the stopwatch
was paused, stop ticking." The loop uses that to put itself back to sleep. An idle
stopwatch consumes **zero** wakeups. This is the difference between a polling
mindset ("check 10× a second forever") and an event mindset ("do work only when
there is work").

### Two streams from one source: HTML for Datastar, JSON for everyone else

The stopwatch broadcasts a `StopwatchEvent{Snapshot, StateChange}` where
`StateChange` distinguishes a real user action (start/stop/lap/reset) from a
periodic tick. Two endpoints subscribe and filter differently:

- `/api/stopwatch/stream` — **HTML patches** for Datastar, on *every* event (the
  readout must update 10×/sec), but the lap list only on `StateChange` (laps
  don't change on a tick), keeping the per-tick payload small.
- `/api/stopwatch/events` — **JSON**, only on `StateChange`. Consumed by the
  broker to feed the Elm analyzer and to re-sync HTMX controls across tabs.

One source of truth, two projections, each shaped for its consumer. The 10×/sec
tick stream never touches the JSON consumers, so the Elm island and the control
sync aren't woken by noise they don't care about.

---

## 2.5 Graceful shutdown, revisited

Doc 1 explained the chunked-encoding bug. Here's the server-side resolution in
context, because it's a model for any streaming server:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

stopwatch := NewStopwatch()
go stopwatch.Run(ctx) // the tick loop dies with the signal context too

server := &http.Server{
	// ...
	BaseContext: func(net.Listener) context.Context { return ctx },
}

// ... serve ...

<-ctx.Done()        // a signal arrived
server.Shutdown(shutdownCtx) // handlers' r.Context() are children of ctx,
                             // so they're already unblocking and returning
```

The single `ctx` ties together: the stopwatch goroutine, every request context,
and the shutdown trigger. One cancellation drains the entire system cleanly. SSE
handlers return → Go writes the chunked terminators → connections close cleanly →
browsers reconnect silently. (See [doc 1, §1.4](./01-networking-foundations.md)
for the byte-level "why.")

---

## 2.6 What to take away

- Go's **goroutine-per-connection** model lets you write blocking, sequential
  handlers that still scale to many idle-but-open streams.
- **Channels** turn "fan one change out to N listeners" into a few lines, and
  let you keep shared state behind a single owner.
- **Hold locks only over memory mutations**, never across channel sends or I/O.
- **Optimistic concurrency** (per-key versions) detects conflicts without holding
  locks across the network; **global sequence numbers** let clients discard stale
  data and recover by re-hydrating.
- **Drop-on-full + hydrate-on-connect** is a robust pairing: lossy under pressure,
  always recoverable.
- **Inject time and randomness** to make logic deterministic and testable.
- **Tie everything to one lifecycle context** so shutdown is clean — which, as we
  saw, has effects all the way down at the TCP/chunked layer.

Next: [HTMX and the idea that HTML itself can be the state machine →](./03-htmx-hypermedia.md)
