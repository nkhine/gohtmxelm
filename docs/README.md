# gohtmxelm Architecture Notes

These notes explain the architecture behind `gohtmxelm`: a Go-first way to use
HTMX, Datastar, Elm islands, and Server-Sent Events together without letting
multiple runtimes fight over the same DOM.

The `demo/` app is the reference implementation used throughout the docs. It is
not the library API; it is a concrete example of the ownership rules and bridge
patterns that the `pkg/` package supports.

Read these in order if you want the deeper rationale behind the integration
model.

## Reading Order

| # | Doc | Focus |
|---|-----|-------|
| 1 | [Networking foundations](./01-networking-foundations.md) | TCP, HTTP/1.1, chunked transfer encoding, and Server-Sent Events. |
| 2 | [Go backend patterns](./02-go-backend.md) | Streaming handlers, goroutines, channels, graceful shutdown, and server-owned state. |
| 3 | [HTMX hypermedia](./03-htmx-hypermedia.md) | Server-rendered fragments, request/response interactions, and HTML over the wire. |
| 4 | [Datastar signals](./04-datastar-signals.md) | Declarative signals and server-pushed DOM/signal patches. |
| 5 | [Elm islands](./05-elm-types.md) | Typed local state machines, ports, flags, and the Elm Architecture. |
| 6 | [The fusion pattern](./06-fusion-pattern.md) | DOM ownership boundaries, broker responsibilities, and convergence through Go. |
| 7 | [React and the SPA tax](./07-react-and-the-spa-tax.md) | Tradeoffs this pattern avoids and tradeoffs it keeps. |
| 8 | [Svelte and compiler-driven UI](./08-svelte-the-compiler.md) | How compiler-oriented UI compares to this ownership model. |
| 9 | [Testing the contract](./09-testing-the-contract.md) | The convergence invariant, deterministic simulation (`pkg/simnet`), real-code `synctest` tests, and the live simulator. |
| 10 | [Localization boundary](./10-localization-boundary.md) | Where i18n/l10n policy lives, and what the library carries as neutral props. |
| 11 | [Datastar over SSE through the edge](./11-edge-sse.md) | Lambda response streaming, API Gateway, Starbase `/api/*`, and Datastar rebinding. |
| 12 | [Local SSO login simulator](./12-local-sso.md) | Browser redirects, callback validation, HttpOnly session cookies, HTMX rehydration, and shared auth presence. |
| 13 | [Integration contracts](./13-contracts.md) | Explicit DOM, broker, SSE, Datastar, HTMX, and interaction result contracts. |
| 14 | [Server-rendered interactions](./14-interactions.md) | Reusable overlay/result conventions for dialogs, pickers, menus, drawers, and flows. |

## Core Model

```text
Go owns shared truth.
HTMX requests server-rendered fragments.
Datastar receives server-pushed patches/signals.
Elm owns typed client-side islands.
SSE carries convergence events.
```

The most important rule is physical DOM ownership:

- HTMX swaps its own fragments.
- Datastar patches its own regions.
- Elm owns its own island roots.
- Go coordinates shared state and broadcasts changes.

Convergence — every surface ending up agreeing with Go — is the contract the
model makes. [Chapter 9](./09-testing-the-contract.md) shows how it is verified
deterministically and why it depends on reconnect-and-rehydrate rather than on
guaranteed delivery.
