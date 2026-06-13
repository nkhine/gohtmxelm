# Go + HTMX + Datastar + Elm

This is a small teaching app for combining three frontend styles around one
end-user workflow: update the shared `message` key and watch every surface
converge. Every write is **attributed** — the Go store records which surface
wrote (`htmx`, `datastar`, `app-a`, `app-b`, `go`) and every pane renders that
same truth its own way.

## Learn from it

The [`docs/`](./docs/README.md) directory is a guided, read-in-order tour that
uses this codebase to teach the stack from the bottom up — TCP and chunked
encoding, Server-Sent Events, Go's streaming backend, the philosophy of HTMX,
Datastar, and Elm, the fusion pattern that lets them coexist, and an honest look
at what large client-side frameworks cost. Start at
[`docs/README.md`](./docs/README.md).

## Run it

```sh
make dev
```

For a live rebuild/restart loop while editing Go, templ, Elm, or `broker.js`:

```sh
make watch
```

`make watch` uses [Air](https://github.com/air-verse/air). Install it if needed:

```sh
go install github.com/air-verse/air@latest
```

## Example Lattice

The examples are reusable templ components under `examples/`.

Routes:

```text
/                   composed lattice with all examples
/examples/message   shared message workbench only
/examples/stopwatch stopwatch example only
```

Both the index and individual routes render the same components, so adding a
new example means adding one component and one registry entry in `main.go`.

The Makefile copies Datastar from:

```sh
/Users/nkhine/go/src/github.com/starfederation/datastar/bundles/datastar.js
```

Override that path if needed:

```sh
make dev DATASTAR_SRC=/path/to/datastar/bundles/datastar.js
```

## Ownership Rule

The page is one fused workbench, but each library still owns a clear boundary:

| Library | Owns | Strength on display |
| --- | --- | --- |
| HTMX | The server-rendered store table | hypermedia controls: each row carries a server-rendered `hx-delete` button; state transitions live in HTML, not JS |
| Datastar | The live store patch region and signal-driven form | `datastar-patch-elements` re-renders its island and `datastar-patch-signals` drives the live write counter / last-writer chips with zero client JS |
| Elm | The Elm island roots | App A is a typed draft editor — a `Draft = Empty \| TooLong \| Valid` state machine makes invalid writes unrepresentable; App B folds the SSE event stream into a typed, bounded history log |
| Go | The shared KV store and SSE endpoints | single source of truth: versions, optimistic locking, write attribution, and fan-out to every surface |

The important rule is: do not let HTMX or Datastar patch inside an Elm root, and
do not let Elm render inside a Datastar-owned region. They fuse through Go state
and browser events.

## Teaching Flows

1. HTMX writes `message` with a normal form post to `POST /api/store` (source defaults to `htmx`).
2. HTMX deletes any key through a server-rendered `hx-delete` row control — hypermedia as the engine of application state.
3. Datastar writes `message` with `data-bind`, `data-text`, and declarative `@post`; the server stamps the write as `datastar`.
4. Elm App A validates a typed draft (empty / too long / valid) and only the `Valid` branch can emit a port write; broker.js attributes it to the island id.
5. Go stores the winning value with its source and broadcasts it over SSE.
6. HTMX refreshes the server-rendered table, now showing a "By" attribution chip per key.
7. Datastar receives `datastar-patch-elements` for its DOM region **and** `datastar-patch-signals` updating `$writes` / `$lastWriter`.
8. Elm receives the same change through the broker EventSource: App A shows "last write by", App B appends it to a typed event log.
9. Elm can also ask the broker to perform an HTMX fragment swap, showing a cross-library command path.

## Why use all three?

Use HTMX where server-rendered HTML is the simplest contract. Use Datastar where
the server should push small live DOM patches or signal updates and local
`data-*` signals are enough. Use Elm where the UI has a real client-side state
machine that benefits from types, ports, and explicit update logic. Fuse them
through shared server state, not by letting multiple libraries mutate the same
DOM subtree — and attribute every write so each pane can prove the convergence
to the user.

## Hello Stopwatch

The stopwatch card is a compact four-way fusion around one server-owned timer:

1. **Go** owns elapsed time and lap history. A single goroutine ticks only while
   the timer runs (it parks itself when paused) and fans out on two streams.
2. **HTMX** owns the start/stop/lap/reset buttons. Each action `hx-post`s and
   swaps the controls fragment. Crucially, the controls are also a self-refreshing
   element (`hx-trigger="stopwatch-state-change from:body"`): when any tab acts,
   an SSE state event re-triggers the fragment in every *other* tab, so the
   controls never go stale across clients.
3. **Datastar** consumes `/api/stopwatch/stream` and patches the live readout
   every tick, but the lap list only on state changes — the 10/sec stream stays
   small because the readout (`#stopwatch-readout`) and laps (`#stopwatch-laps`)
   are separately targetable.
4. **Elm** (`LapStats`) subscribes to the same stopwatch state (relayed by
   `broker.js` from the JSON `/api/stopwatch/events` stream) and derives typed
   lap analytics — fastest / slowest / average / last split — as a state machine
   that models the no-laps case explicitly. No other layer computes these.

The two streams are deliberate: `/api/stopwatch/stream` carries HTML patches for
Datastar (every tick), while `/api/stopwatch/events` carries JSON and fires only
on discrete state changes, which is all the Elm analyzer and the cross-tab
control sync need.

## CSP Note

Datastar v1.0.2 compiles declarative expressions such as `data-signals`,
`data-text`, and `@post(...)` in the browser. For this teaching demo the CSP
therefore includes `script-src 'self' 'unsafe-eval'`. Without that, browsers
block Datastar expression evaluation with a `GenerateExpression` error.
