# gohtmxelm Architecture Notes

These notes explain the architecture behind `gohtmxelm`: a Go-first integration
kit for server-owned, real-time hypermedia apps. It lets a Go application use
HTMX, Datastar, Elm islands, immediate-mode canvas islands, and Server-Sent
Events together without letting multiple runtimes fight over the same DOM.

The project is deliberately a library, not an application framework. Your app
still owns routing, persistence, auth, sessions, templates, validation,
localization policy, and deployment. `gohtmxelm` supplies the bridge contracts
and runtime conventions that let independent UI surfaces converge on Go-owned
state.

The `demo/` app is the reference implementation used throughout the docs. It is
not the library API; it is a concrete example of the ownership rules and bridge
patterns that the `github.com/nkhine/gohtmxelm` package supports.

```sh
go install github.com/nkhine/gohtmxelm/cmd/gohtmxelm@latest
gohtmxelm init myapp && cd myapp && make dev   # http://localhost:8080
```

Read these in order if you want the deeper rationale behind the integration
model. These pages are also published as a site (built with
[Zensical](https://zensical.org) and deployed to GitHub Pages by
`.github/workflows/docs.yml`).

New to the project? Start with **[Getting Started](./getting-started.md)** —
install, scaffold, run, and deploy.

## Audience

These docs are for Go teams that want server-rendered workflows with selective
client-side richness:

- backend-heavy apps that want live updates without a full SPA rewrite,
- admin/internal tools where server-owned state and validation matter,
- products that need a few typed islands or canvas tooling surfaces,
- teams that prefer explicit integration contracts over a large framework.

If you want a batteries-included framework, start with a framework. If you want
React/Vue/Svelte to own routing and most state, this library can still inspire
patterns, but it is not trying to be that stack.

## Adoption Paths

You can adopt the library in layers:

| Path | Use when |
|---|---|
| SSE only | You need Go-owned snapshots pushed to open tabs. |
| HTMX plus SSE | You want server-rendered fragments and live updates. |
| Datastar patches | You need small declarative signals or streamed DOM patches. |
| Elm islands | You need a typed local state machine inside a bounded root. |
| Interactions | You need reusable server-rendered overlays and result handling. |
| IMUI islands | You need a canvas surface with local frame-rate interaction. |

The layers are optional. The core rule is not "use everything"; it is "each
surface owns a physical region, and Go coordinates durable state."

## Reading Order

| # | Doc | Focus |
|---|-----|-------|
| — | [Getting Started](./getting-started.md) | Install the CLI, scaffold a project, run it, and deploy. |
| 1 | [Networking foundations](./01-networking-foundations.md) | TCP, HTTP/1.1, chunked transfer encoding, and Server-Sent Events. |
| 2 | [Go backend patterns](./02-go-backend.md) | Streaming handlers, goroutines, channels, graceful shutdown, and server-owned state. |
| 3 | [HTMX hypermedia](./03-htmx-hypermedia.md) | Server-rendered fragments, request/response interactions, and HTML over the wire. |
| 4 | [Datastar signals](./04-datastar-signals.md) | Declarative signals and server-pushed DOM/signal patches. |
| 5 | [Elm islands](./05-elm-types.md) | Typed local state machines, ports, flags, and the Elm Architecture. |
| 6 | [The fusion pattern](./06-fusion-pattern.md) | DOM ownership boundaries, broker responsibilities, and convergence through Go. |
| 7 | [React and the SPA tax](./07-react-and-the-spa-tax.md) | Tradeoffs this pattern avoids and tradeoffs it keeps. |
| 8 | [Svelte and compiler-driven UI](./08-svelte-the-compiler.md) | How compiler-oriented UI compares to this ownership model. |
| 9 | [Testing the contract](./09-testing-the-contract.md) | The convergence invariant, deterministic simulation (`simnet`), real-code `synctest` tests, and the live simulator. |
| 10 | [Localization boundary](./10-localization-boundary.md) | Where i18n/l10n policy lives, and what the library carries as neutral props. |
| 11 | [Datastar over SSE through the edge](./11-edge-sse.md) | Lambda response streaming, API Gateway, Starbase `/api/*`, and Datastar rebinding. |
| 12 | [Local SSO login simulator](./12-local-sso.md) | Browser redirects, callback validation, HttpOnly session cookies, HTMX rehydration, and shared auth presence. |
| 13 | [Integration contracts](./13-contracts.md) | Explicit DOM, broker, SSE, Datastar, HTMX, and interaction result contracts. |
| 14 | [Server-rendered interactions](./14-interactions.md) | Reusable overlay/result conventions for dialogs, pickers, menus, drawers, and flows. |
| 15 | [Immediate-mode canvas islands](./15-immediate-ui.md) | Canvas tooling surfaces for high-frequency interaction under the same ownership model. |

## Core Model

```text
Go owns shared truth.
HTMX requests server-rendered fragments.
Datastar receives server-pushed patches/signals.
Elm owns typed client-side islands.
IMUI owns canvas-backed tooling islands.
SSE carries convergence events.
```

The most important rule is physical DOM ownership:

- HTMX swaps its own fragments.
- Datastar patches its own regions.
- Elm owns its own island roots.
- IMUI owns its own canvas roots.
- Go coordinates shared state and broadcasts changes.

Convergence — every surface ending up agreeing with Go — is the contract the
model makes. [Chapter 9](./09-testing-the-contract.md) shows how it is verified
deterministically and why it depends on reconnect-and-rehydrate rather than on
guaranteed delivery.
