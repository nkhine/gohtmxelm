# 15. Immediate-mode canvas islands

Immediate-mode UI is for canvas-backed tooling surfaces: construction planes,
network diagrams, timelines, graph editors, simulation playback, and other
high-frequency views where DOM patching is the wrong unit of work.

The library normally encourages HTML over the wire and typed islands because
those are excellent defaults for forms, tables, dashboards, message feeds,
dialogs, and business flows. Canvas tooling has a different shape. A pointer
can move dozens of times between server round trips. A drag preview should not
wait for a DOM patch. A pan or zoom gesture should be local, fluid, and cheap.

IMUI exists for that gap. It lets a page host a local, immediate-mode drawing
surface without changing the core philosophy of `gohtmxelm`: Go still owns the
shared truth, commands are still validated on the server, and convergence still
arrives over SSE.

It does not replace the rest of the pattern. It adds one more bounded owner:

| Surface | Owns |
|---|---|
| Go | durable state, validation, commands, SSE fan-out |
| HTMX | server-rendered request/response fragments |
| Datastar | declarative local signals and server-pushed DOM/signal patches |
| Elm | typed client-side islands and local state machines |
| IMUI | canvas drawing, viewport state, pointer/keyboard state, drag previews |

The same ownership rule applies: no library patches another library's DOM. An
IMUI island owns one `<canvas>` element and redraws that canvas from local state.
It may keep ephemeral interaction state locally, but durable domain state should
still converge through Go.

## Why immediate mode belongs here

The rest of `gohtmxelm` is deliberately conservative:

- HTMX moves HTML fragments across request/response boundaries.
- Datastar binds small declarative signals and applies server-pushed patches.
- Elm owns typed local state machines where client logic needs structure.
- SSE distributes the current Go-owned state until every surface agrees.

That model works best when the unit of UI is DOM. Canvas tools use a different
unit: a frame. A graph editor wants to repaint the same canvas from a model on
every animation frame. A simulation wants to scrub or replay without replacing
nodes. A topology view wants hover, selection, panning, and lasso previews that
feel local.

IMUI keeps that frame loop local while preserving the integration contract:

```text
pointer/keyboard input -> local canvas preview
accepted command       -> POST JSON to Go
Go validation          -> mutate canonical state
SSE snapshot           -> canvas, Elm, HTMX, Datastar converge
```

The end user gets the important part: rich interaction that feels immediate,
without a separate client-side source of truth drifting away from the server.

## Mount the runtime

```go
kit := gohtmxelm.New(gohtmxelm.Options{
	AssetPath: "/gohtmxelm",
	Sources: []gohtmxelm.Source{
		{URL: "/api/events", Events: []string{"lattice-snapshot"}},
	},
})

template.HTML(kit.IMUIScript())
```

If the broker runtime is already present, IMUI listens to `gohtmxelm:sse` events
from it. If no broker is present, the IMUI runtime opens the configured SSE
sources itself.

That means IMUI can be used in two modes:

- **Beside Elm/HTMX/Datastar**: render `kit.BrowserScript()` once and
  `kit.IMUIScript()` on pages with canvas islands. The broker opens the SSE
  stream, and IMUI consumes the brokered DOM event.
- **Canvas-only page**: render `kit.IMUIScript()` with `Options.Sources`. The
  IMUI runtime opens its own `EventSource` connections and delivers events to
  registered modules.

## Render a canvas island

```go
html, err := gohtmxelm.CanvasIsland("lattice", "LatticeTool",
	map[string]any{"snap": true},
	gohtmxelm.CanvasOptions{
		CommandURL: "/api/lattice/commands",
		Events:     []string{"lattice-snapshot"},
		Label:      "Lattice construction canvas",
	},
)
```

The generated element is a normal canvas with stable data attributes:

```html
<canvas
  class="gohtmxelm-imui"
  id="lattice"
  data-gohtmxelm-imui-module="LatticeTool"
  data-gohtmxelm-imui-id="lattice"
  data-props="{&quot;snap&quot;:true}"
  data-command-url="/api/lattice/commands"
  data-events="[&quot;lattice-snapshot&quot;]"
  tabindex="0"
  role="img"
  aria-label="Lattice construction canvas">
</canvas>
```

`CanvasOptions` keeps the server-side contract explicit:

| Option | Purpose |
|---|---|
| `Width`, `Height` | Optional CSS dimensions in pixels. The runtime still sizes the backing buffer for `devicePixelRatio`. |
| `CommandURL` | Receives `api.command(payload)` POSTs. Leave empty for read-only visualisations. |
| `Events` | Limits which SSE event names are delivered to this island. Empty means every configured event is delivered. |
| `Class` | Adds CSS classes beside `gohtmxelm-imui`. |
| `Role`, `Label` | Accessibility attributes. `Label` defaults the role to `img`. |

## Register a drawing module

```js
window.GoHTMXElmIMUI.register("LatticeTool", {
  init(api, props) {
    return {
      snapshot: null,
      hover: null,
      drag: null,
      viewport: { x: 0, y: 0, scale: 1 },
      snap: props.snap === true,
    }
  },

  event(model, event, api) {
    if (event.name === "lattice-snapshot") {
      model.snapshot = event.data
      api.invalidate()
    }
  },

  input(model, input, api) {
    if (input.type === "pointermove") {
      model.hover = input.position
      api.invalidate()
    }
    if (input.type === "pointerdown") {
      model.drag = { from: input.position, to: input.position }
      api.invalidate()
    }
    if (input.type === "pointerup" && model.drag) {
      api.command({ type: "add-cover", from: model.drag.from, to: input.position })
      model.drag = null
      api.invalidate()
    }
  },

  draw(model, api) {
    const { ctx, canvas, dpr } = api
    const width = canvas.width / dpr
    const height = canvas.height / dpr
    ctx.clearRect(0, 0, width, height)
    // Draw from model.snapshot plus ephemeral local state.
  },
})
```

The runtime calls four optional hooks:

| Hook | When it runs |
|---|---|
| `init(api, props)` | Once at mount. Return the module's local model. |
| `event(model, event, api)` | For each matching SSE event. |
| `input(model, input, api)` | For normalised pointer, wheel, keyboard, focus, and blur input. |
| `draw(model, api)` | On the next animation frame after `api.invalidate()`. |
| `destroy(model, api)` | When the canvas leaves the document. |

The `api` object contains `islandId`, `canvas`, `ctx`, `dpr`,
`invalidate()`, `command(payload)`, and `emit(name, detail)`.

`input.position` is in CSS pixels relative to the canvas. The runtime handles
high-DPI backing-buffer sizing and applies the `devicePixelRatio` transform
before calling `draw`, so drawing code can also use CSS pixels.

`api.command(payload)` posts JSON to the island's `CommandURL`:

```json
{
  "islandId": "lattice",
  "command": {
    "type": "add-cover",
    "from": { "x": 100, "y": 80 },
    "to": { "x": 180, "y": 120 }
  }
}
```

For lattice construction, the canvas should preview gestures immediately, then
send commands such as `add-node`, `add-cover`, `move-node`, or `select`. Go
validates the partial order/lattice rules, mutates the canonical model, and
broadcasts the next authoritative snapshot over SSE.

## Handle commands in Go

An IMUI command endpoint is intentionally ordinary HTTP. The runtime sends JSON,
the handler decodes it, validates the command against the same rules used by
other surfaces, mutates Go-owned state, and broadcasts a snapshot.

```go
type LatticeCommandRequest struct {
	IslandID string          `json:"islandId"`
	Command  json.RawMessage `json:"command"`
}

type AddCoverCommand struct {
	Type string `json:"type"`
	From Point  `json:"from"`
	To   Point  `json:"to"`
}

func latticeCommandHandler(w http.ResponseWriter, r *http.Request) {
	var req LatticeCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(req.Command, &envelope); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch envelope.Type {
	case "add-cover":
		var cmd AddCoverCommand
		if err := json.Unmarshal(req.Command, &cmd); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := lattice.AddCover(cmd.From, cmd.To); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
	default:
		http.Error(w, "unknown command", http.StatusBadRequest)
		return
	}

	broadcaster.Publish(lattice.Snapshot())
	w.WriteHeader(http.StatusNoContent)
}
```

The important point is not the exact command type. It is that the command shape
can be shared. In the demo lattice card, HTMX, Datastar, Elm, and IMUI all send
commands to Go. IMUI only gets special treatment for the canvas event loop.

## Fit with the rest of the library

IMUI sits beside the existing browser integrations:

```text
Go
├── renders HTML fragments for HTMX
├── sends Datastar element/signal patches
├── sends broker envelopes to Elm islands
├── sends snapshots to IMUI islands
└── accepts commands from every surface
```

Use IMUI when the display surface itself is a canvas tool. Do not use it just
because a component has local state. A date picker, confirmation dialog, menu,
table filter, or form workflow is usually better as HTMX, Datastar, Elm, or a
server-rendered interaction. IMUI earns its place when the user manipulates a
visual plane directly.

A practical split looks like this:

| Need | Prefer |
|---|---|
| Submit a form and replace a fragment | HTMX |
| Bind small local values and receive server-pushed patches | Datastar |
| Maintain a typed client-side state machine | Elm |
| Draw and manipulate a high-frequency visual plane | IMUI |

## Runtime behavior

The runtime is deliberately small and mechanical:

- `mountAll(root)` finds `[data-gohtmxelm-imui-module]` canvases.
- `register(name, module)` stores a drawing module and mounts matching canvases.
- `ResizeObserver` invalidates the frame when the element changes size.
- `requestAnimationFrame` batches redraws after `api.invalidate()`.
- Pointer, wheel, keyboard, focus, and blur events are normalised before they
  reach the module.
- `gohtmxelm:sse` events from the broker are delivered to matching islands.
- Without the broker, configured `EventSource` streams are opened directly.
- Removed canvases are unmounted, their listeners are detached, and
  `destroy(model, api)` is called when present.

The runtime also emits DOM events for observability:

| Event | Detail |
|---|---|
| `gohtmxelm:imui-mounted` | `{ islandId, module }` |
| `gohtmxelm:imui-unmounted` | `{ islandId }` |
| `gohtmxelm:imui-mount-failed` | `{ reason }` |
| `gohtmxelm:imui-command` | `{ islandId, command }` before POST |
| `gohtmxelm:imui-command-result` | `{ islandId, ok, status }` after POST |
| `gohtmxelm:imui-sse` | `{ event, data }` for directly opened streams |
| `gohtmxelm:imui-source-open` | `{ url }` |
| `gohtmxelm:imui-source-error` | `{ url }` |

Those events make browser tests and host-level logging possible without forcing
test hooks into the drawing module.

## Accessibility and fallbacks

A canvas can be an excellent interaction surface and a poor semantic surface.
Use `Label` to name it, keep the canvas focusable, and provide keyboard input
for important operations. If the canvas represents durable data that must be
read or copied, render a server-owned companion table, summary, or details
panel outside the canvas. That companion surface can be HTMX, Datastar, or Elm;
it should consume the same Go-owned snapshot.

## Testing

Most IMUI tests should avoid pixel-perfect assertions. Test the boundaries:

- Go tests decode command payloads, validate command policy, and assert the
  resulting snapshot.
- JavaScript or Playwright tests dispatch pointer/keyboard events and assert
  that `gohtmxelm:imui-command` carries the expected command shape.
- Browser tests can assert that an SSE snapshot causes the module to redraw or
  updates a visible companion surface.
- Contract tests should still verify convergence at the Go/SSE boundary, not
  rely on every intermediate event being delivered.

For the reusable package itself, `imui_test.go` checks script generation,
canvas markup, command event JSON, and runtime embedding. The demo's browser
tests cover the integrated five-way example.
