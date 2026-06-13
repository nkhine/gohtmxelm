# 4. Datastar: signals and server-pushed reactivity

> HTMX swaps whole fragments on request/response. Datastar is for the other half
> of reactivity: **fine-grained local state** (signals) and **a server that
> pushes DOM patches and state updates** down a live stream. It does this with
> declarative attributes and no application JavaScript at all.

---

## 4.1 The two ideas: signals and patches

Datastar combines two things that usually require a framework each:

1. **Signals** — reactive variables that live in the DOM. When a signal changes,
   every expression that reads it updates automatically. This is the same
   "fine-grained reactivity" model as SolidJS or Vue's refs, but expressed in
   `data-*` attributes rather than a component tree.
2. **Server-driven patches over SSE** — the server can push two kinds of events:
   `datastar-patch-elements` (replace/morph DOM by id) and
   `datastar-patch-signals` (update signal values). The browser applies them with
   no glue code.

The combination is potent: local interactivity is declarative and instant
(signals), while the server can reach into the page and update both *content* and
*state* whenever it likes (patches). And critically — **you write no custom JS to
wire any of it.**

---

## 4.2 Signals, declared in markup

Here's the Datastar write form from this app
([`demo/internal/ui/page.templ`](../demo/internal/ui/page.templ)):

```html
<div class="control-group" data-signals="{messageDraft: ''}">
  <form data-on:submit="@post('/api/datastar/store',
                               {payload: {key: 'message', value: $messageDraft}})">
    <input data-bind:message-draft type="text" .../>
    <button type="submit">Save with Datastar</button>
  </form>
  <p data-text="$messageDraft
                  ? 'Datastar will save: ' + $messageDraft
                  : 'Datastar signal preview appears here.'">
  </p>
</div>
```

Walk through the `data-*` attributes — each is a small, declarative binding:

- `data-signals="{messageDraft: ''}"` declares a reactive signal `messageDraft`,
  scoped to this subtree.
- `data-bind:message-draft` two-way binds the input to `$messageDraft`. Type in
  the box and the signal updates on every keystroke.
- `data-text="..."` makes the paragraph's text a live expression. Because it reads
  `$messageDraft`, it re-renders the instant the signal changes — a live preview,
  with zero event listeners written by you.
- `data-on:submit="@post(...)"` issues a POST whose body is built from signals.

No component, no `useState`, no virtual DOM. The reactive graph is the set of
expressions referencing each signal, and Datastar maintains it. This is *fine-
grained* reactivity: only the exact text node bound to `$messageDraft` updates,
not a re-rendered component subtree that a diff then reconciles.

---

## 4.3 The server pushes DOM patches *and* signals

This is the part that distinguishes Datastar from a pure client reactivity
library. The server holds a long-lived SSE connection and pushes two event types.
From [`main.go`](../main.go):

```go
func writeDatastarPatchElements(w http.ResponseWriter, elements string) {
	fmt.Fprint(w, "event: datastar-patch-elements\n")
	for _, line := range strings.Split(elements, "\n") {
		fmt.Fprintf(w, "data: elements %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func writeDatastarPatchSignals(w http.ResponseWriter, signals string) {
	fmt.Fprint(w, "event: datastar-patch-signals\n")
	fmt.Fprintf(w, "data: signals %s\n\n", signals)
}
```

These are *just SSE events* (see [doc 1](./01-networking-foundations.md)) with
names Datastar recognizes. The browser opens the stream declaratively:

```html
<div id="datastar-stream"
     data-signals="{writes: 0, lastWriter: ''}"
     data-init="@get('/api/datastar/store/stream')">
```

`data-init="@get(...)"` opens the SSE connection on load. From then on:

- **`datastar-patch-elements`** carries an HTML fragment. Datastar finds the
  element with the matching `id` and **morphs** it — patching only the parts that
  differ rather than blowing away and rebuilding. The store table region and the
  stopwatch readout update this way.
- **`datastar-patch-signals`** carries new signal values. The server can set
  `writes` and `lastWriter` from afar:

```go
writeDatastarPatchSignals(w, fmt.Sprintf(`{"writes": %d, "lastWriter": %q}`, e.Seq, e.Source))
```

And in the markup, those server-set signals drive live text with no client code:

```html
<span data-text="'Total writes: ' + $writes">Total writes: 0</span>
<span data-text="$lastWriter ? 'Last writer: ' + $lastWriter : 'No writes this session'"></span>
```

Sit with that for a second. **The server incremented a counter and the badge in
the browser updated — and there is no JavaScript anywhere that you wrote to make
that happen.** The signal is the contract; the server patches it; the bound
expression re-renders. This is the "no custom app JS" promise made concrete.

---

## 4.4 Splitting a stream by change frequency

A subtle but important technique appears in the stopwatch. The live readout ticks
10×/second, but the lap list changes only when the user hits Lap or Reset. If we
pushed the whole region every tick, we'd re-send the lap list 10×/second for
nothing. So the server splits the region into two separately-targetable elements
and patches them at different rates:

```go
// every tick — tiny:
writeDatastarPatchElements(w, renderStopwatchReadout(ev.Snapshot)) // #stopwatch-readout
// only on a real state change — larger, but rare:
if ev.StateChange {
	writeDatastarPatchElements(w, renderStopwatchLaps(ev.Snapshot)) // #stopwatch-laps
}
```

Because Datastar patches *by element id*, two ids means two independent update
streams from one connection. The high-frequency thing stays small; the larger
thing updates only when it must. This is the patch-granularity lever that
fine-grained, id-addressable patching gives you — and it's the kind of
optimization that a "re-render the component and diff it" model makes awkward.

---

## 4.5 The cost: in-browser expression compilation (and a CSP note)

Datastar's declarative expressions (`data-text="..."`, `@post(...)`) are compiled
**in the browser** at runtime. That is what makes them so terse — but it means the
browser must be allowed to evaluate dynamically generated code. This app's Content
Security Policy therefore includes `script-src 'self' 'unsafe-eval'`:

```go
w.Header().Set("Content-Security-Policy",
	"default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'")
```

This is a real, honest trade-off worth naming: declarative-expression libraries
buy their ergonomics with `unsafe-eval`. In a high-security context you'd weigh
that carefully. It's the kind of cost that's invisible until you understand the
layer beneath the framework — a recurring theme in these docs.

---

## 4.6 Datastar vs HTMX — they are not competitors

It's tempting to see HTMX and Datastar as rivals; they're complements with
different shapes:

| | HTMX | Datastar |
|---|---|---|
| Direction | Request → HTML response | Local signals + server **push** |
| Update unit | Swap an HTML fragment | Morph an element by id; set a signal |
| Local state | None (server holds it) | Signals (`data-signals`) |
| Best for | Forms, navigation, server-rendered views | Live readouts, declarative local UI, server-pushed dashboards |
| Custom JS | None | None |

In this app HTMX owns request/response surfaces (the store table, the stopwatch
buttons) and Datastar owns the *live, server-pushed* surfaces (the patched store
region, the ticking readout, the write counter). Neither patches the other's DOM.
They meet only through shared **server state** and the **SSE stream** — never by
fighting over the same nodes. (Doc 6 formalizes this rule.)

---

## 4.7 What to take away

- Datastar = **signals** (fine-grained reactive local state in `data-*` attrs) +
  **server-pushed patches** (`datastar-patch-elements`, `datastar-patch-signals`)
  over SSE.
- The server can update both **content** and **state** in the browser with no
  application JavaScript — the signal is the contract.
- Fine-grained, **id-addressable** patching lets you tune update granularity (the
  stopwatch's split readout/laps), which a re-render-and-diff model handles less
  gracefully.
- The ergonomics cost `unsafe-eval` in the CSP — a real trade-off you can only
  evaluate if you understand the layer beneath the framework.
- Datastar and HTMX are **complementary**: one for request/response HTML, one for
  live local + pushed state. The discipline is to never let them touch the same
  DOM.

Next: [Elm — types, the update loop, and making impossible states unrepresentable →](./05-elm-types.md)
