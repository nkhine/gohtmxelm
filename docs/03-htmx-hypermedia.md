# 3. HTMX: hypermedia as the engine of application state

> HTMX is not a JavaScript framework that happens to be small. It's a different
> *theory of the web* — the original one — made ergonomic. To appreciate it you
> have to unlearn the assumption that the browser should hold application state.

---

## 3.1 The idea the industry skipped past: HATEOAS

In the original REST dissertation, there's a constraint with an unwieldy acronym:
**HATEOAS** — *Hypermedia As The Engine Of Application State.* Strip the jargon
and it says something simple and profound:

> The server sends the client not just data, but **the data plus the controls for
> what to do next** — links and forms. The client doesn't need to know the
> application's rules. It just renders what it's given and follows the controls
> the server included.

A plain HTML page is the purest example. It contains content *and* `<a>` links and
`<form>`s — the available next actions — woven together. The browser doesn't know
your business logic; it knows how to render hypermedia and submit forms. The
server drives the application by choosing which controls to send.

The single-page-app era abandoned this. In a typical React app the server sends
**raw JSON**, and the *client* holds the logic for what to do with it: which
buttons exist, when they're enabled, what each one calls, how to re-render. The
hypermedia — the controls — moved into the JavaScript bundle. The server demoted
itself to a database with a URL.

HTMX's bet is that this was a wrong turn for a large class of applications, and
that you can have rich interactivity **while keeping the controls on the server**,
by letting *any* element issue an HTTP request and swap *any* HTML fragment back
into the page.

---

## 3.2 How HTMX works, on the wire

HTMX adds a few HTML attributes. Here is the HTMX write form from this app
([`demo/internal/ui/page.templ`](../demo/internal/ui/page.templ)):

```html
<form hx-post="/api/store" hx-swap="none">
  <input type="hidden" name="key" value="message"/>
  <input type="text" name="value" placeholder="Message from HTMX" required/>
  <button class="btn-htmx" type="submit">Save with HTMX</button>
</form>
```

When submitted, HTMX:

1. Serializes the form as ordinary `application/x-www-form-urlencoded` data —
   the exact bytes a plain HTML form would send.
2. Issues `POST /api/store` via `fetch`.
3. Looks at `hx-swap` to decide what to do with the response. Here it's `none` —
   we don't swap anything, because the SSE stream will update every view. In the
   stopwatch, the buttons use `hx-swap="innerHTML"` to replace themselves with the
   server's freshly-rendered controls.

The server's reply is **HTML, not JSON**:

```go
r.Get("/api/store/fragment", func(w http.ResponseWriter, r *http.Request) {
	ui.StoreEntries(kv.Entries()).Render(r.Context(), w) // renders <table>...
})
```

That is the whole model. The server renders HTML fragments. The client swaps them
in. The "state machine" — which buttons exist, what they do, what the table looks
like — lives in the templates on the server, expressed in HTML.

### The store table is pure hypermedia

Look at how a delete works. The server renders this per row:

```html
<button hx-delete="/api/store/message"
        hx-confirm='Delete key "message" everywhere?'
        hx-swap="none">delete</button>
```

The *control* to delete a key is **part of the rendered state**. The client holds
no list of "deletable keys" and no delete logic. It received a button that knows
its own URL, and clicking it issues `DELETE /api/store/message`. This is HATEOAS
in miniature: the server sent the data (the row) together with the next available
action (the delete control). Add a new row server-side and its delete button comes
with it, for free, with zero client changes.

### Triggers: HTML that reacts to events

HTMX elements can fire on events other than clicks. The store table reloads itself
whenever a custom `store-refresh` event occurs:

```html
<div id="store-entries"
     hx-get="/api/store/fragment"
     hx-trigger="load, store-refresh"
     hx-swap="innerHTML">
```

`hx-trigger="load, store-refresh"` means: fetch on initial load, *and* re-fetch
whenever a `store-refresh` event fires on this element. Our broker dispatches that
event when an SSE store change arrives:

```js
window.htmx.trigger(storeEl, "store-refresh");
```

This is the bridge between push and pull: the **SSE stream** (push) signals that
something changed; **HTMX** (pull) re-fetches the authoritative HTML. HTMX never
has to parse the change or patch the DOM surgically — it just asks the server to
re-render the truth. The stopwatch controls use the same trick with
`hx-trigger="stopwatch-state-change from:body"` so every tab re-syncs its buttons
when any tab acts.

---

## 3.3 Why "HTML over the wire" is not a regression

The instinctive objection is "isn't sending HTML wasteful compared to JSON?"
Usually no, and even when the payload is larger, the trade is overwhelmingly in
your favour:

- **The rendering logic already exists on the server**, in your templates, in one
  language. You don't duplicate it as a second rendering layer in JavaScript. The
  whole *category* of "keep the client and server views in sync" disappears.
- **HTML gzips extremely well** — it's repetitive, structured text. The wire
  difference is often negligible.
- **You ship almost no application JavaScript.** HTMX is ~14 KB once, cached
  forever. There is no per-feature bundle growth. Compare with doc 7.
- **The browser does what it's optimized for**: parsing and rendering HTML. That
  path is decades-tuned C++.

The deeper win is **locality of behaviour**. With HTMX, an element declares what
it does *right there in the markup*: `hx-post`, `hx-target`, `hx-swap`. You don't
trace through a component tree, a reducer, an action creator, and an effect to
learn what a button does. You read the tag. For a huge class of CRUD-shaped,
form-shaped, page-shaped applications — which is most applications — this locality
is a massive maintainability win.

---

## 3.4 Where HTMX's boundary is — and why that's healthy

HTMX is superb at *request/response over hypermedia*. It is deliberately **not**
good at:

- rich client-side state that never touches the server (a complex multi-step
  wizard with intricate local validation),
- continuous local computation (a live-updating chart from a fast data feed),
- anything where the *interesting logic is inherently client-side*.

This app respects that boundary precisely. HTMX owns the **server-rendered store
table** and the **stopwatch control buttons** — request/response surfaces. It does
*not* try to own the live-ticking readout (that's Datastar's push job) or the
typed lap analytics (that's Elm's computation job). A tool with a clear boundary
that it stays inside is a tool you can reason about. (Doc 6 makes these boundaries
explicit.)

A good rule emerges: **use HTMX wherever the server rendering the HTML is the
simplest honest description of the feature.** When that stops being true, you've
found the edge — and that's where Datastar or Elm earn their place.

---

## 3.5 What to take away

- **HATEOAS**: the server sends content *and the controls for what to do next*.
  HTMX makes this practical for interactive apps.
- HTMX works by letting any element issue an HTTP request and swap an **HTML**
  fragment from the response — the bytes are ordinary form posts and HTML.
- **Application state and rendering live on the server**, in one language, with no
  duplicated client renderer to keep in sync.
- `hx-trigger` lets HTML **react to events** — including events fired by an SSE
  stream — which is how push (SSE) and pull (HTMX re-fetch) combine here.
- **Locality of behaviour**: a tag declares what it does. You read it, you know it.
- HTMX has a clear boundary. Respecting it — and reaching for the right tool past
  it — is what makes the whole fusion coherent.

Next: [Datastar — fine-grained reactivity and a server that pushes signals →](./04-datastar-signals.md)
