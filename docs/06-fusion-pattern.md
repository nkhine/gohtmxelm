# 6. The fusion pattern: three front-ends, one truth

> Now we assemble the pieces. The interesting claim of this whole project is that
> HTMX, Datastar, and Elm can share a single page **without conflict** — not by
> compromise, but because each owns a clearly bounded job and they communicate
> only through **shared server state** and the **browser event system**, never by
> mutating each other's DOM.

---

## 6.1 The one rule that makes it work

> **No library may patch the DOM that another library owns.**

That's it. Everything else follows. HTMX swaps its fragments; Datastar morphs its
regions; Elm renders its islands. The boundaries are physical — different DOM
subtrees — and they are never crossed. When two reactivity systems write to the
same nodes, you get the classic disaster: each clobbers the other's changes, you
get flicker, lost input, and bugs no one can reproduce. The reference demo avoids the whole
category by **partitioning the DOM** and forbidding overlap.

So how do the surfaces ever agree on anything? They don't talk to each other at
all. They each talk to the **server**, and the server tells everyone the new
truth over SSE. Convergence is a *consequence* of shared truth, not a negotiation
between front-ends.

---

## 6.2 Ownership map

| Surface | Owner | Mechanism |
|---|---|---|
| Server-rendered store table (+ delete buttons) | **HTMX** | `hx-get`/`hx-delete`, fragment swaps |
| Live-patched store region + write counter | **Datastar** | `datastar-patch-elements` / `-signals` over SSE |
| Stopwatch control buttons | **HTMX** | `hx-post`, self-refresh via `hx-trigger` |
| Stopwatch live readout + lap list | **Datastar** | id-addressed patches over SSE |
| Typed draft editor, event log, lap analytics | **Elm** | islands, ports, decoders |
| The KV store, the stopwatch, all SSE endpoints | **Go** | the single source of truth |

Each row is a different DOM region and a different tool. No region appears twice.

---

## 6.3 The convergence loop, end to end

Trace a single write — say, a user typing into the Elm draft editor and clicking
Save. This one path exercises every layer in the project:

```
1. Elm (App A)  classifyDraft → Valid "hello"
                update returns sendStateSet over the brokerOut PORT
                                  │
2. broker.js    receives the port message, stamps the source as "app-a",
                POSTs JSON to /api/store with the known version
                                  │
3. Go           SetIf("message", "hello", "app-a", version)
                → store mutates, seq++, fan-out on every subscriber channel
                                  │
        ┌─────────────────────────┴───────────────────────────┐
        ▼                          ▼                            ▼
4a. /api/events           4b. /api/datastar/...        (all from ONE store change)
    JSON store-change          datastar-patch-elements
        │                          + datastar-patch-signals
        ▼                          ▼
   broker.js applies          Datastar morphs its region,
   it, fires HTMX             updates $writes / $lastWriter
   "store-refresh",
   broadcasts STORE_CHANGE
   to Elm islands
        │
        ▼
5. HTMX re-fetches /api/store/fragment → fresh server-rendered table
   Elm App B appends the change to its typed event log
   Elm App A shows "last write by: app-a"
```

Every surface now shows `message = "hello"`, attributed to `app-a` — the HTML
table, the Datastar region, the Elm islands — and they got there **independently
from the same broadcast**, not by copying from one another. The write originated
in Elm, traveled out through a port, became authoritative in Go, and came back to
*all three* front-ends through the stream. That round trip is the entire thesis of
the architecture in one click.

---

## 6.4 The broker: a generic typed message bus

The broker is split in two so the reusable transport is never entangled with
one app's policy:

- [`pkg/runtime/gohtmxelm-broker.js`](../pkg/runtime/gohtmxelm-broker.js) — the
  **generic** broker shipped by the package. It knows envelopes, routing,
  shared state, island mounting, and how to bridge *any* SSE source. It contains
  no store endpoints, no optimistic locking, no activity log. It exposes its
  activity as `gohtmxelm:*` DOM events.
- [`demo/static/demo-ui.js`](../demo/static/demo-ui.js) — the **demo's** glue. It
  listens to those DOM events and adds the app-specific behaviour: mirroring Elm
  writes to the Go store with optimistic versioning, driving HTMX refreshes, and
  rendering the teaching activity log.

The generic broker does a handful of well-defined jobs, and its design is worth
studying as a small systems exercise:

- **Mounts/unmounts Elm islands** by scanning for `data-elm-module` and calling
  `Elm.<Module>.init`. A `MutationObserver` unmounts islands when their DOM is
  removed, preventing leaks.
- **Speaks one versioned envelope** for every message:
  `{ version, type, source, target, payload }`. The `version` field is checked on
  every message — an explicit, forward-compatible contract rather than ad-hoc
  objects. A malformed or wrong-version message is rejected, not guessed at.
- **Routes** by `target`: a specific island, `broadcast` (everyone), `others`
  (everyone but the sender), or `broker` (handled internally). The sender never
  names itself — the broker **stamps `source`**, so an island can't lie about who
  it is.
- **Bridges any SSE source to islands**: it opens every `EventSource` listed in
  `data-sources`, forwards each named event to islands as a generic `SSE_EVENT`
  envelope `{ event, data }`, and **caches the last value per event name**,
  replaying it to islands that mount late so a fresh analyzer is immediately
  correct.
- **Emits `gohtmxelm:*` DOM events** for everything it does (mounted, sse,
  state-set, source-open/error, htmx-swap), so host pages layer their own policy
  without forking it.

The **demo's** glue (`demo-ui.js`) is what mirrors Elm writes to Go with
optimistic versioning (translating an HTTP 409 into a no-op — see
[doc 2](./02-go-backend.md)), applies `seq` stale-dropping, and drives the HTMX
refreshes. Keeping that in the host means the broker stays a pure, reusable
translator between "Elm port message," "SSE event," and "DOM event." Each
front-end stays ignorant of the others; the broker plus its host glue is the
single translator.

---

## 6.5 Why this is a genuinely strong pattern

It's easy to dismiss "three frameworks on a page" as a stunt. It isn't — the
underlying architecture is one that scales and ages well, for reasons that apply
far beyond this toy:

1. **Single source of truth.** State lives in exactly one place (the Go store).
   There is no client/server state-synchronization problem because the client
   holds **no authoritative state** — only a cache that the server refreshes. The
   single hardest problem in front-end engineering ("keep these two copies of the
   truth in sync") is *defined out of existence*.

2. **Boundaries are physical and enforced.** Each tool owns disjoint DOM. You can
   reason about, test, and replace any one surface without touching the others.
   The store table could be rewritten in raw JS tomorrow and nothing else would
   notice, because it communicates only through the store and the stream.

3. **Right tool, right job.** Request/response → HTMX. Live push + local signals →
   Datastar. Complex typed client logic → Elm. None is stretched past what it's
   good at, so none accumulates the workarounds that come from misuse.

4. **The transport is uniform.** *Everything* converges through one mechanism: a
   write to Go, then an SSE broadcast. Adding a fourth surface tomorrow means
   subscribing to the same stream — not inventing a new sync path. The system
   grows by addition, not by multiplying integration points.

5. **Failure is recoverable by design.** Drops, conflicts, and stale data are
   handled by hydrate-on-connect and `seq` ordering, not by hoping the network is
   perfect. Reconnection is just connection.

The generalizable principle: **let the server own the truth, give each part of
the UI a bounded job, and make all of them converge through one broadcast
channel.** You don't need three frameworks to use this pattern — you need *one
source of truth and clear ownership*. The three frameworks are just a vivid proof
that the pattern is robust enough to absorb radically different UI philosophies at
once.

---

## 6.6 When NOT to reach for this

Honesty matters more than advocacy. This architecture is a poor fit when:

- **The UI is genuinely offline-first or local-first.** If the client must be the
  source of truth (an offline notes app, a collaborative editor with CRDTs), a
  server-authoritative model fights you.
- **Latency to the server dominates the interaction.** A drawing canvas or a game
  needs sub-frame local response; a server round-trip per change is wrong. (Note:
  the reference demo already pushes such cases to Elm's *local* state — the boundary is the
  same one in miniature.)
- **You have no server, or a purely static deploy.** This pattern assumes a live
  server holding state and connections.

The skill is recognizing which parts of *your* app are server-authoritative
request/response (most CRUD), which are server-pushed live views (dashboards,
readouts), and which are genuinely client-stateful (complex editors) — and then
applying HTMX / Datastar / Elm-style boundaries accordingly. The lesson isn't
"use these three libraries." It's "**find the boundaries, assign owners, converge
through one truth.**"

---

## 6.7 What to take away

- The enabling rule: **no library patches another's DOM.** Boundaries are physical
  DOM subtrees.
- Front-ends don't coordinate with each other; they each talk to the **server**,
  which broadcasts the new truth. Convergence is a side effect of shared truth.
- The **broker** is the single translator between Elm ports, Go HTTP, and SSE —
  one place that knows the wiring, keeping every surface ignorant of the others.
- The client holds **no authoritative state**, which eliminates the
  client/server sync problem entirely.
- The pattern generalizes far past this demo: **one source of truth, bounded
  ownership, one convergence channel.** The three frameworks are proof of its
  robustness, not the point of it.

"Convergence is a side effect of shared truth" is a claim, not a hope — and it
is testable. [Chapter 9](./09-testing-the-contract.md) shows how convergence is
verified deterministically under a hostile network, and why it depends on
reconnect-and-rehydrate rather than on the (deliberately lossy) stream.

Next: [the contrast case — React and the cost of putting the truth in the browser →](./07-react-and-the-spa-tax.md)
