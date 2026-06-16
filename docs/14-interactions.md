# Server-Rendered Interactions

Interactions are temporary UI surfaces: confirms, prompts, pickers, drawers,
menus, lightboxes, and wizards. The reusable package provides a small browser
runtime and HTML conventions for these without owning application state.

## Mount Assets

Mount the embedded runtime once:

```go
kit := gohtmxelm.New(gohtmxelm.Options{AssetPath: "/gohtmxelm"})

mux.Handle("/gohtmxelm/",
	http.StripPrefix("/gohtmxelm/", kit.Assets()))
```

Render the script and root on pages that open interactions:

```go
template.HTML(kit.InteractionScript())
template.HTML(gohtmxelm.InteractionRoot(""))
```

## Open A Fragment

The runtime listens for `data-gohtmxelm-open`. The value may be a full URL:

```html
<button
  data-gohtmxelm-open="/api/interactions/confirm"
  data-gohtmxelm-status="#delete-result">
  Delete
</button>

<span id="delete-result">awaiting click</span>
```

The runtime loads the fragment with HTMX and appends it to
`[data-gohtmxelm-interactions-root]`.

## Render A Fragment

Fragments carry their result target:

```html
<div data-gohtmxelm-fragment data-gohtmxelm-backdrop>
  <section role="dialog" aria-modal="true"
    data-gohtmxelm-status-target="#delete-result">
    <p>Delete this item?</p>
    <button data-gohtmxelm-result="cancelled">Cancel</button>
    <button data-gohtmxelm-result="deleted">Continue</button>
  </section>
</div>
```

Clicking a result button:

- updates the target status element,
- emits `gohtmxelm:interaction-result`,
- removes the closest fragment/backdrop.

## Result Event

Host pages can observe results:

```js
document.addEventListener("gohtmxelm:interaction-result", (e) => {
  const { target, result } = e.detail
  console.log(target, result)
})
```

Treat this as local UI feedback. Post to Go for durable writes.

## Programmatic API

The runtime exposes:

```js
window.GoHTMXElmInteractions.open("/api/interactions/picker", "#result")
window.GoHTMXElmInteractions.close(element, "selected Invoice")
window.GoHTMXElmInteractions.setStatus("#result", "selected Invoice")
window.GoHTMXElmInteractions.top()
```

This is useful for keyboard shortcuts, caller-side timeouts, and SSE-driven
resolution.

## Validation Flow

For server-side form validation, target the panel:

```html
<form
  hx-post="/api/interactions/save"
  hx-target="closest [data-gohtmxelm-status-target]"
  hx-swap="outerHTML">
  <input name="name">
  <button type="submit">Save</button>
</form>
```

On failure, return the same panel with submitted values and errors. On success,
return a panel with a result button or send `204` and emit a separate event.

## Streamed Progress

For singleton toasts, give the target a stable id and patch it from Go:

```go
stream.PatchElements(render(ProgressToast("Downloading...", 40)))
```

Every update replaces the same element instead of opening a new toast.
