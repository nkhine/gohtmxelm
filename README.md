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
	AssetPath:   "/gohtmxelm",
	EventStream: "/api/events",
	EventNames:  []string{"store-change"},
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

Use the server helpers from handlers:

```go
gohtmxelm.PrepareSSE(w)
gohtmxelm.WriteSSE(w, "store-change", payload)
gohtmxelm.WriteDatastarPatchElements(w, html)
gohtmxelm.WriteDatastarPatchSignals(w, `{"count":1}`)
gohtmxelm.Trigger(w, "store-refresh")
```

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

demo/                   reference app users can copy from
demo/main.go            demo server and routes
demo/elm/               demo Elm source and elm.json
demo/static/            demo browser assets
demo/internal/store/    demo state store
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
- local Elm source under `demo/elm`
- demo-only browser assets under `demo/static`

Run it:

```sh
make dev
```

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
```

The Makefile copies Datastar from:

```sh
/Users/nkhine/go/src/github.com/starfederation/datastar/bundles/datastar.js
```

Override that path if needed:

```sh
make dev DATASTAR_SRC=/path/to/datastar/bundles/datastar.js
```

## CSP Note

Datastar v1.0.2 compiles declarative expressions such as `data-signals`,
`data-text`, and `@post(...)` in the browser. If you use those expressions,
your CSP needs to allow that evaluation path. The reference demo currently uses
`script-src 'self' 'unsafe-eval'` for this reason.
