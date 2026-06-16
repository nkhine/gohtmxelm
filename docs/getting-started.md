# Getting Started

This page covers installing the CLI, scaffolding a project, running it, and
deploying. For the *why* behind the integration model, read the chapters from
[Networking foundations](01-networking-foundations.md) onward.

## Install

```sh
go get github.com/nkhine/gohtmxelm                          # the library
go install github.com/nkhine/gohtmxelm/cmd/gohtmxelm@latest # the CLI
gohtmxelm doctor                                            # check the toolchain
```

The CLI needs `go`; the full (Elm) scaffold also needs the `elm` compiler.
`templ` is fetched as a Go tool, so you don't install it separately.

## Scaffold a project

`gohtmxelm init` generates a complete, runnable example: a server-owned counter
pushed to an Elm island over SSE. It installs dependencies and builds the assets,
leaving an app you can run immediately.

```sh
gohtmxelm init myapp          # full Elm-island app in ./myapp
cd myapp && make dev          # http://localhost:8080
```

Click **+1**: Go owns the count, broadcasts it over SSE, and every open tab
re-renders from the stream.

### Flavours

```sh
gohtmxelm init myapp --minimal   # SSE-only, no Elm and no build step (go run .)
gohtmxelm init myapp --deploy    # also emit deploy scaffolding (see below)
gohtmxelm init myapp --no-build  # write files only; skip go get / generate / build
```

### Generated layout (full)

```text
main.go             chi server: mounts the broker runtime, the SSE stream, /api/bump
page.templ          host shell (templ); injects the broker script + Elm island
elm/Counter.elm     the Elm island
elm/BrokerPort.elm  canonical wire contract (vendored from the library)
static/elm.js       compiled Elm bundle (built by `make elm`)
```

## Add to an existing Go project

Run `init` inside a directory that already has a `go.mod` and it adds a
self-contained, mountable `gohtmxelmkit/` package instead of a standalone app —
it never touches your existing `main.go`:

```sh
cd my-existing-app
gohtmxelm init
```

Then wire it into your chi router:

```go
import "your.module/gohtmxelmkit"

kit := gohtmxelmkit.New("/counter") // any prefix, or "" for root
kit.Mount(r)                        // r is your chi.Router
```

## Keep the Elm contract in sync

`BrokerPort.elm` is the Elm peer of the wire contract and stamps the same
`ProtocolVersion` as the Go `Envelope` and the broker runtime. After upgrading
the library, re-vendor it:

```sh
gohtmxelm vendor-elm        # rewrites elm/BrokerPort.elm
```

See [Integration contracts](13-contracts.md) for the full contract surface.

## Deploy

`--deploy` (at init time) or `gohtmxelm deploy` (on an existing scaffold) emit
**template-only** container + CI scaffolding — nothing is ever deployed for you
and no credentials are handled:

```sh
gohtmxelm init myapp --deploy   # scaffold + Dockerfile + CI
gohtmxelm deploy                # add it to an app you already scaffolded
```

This writes:

- a multi-stage **distroless, non-root `Dockerfile`** (Go-only for `--minimal`;
  templ + Elm bundle for the full scaffold),
- `.dockerignore` and `docker-compose.yml` for `docker compose up --build`,
- a **GitHub Actions** workflow that builds, tests, and pushes the image to
  **GitHub Container Registry** (`ghcr.io/<owner>/<repo>`) using the built-in
  `GITHUB_TOKEN` — no secrets to configure,
- a `DEPLOY.md` documenting the SSE-specific gotchas.

### SSE is the deployment constraint

The broker holds a long-lived SSE connection per tab. Whatever fronts the
container **must not buffer responses**:

- The server already sends `Cache-Control: no-cache` and `X-Accel-Buffering: no`
  and flushes after every event.
- For proxies that ignore that, disable response buffering explicitly (e.g.
  nginx `proxy_buffering off;`).
- Allow long idle timeouts; SSE streams stay open between events.
- Prefer HTTP/2 at the edge — it multiplexes many streams over one connection,
  avoiding the browser's ~6-connections-per-host limit.
- If your platform scales to zero, keep one instance warm or accept that idle
  clients reconnect (the broker re-hydrates on reconnect).
