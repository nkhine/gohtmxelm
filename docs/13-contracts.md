# Integration Contracts

`gohtmxelm` is mostly a set of transport and DOM ownership conventions. This
page names the contracts explicitly so host applications can copy the pattern
without copying the demo.

## DOM Ownership

One runtime owns one physical region:

- HTMX swaps server-rendered fragments outside Elm roots.
- Datastar patches elements that are not inside Elm roots.
- Elm owns `.elm-island` roots after the broker mounts them.
- Go owns durable state and sends convergence events.

Do not patch, morph, or swap inside another runtime's owned root. When two
surfaces need the same state, send the change to Go and let Go fan out the
result.

## Broker Envelope

Elm islands talk to the broker with this JSON envelope:

```json
{
  "version": 1,
  "type": "STATE_SET",
  "target": "broker",
  "payload": { "key": "message", "value": "hello" }
}
```

The broker stamps `source` itself. Island-supplied `source` is ignored.

Common types:

- `READY`: island asks the broker to complete the handshake.
- `STATE_SET`: replace one shared broker state key.
- `STATE_PATCH`: merge several shared broker state keys.
- `HTMX_SWAP`: ask HTMX to load a server fragment.
- `SSE_EVENT`: broker-to-island event created from a forwarded SSE frame.
- `BROKER_READY`: broker-to-island handshake response.

Targets:

- `broker`: handled by the broker.
- `broadcast`: sent to every mounted island.
- `others`: sent to every mounted island except the sender.
- any island id: sent to that island if mounted.

## SSE Events

Use named SSE events and JSON payloads when the browser should route by domain:

```text
event: store-change
data: {"key":"message","value":"hello","version":2}
```

The broker forwards configured events to islands as:

```json
{
  "type": "SSE_EVENT",
  "payload": {
    "event": "store-change",
    "data": { "key": "message", "value": "hello", "version": 2 }
  }
}
```

Prefer one multiplexed broker stream per page. Separate EventSource connections
are fine for isolated Datastar examples, but gallery-style pages should avoid
opening many long-lived HTTP/1.1 connections.

## Datastar Patches

Element patches must include stable ids:

```go
stream.PatchElements(`<div id="summary">...</div>`)
```

Signal patches must be JSON objects:

```go
stream.PatchSignals(map[string]any{"count": 12})
```

Datastar-owned regions may contain HTMX controls, but Datastar should not morph
inside Elm roots.

## HTMX Responses

Use normal HTML fragments for request/response interactions. Use `HX-Trigger`
when a server-side change should wake another HTMX region:

```go
gohtmxelm.Trigger(w, "store-refresh")
gohtmxelm.NoContent(w)
```

If a form should stay open on validation failure, return the same panel fragment
with the submitted values preserved and target `closest` panel with
`hx-swap="outerHTML"`.

## Interaction Result Event

The interaction runtime emits:

```text
gohtmxelm:interaction-result
```

with detail:

```json
{
  "target": "#result",
  "result": "accepted"
}
```

This is intentionally UI-local. Persist anything important by posting to Go.
