# How this app works — and why it's built this way

This is a guided tour of a small but unusual web application. On one screen it
runs **four different ways of building UIs at once** — HTMX, Datastar, and Elm —
all fused together by a single **Go** backend and a stream of **Server-Sent
Events (SSE)**.

That sounds like a recipe for chaos. It isn't. The whole point is to show that
when you understand what each tool is *actually for* — and what the network
underneath is *actually doing* — you can combine them cleanly, with each one
owning the job it's best at, and none of them fighting over the same DOM.

These docs are written to be **read in order**, like a short book. You do not
need to be an expert. You do need to be willing to think about what happens on
the wire, not just what happens in the framework.

## Why you should care

Most front-end tutorials teach you a framework. They rarely teach you the layer
the framework sits on. So developers end up fluent in React or Vue but unable to
answer basic questions:

- What is actually sent over the wire when a user clicks a button?
- Why does my "real-time" feature need a 300 KB JavaScript bundle?
- Where does my application's *state* really live, and who is allowed to change it?
- Why did the browser console say `ERR_INCOMPLETE_CHUNKED_ENCODING`?

This project is small enough to read in an afternoon and deep enough to answer
all of those. By the end you should be able to reason from **TCP bytes → HTTP
framing → SSE → framework → pixels** without any magic in between.

## The reading order

| # | Doc | What you'll learn |
|---|-----|-------------------|
| 1 | [Networking foundations](./01-networking-foundations.md) | TCP, HTTP/1.1, chunked transfer encoding, and Server-Sent Events — from first principles, on the wire. Includes the real `ERR_INCOMPLETE_CHUNKED_ENCODING` bug we hit and fixed. |
| 2 | [Go: the backend and the single source of truth](./02-go-backend.md) | Why Go is a natural fit for streaming servers: goroutines, channels, a pub/sub key-value store, and graceful shutdown that doesn't corrupt streams. |
| 3 | [HTMX: hypermedia as the engine of state](./03-htmx-hypermedia.md) | The original vision of the web (HATEOAS), why "HTML over the wire" is not a step backwards, and how HTMX works byte-for-byte. |
| 4 | [Datastar: signals and server-pushed reactivity](./04-datastar-signals.md) | Fine-grained reactivity without a virtual DOM, declarative `data-*` attributes, and a server that pushes DOM patches *and* state signals. |
| 5 | [Elm: types, the update loop, and impossible states](./05-elm-types.md) | A pure functional language that makes whole categories of bugs unrepresentable. The Elm Architecture, ports, and why "it compiles" means more here. |
| 6 | [The fusion pattern](./06-fusion-pattern.md) | How the three front-ends coexist: ownership boundaries, the broker, and convergence through shared server state. Why this is a genuinely strong architecture. |
| 7 | [React and the SPA tax](./07-react-and-the-spa-tax.md) | An honest look at what large client-side frameworks cost you — bundle size, hydration, state-management sprawl — and which of those costs this design avoids, and which it doesn't. |
| 8 | [Svelte: the framework that compiles itself away](./08-svelte-the-compiler.md) | The compiler approach to UI: no virtual DOM, signals via runes, why "the framework disappears at build time" is genuinely interesting — and which SPA taxes it does and doesn't escape. |

## The one-paragraph version

The server (Go) owns all the truth. The browser holds **no authoritative
state** — every meaningful change is a request to the server, and the server
broadcasts the result back over a long-lived SSE connection. HTMX uses that to
swap server-rendered HTML fragments. Datastar uses it to patch DOM regions and
update reactive signals. Elm uses it to feed typed state machines that compute
things types make safe. Because the *truth* lives in one place and travels over
one stream, three very different UI philosophies can share a page without ever
corrupting each other.

## The mental model to hold the whole time

```
        ┌─────────── the browser (one tab) ───────────┐
        │                                              │
  HTMX form ─┐   Datastar form ─┐   Elm port ─┐        │
             │                  │             │        │
             └──────────────────┴─────────────┘        │
                          │ HTTP POST / fetch           │
                          ▼                             │
                ┌───────────────────┐                  │
   ◀── SSE ─────│   Go server       │                  │
   (one long    │   - KV store      │   the ONLY       │
    stream per  │   - pub/sub       │   source of      │
    consumer)   │   - stopwatch     │   truth          │
                └───────────────────┘                  │
                          │ broadcast                   │
        ┌─────────────────┴───────────────────┐        │
        ▼                 ▼                    ▼        │
   HTMX table       Datastar region      Elm islands    │
   (HTML swap)      (DOM patch+signals)  (typed update)  │
        └──────────────────────────────────────────────┘
              every surface converges on the same truth
```

Keep that picture in your head. Everything else is detail.
