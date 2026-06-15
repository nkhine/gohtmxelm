# gohtmxelm

`gohtmxelm` is a small Go-first integration library for combining HTMX,
Datastar, Elm islands, and Server-Sent Events in existing Go applications.

The package does not impose an application framework. It provides the reusable
bridge pieces: SSE response helpers, Datastar patch helpers, HTMX response
helpers, Elm island HTML conventions, and an embedded browser broker runtime.

## Install

```sh
go get github.com/nkhine/gohtmxelm/pkg
go install github.com/nkhine/gohtmxelm/cmd/gohtmxelm@latest
```

Check local tooling:

```sh
gohtmxelm doctor
```

Create a starter config:

```sh
gohtmxelm init
```

## Use In A Go App

Mount the embedded browser runtime:

```go
import gohtmxelm "github.com/nkhine/gohtmxelm/pkg"

kit := gohtmxelm.New(gohtmxelm.Options{
	AssetPath: "/gohtmxelm",
	Sources: []gohtmxelm.Source{
		{URL: "/api/events", Events: []string{"store-hydrate", "store-change"}},
	},
})

mux.Handle("/gohtmxelm/", http.StripPrefix("/gohtmxelm/", kit.Assets()))
```

Render the broker script on pages that mount Elm islands:

```go
template.HTML(kit.BrowserScript())
```

Render an Elm island mount point:

```go
html, err := gohtmxelm.ElmIsland("counter", "Counter", map[string]any{
	"initial": 0,
})
```

Pass locale metadata and a scoped message bundle into an island without making
the library own your translation system:

```go
locale := gohtmxelm.LocalePropsFrom("en-GB", "Europe/London", "GBP", translator,
	"common.save",
	"counter.title",
)
props, err := gohtmxelm.LocalizedProps(map[string]any{"initial": 0}, locale)
if err != nil {
	// base props must encode as a JSON object
}
html, err := gohtmxelm.ElmIsland("counter", "Counter", props)
```

`gohtmxelm` only defines the transport convention:

```json
{
  "locale": "en-GB",
  "timezone": "Europe/London",
  "currency": "GBP",
  "messages": {
    "common.save": "Save"
  }
}
```

The host application still owns locale resolution, supported locales, catalogue
loading, fallback rules, interpolation, pluralisation, persistence, and
date/money formatting.

Stream Server-Sent Events from a handler. A `Stream` bundles the
`ResponseWriter`, its flusher, and the request context, and flushes on every
write. Pair it with a `Broadcaster[T]` and `Serve` runs the whole
subscribe → hydrate → fan-out loop:

```go
func handler(w http.ResponseWriter, r *http.Request) {
	stream, err := gohtmxelm.NewStream(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gohtmxelm.Serve(stream, store.Events(),
		func(s *gohtmxelm.Stream) error { return s.Send("store-hydrate", store.Snapshot()) },
		func(s *gohtmxelm.Stream, e store.Event) error { return s.Send("store-change", e) },
	)
}
```

Each write method has a direct form too — `s.Send(event, data)`,
`s.PatchElements(html)`, `s.PatchSignals(data)`, `s.Ping()` — and the lower-level
`PrepareSSE` / `WriteSSE` / `WriteDatastarPatch*` functions remain available.

## Architecture

The intended ownership model is:

| Layer | Owns |
| --- | --- |
| Go | durable state, validation, commands, SSE fan-out |
| HTMX | server-rendered request/response fragments |
| Datastar | declarative local signals and server-pushed DOM/signal patches |
| Elm | typed client-side islands and local state machines |

Keep the DOM ownership boundaries physical:

- HTMX should not swap inside an Elm root.
- Datastar should not patch inside an Elm root.
- Elm should not render inside a Datastar-owned region.
- Go is the convergence point for shared state.

## Repository Layout

```text
cmd/gohtmxelm/          CLI for init and doctor workflows
pkg/                    public importable Go package
pkg/runtime/            embedded browser broker runtime
pkg/simnet/             deterministic simulation harness for the convergence contract
demo/internal/dynamo/   in-memory DynamoDB-style table (no external service)

demo/                   reference app users can copy from
demo/main.go            demo server and routes
demo/elm/               demo Elm source and elm.json
demo/static/            demo browser assets
demo/internal/store/    demo state store
demo/internal/simviz/   drives pkg/simnet live for the contract simulator card
demo/internal/ui/       demo templ shell/page composition
demo/internal/ui/components/
                        demo templ components

docs/                   architecture notes and deeper rationale
```

## Reference Demo

The `demo/` directory is a complete reference implementation that shows the
library pattern in a real Go app. It includes:

- a shared message workbench using HTMX, Datastar, Elm, Go, and SSE
- a server-owned stopwatch using HTMX controls, Datastar live patches, and Elm
  lap analytics
- a bank-statement view: an Elm range picker filters Go-owned transfers while
  HTMX renders the table and Datastar pushes the summary
- a **Seed** card that fakes account transfers (the
  [heritage](https://github.com/Shieldpay/heritage) treasury payment-row model,
  with [gofakeit](https://github.com/brianvoe/gofakeit) counterparty names) into
  the statement's **in-memory DynamoDB-style table** (`demo/internal/dynamo`, no
  Docker/AWS). Submitting the form writes the rows and broadcasts the change, so
  the statement's HTMX table, Datastar summary, and Elm picker all update — the
  statement data is generated at runtime, not hard-coded.
- a **Localization boundary** card with a demo-owned TOML-style catalogue and
  locale registry. Changing the language uses HTMX to re-render server copy,
  Datastar to bind localized date/money signals, and Elm island flags built via
  `gohtmxelm.LocalizedProps`. The example intentionally keeps catalogue and
  formatting policy in `demo/internal/localize`, not in the reusable package.
- an **Edge Datastar SSE** card that opens same-origin
  `/api/edge-datastar/stream`, loops a visual browser-to-edge-to-Lambda trace,
  and proves Datastar applies `datastar-patch-signals` and
  `datastar-patch-elements` frames and re-binds signal/click handlers inside a
  morphed element. The same handler is wrapped by a Go Lambda
  response-streaming entrypoint and a floci/Pulumi local API Gateway stack under
  `infra/`.
- a **Local SSO login** card that runs a complete local browser redirect flow:
  `/api/sso/start` sets state, a fixture identity-provider form issues a
  one-use code after approval, `/api/sso/callback` validates state and writes an HttpOnly
  session cookie, and HTMX rehydrates the signed-in claims panel. Successful
  login/logout also drives a demo-wide auth presence signal: every card header
  shows red logged-out, green online, or orange idle, while the SSO card renders
  the same state through HTMX, Datastar signal patches, and an Elm island.
- a **Contract simulator** that runs the `pkg/simnet` harness live: it replays a
  deterministic run over the library's own `Broadcaster` and visualises the full
  request path (Go → Broadcaster → SSE → `bridge.js` → Elm/HTMX/Datastar) under
  an adversarial network — dropping, delaying, and partitioning SSE while the
  convergence invariant holds (or, with recovery off, fails reproducibly). See
  [Testing the pattern](#testing-the-pattern).
- local Elm source under `demo/elm`
- demo-only browser assets under `demo/static`

Run it:

```sh
make dev
```

`make dev` and `make watch` only build local browser/server assets and run the
Go demo app. They do not start floci, Pulumi, API Gateway, or Lambda resources.
Use `make edge-infra-up` only when you want the local AWS edge stack.

By default the dev server runs over HTTP/2 with a self-signed localhost
certificate (`https://localhost:8091`). HTTP/2 multiplexes every SSE stream and
request over a single connection, which matters on the `/` gallery page: it
opens one SSE stream per card plus the broker stream, and over plain HTTP/1.1
that hits the browser's ~6-connections-per-host limit and starves htmx requests
(e.g. the SSO logout POST hangs). Your browser will warn once about the
self-signed cert — accept it for the session. To fall back to plain HTTP, pass
`TLS=0` (e.g. `make watch TLS=0`).

Run with rebuild/restart while editing Go, templ, Elm, or browser assets:

```sh
make watch
```

`make watch` uses [Air](https://github.com/air-verse/air):

```sh
go install github.com/air-verse/air@latest
```

Routes:

```text
/                   all demo examples
/examples/message   message workbench only
/examples/stopwatch stopwatch only
/examples/statement bank-statement view only
/examples/seed      seed transfers only
/examples/localization i18n/l10n boundary only
/examples/edge-datastar Datastar SSE through the edge only
/examples/sso-local local SSO redirect/session demo only
/examples/simulator contract simulator only
```

Run the local floci/API Gateway Lambda streaming stack:

```sh
make edge-infra-up
make edge-infra-output
```

See [Datastar over SSE through the edge](./docs/11-edge-sse.md) for the
Starbase `/api/*` origin wiring, SigV4 direct-call requirement, and Lambda
response streaming notes.

The local SSO simulator is built into `make dev`; it does not require floci or
Alcove. See [Local SSO login simulator](./docs/12-local-sso.md).

The Makefile copies Datastar from:

```sh
/Users/nkhine/go/src/github.com/starfederation/datastar/bundles/datastar.js
```

Override that path if needed:

```sh
make dev DATASTAR_SRC=/path/to/datastar/bundles/datastar.js
```

## Testing the pattern

The contract this library makes is **convergence**: after writes stop and the
network settles, every surface presents the same state Go owns. That guarantee
is non-trivial because `Broadcaster` is intentionally **lossy** — `Publish`
skips any subscriber whose buffer is full and never blocks, so a delivered
stream alone cannot keep surfaces in sync. Convergence depends on
**reconnect-and-rehydrate** (and/or idempotent snapshot events): a surface that
misses an event reconnects and `Serve` re-sends the current snapshot.

Two layers verify this, holding the model and the implementation to the **same
invariants** (`simnet.CheckConvergence` / `simnet.CheckMonotonic`):

- **`pkg/simnet`** — a self-contained, dependency-free deterministic simulation
  kernel (PADST in spirit: single-threaded, seed-reproducible, invariant-checked
  after every step). It models the broadcast→SSE→converge contract under a fault
  profile (drop / delay / duplicate / reorder / partition) for both snapshot and
  delta event semantics, and proves convergence holds with resync and diverges
  without it. A failing run is reproducible from its `Seed`.
- **Real-code tests** — `pkg/broadcaster_synctest_test.go` and
  `pkg/serve_contract_test.go` drive the shipped `Broadcaster` / `Stream` /
  `Serve` with Go's `testing/synctest` and `httptest`, asserting the same
  invariants against the actual goroutines and SSE wire.

```sh
go test ./... -race          # both layers
go test ./pkg/simnet/        # the contract model only
```

The **Contract simulator** card (`/examples/simulator`) runs the `simnet`
harness live and visualises it. It auto-loops new seeds, but you can drive it:
play/pause/step, change fault intensity, flip snapshot⇄delta, and **break it**
(turn resync off) to watch divergence. Each completed run is recorded in a
ledger with deterministic metrics (faults weathered, max lag); **violations are
persisted to `sim-violations.jsonl`** (override with `SIM_LOG=...`), reloaded on
startup, and **replayable** by seed straight from the ledger. See
[docs/09 — Testing the contract](./docs/09-testing-the-contract.md).

## CSP Note

Datastar v1.0.2 compiles declarative expressions such as `data-signals`,
`data-text`, and `@post(...)` in the browser. If you use those expressions,
your CSP needs to allow that evaluation path. The reference demo currently uses
`script-src 'self' 'unsafe-eval'` for this reason.
