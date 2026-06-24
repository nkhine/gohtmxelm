# 15. Immediate-mode canvas islands

Immediate-mode UI is for canvas-backed tooling surfaces: construction planes,
network diagrams, timelines, graph editors, simulation playback, and other
high-frequency views where DOM patching is the wrong unit of work.

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

## Register a drawing module

```js
window.GoHTMXElmIMUI.register("LatticeTool", {
  init(api, props) {
    return { snapshot: null, hover: null, viewport: { x: 0, y: 0, scale: 1 } }
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
    if (input.type === "pointerup" && model.hover) {
      api.command({ type: "select", at: model.hover })
    }
  },

  draw(model, api) {
    const { ctx, canvas, dpr } = api
    ctx.clearRect(0, 0, canvas.width / dpr, canvas.height / dpr)
    // Draw from model.snapshot plus ephemeral local state.
  },
})
```

`api.command(payload)` posts JSON to the island's `CommandURL`:

```json
{"islandId":"lattice","command":{"type":"select","at":{"x":100,"y":80}}}
```

For lattice construction, the canvas should preview gestures immediately, then
send commands such as `add-node`, `add-cover`, `move-node`, or `select`. Go
validates the partial order/lattice rules, mutates the canonical model, and
broadcasts the next authoritative snapshot over SSE.
