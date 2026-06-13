# 8. Svelte: the framework that compiles itself away

> Svelte isn't used in the reference demo, but its core idea is one of the most genuinely
> interesting in front-end engineering and it connects directly to two tools we
> *do* use. Where React ships a runtime that interprets your components in the
> browser, **Svelte is a compiler**: it reads your components at build time and
> emits plain, imperative JavaScript that updates the DOM directly. The framework,
> in large part, *disappears* before the browser ever sees it.

---

## 8.1 The core insight: do the work at build time, not run time

React's model is *runtime*. You ship React + ReactDOM to the browser, and at run
time they interpret your component tree, build a **virtual DOM**, diff it against
the previous virtual DOM on every update, and apply the minimal real-DOM changes.
That diffing machinery is powerful and general — and it's pure overhead that every
user downloads and every update pays.

Svelte asks a different question: *why do this work in the browser at all?* Your
component's structure is known at build time. So Svelte's compiler reads:

```svelte
<script>
  let count = 0;
</script>
<button on:click={() => count += 1}>
  clicked {count} times
</button>
```

…and emits JavaScript that, in essence, says: *"create a button; when clicked,
increment `count`; when `count` changes, set this exact text node's content."*
There is **no virtual DOM, no diff, no reconciliation at run time** — just direct,
surgical instructions to update the specific nodes that depend on the specific
data that changed. The "framework" became a handful of tailored DOM operations
baked into your component.

This is the same philosophy as **Elm** ([doc 5](./05-elm-types.md)): a compiler
that processes your code ahead of time and produces lean output, rather than a
runtime that interprets it. Elm compiles for *type safety and dead-code
elimination*; Svelte compiles for *eliminating the runtime and the virtual DOM*.
Different goals, same insight — **move work from the browser to the build.**

---

## 8.2 What this buys: small runtime, small bundles

Because the framework compiles away, what ships is mostly *your component logic
plus a tiny shared helper runtime*, not a general-purpose rendering engine:

- There is no ~45 KB React + ReactDOM baseline to amortize. A small Svelte app's
  JavaScript can be a few KB.
- The cost scales with *your* code, not with a framework you happen to use one
  feature of.
- Updates are direct DOM writes, so there's no per-update diffing cost on the
  main thread.

This directly attacks **Tax #2 (bundle bloat)** from
[doc 7](./07-react-and-the-spa-tax.md). Svelte is one of the clearest answers to
"the framework shouldn't be the biggest thing you ship." (We'll see in §8.5 that
it does *not* escape all four taxes — only some.)

---

## 8.3 Reactivity, and how Svelte 5 became a signals system

How does Svelte know that "`count` changed → update this text node"? This is where
Svelte's story has two distinct chapters, and the second one connects straight to
[Datastar](./04-datastar-signals.md).

### Svelte 3 / 4: compiler-instrumented assignments

In early Svelte, reactivity was driven by the compiler watching **assignments**.
When you wrote `count += 1`, the compiler *rewrote* that statement to also call an
"invalidate `count`" function that schedules the dependent DOM updates. Derived
values used a special label syntax:

```svelte
let count = 0;
$: doubled = count * 2;   // recompute whenever `count` changes
```

This was elegant but a little magical — reactivity was tied to the *syntax of
assignment*, which led to sharp edges (mutating an array in place didn't trigger
an update because there was no assignment to instrument; you had to write
`arr = [...arr, x]`).

### Svelte 5: runes — explicit, fine-grained signals

Svelte 5 reworked reactivity around **runes**: explicit function-like markers that
make a value reactive. Under the hood, this is a **signals** system — the same
fine-grained reactive-primitive model as SolidJS and, conceptually, as Datastar's
signals:

```svelte
<script>
  let count = $state(0);          // a reactive signal
  let doubled = $derived(count * 2); // recomputes automatically when count changes
  $effect(() => console.log(count)); // runs when its dependencies change
</script>
```

- `$state(...)` creates a reactive value (a signal).
- `$derived(...)` is a computed value that tracks its dependencies automatically
  and recomputes only when they change.
- `$effect(...)` runs side effects when the things it reads change.

The important conceptual point: a **signal** tracks, at a fine grain, exactly
which pieces of UI read it, so a change updates *only* those pieces — no component
re-render, no diff. This is precisely the model behind Datastar's `$messageDraft`
and `data-text` expressions ([doc 4, §4.2](./04-datastar-signals.md)). The
difference is *where the reactivity lives*:

- **Datastar** puts signals in **HTML attributes**, interpreted in the browser
  (`data-signals`, `data-text`), with the server able to push signal updates over
  SSE.
- **Svelte** puts signals in a **component file**, and a *compiler* turns them
  into direct DOM-update code at build time.

Two routes to the same fine-grained reactivity: one declarative-and-server-pushed,
one compiled. The industry has broadly converged on signals as the right
reactive primitive — Svelte 5, Solid, Vue's refs, Angular signals, Preact signals,
and Datastar are all variations on it. Seeing the same idea arrive from a
compiler, a runtime library, and a set of HTML attributes is a good lesson in
*separating an idea from its implementation.*

---

## 8.4 SvelteKit: the full-stack story

Svelte the language/compiler is paired with **SvelteKit**, the application
framework: routing, server-side rendering, data loading, form actions, and
endpoints — the equivalent of Next.js for React or Remix. SvelteKit can render on
the server and **hydrate** on the client, exactly like the SPA frameworks in
[doc 7](./07-react-and-the-spa-tax.md). It also supports "form actions" that
progressively enhance ordinary HTML `<form>`s — a nod toward the hypermedia ideas
in [doc 3](./03-htmx-hypermedia.md). So Svelte spans a spectrum: you can use it for
a thin sprinkle of compiled interactivity, or for a full client-state SPA.

---

## 8.5 The honest part: which taxes Svelte avoids, and which it doesn't

It's tempting to file Svelte under "solved it." It solves *some* of the SPA taxes
and shares *others*, and being precise about which is the whole value of putting
it next to this project's architecture:

**Avoided / reduced:**
- **Bundle bloat (Tax #2):** strongly reduced — the framework compiles away, so
  there's no large runtime baseline. This is Svelte's headline win.
- **Virtual-DOM diffing cost:** eliminated entirely. Updates are direct.
- **State-management sprawl (Tax #4):** reduced — runes give a clean, built-in
  fine-grained reactivity model, so you reach for fewer external state libraries
  than a typical React app.

**Still present (because Svelte/SvelteKit is still a client-state model):**
- **The synchronization problem (Tax #1):** if your Svelte app holds authoritative
  client state and talks to a JSON API, you still have two copies of the truth to
  keep in sync — and SvelteKit apps reach for data-loading/caching patterns to
  manage it, just as React apps reach for React Query. Putting reactivity in a
  compiler doesn't change *where the truth lives*.
- **Hydration (Tax #3):** SvelteKit SSR + client hydration has the same "rendered
  twice, interactive-after-a-gap, mismatch-is-an-error" seam as any SSR framework.
- **Types:** Svelte uses TypeScript, so types are **advisory and erasable**
  ([doc 7, §7.6](./07-react-and-the-spa-tax.md)) — not the *guaranteed* safety of
  Elm. (Svelte's compiler does add real template-level checks TypeScript alone
  wouldn't, which is a genuine plus.)

So Svelte attacks the **bundle and runtime** axis brilliantly while remaining, by
default, on the **truth-in-the-browser** side of the line that
[doc 6](./06-fusion-pattern.md) and [doc 7](./07-react-and-the-spa-tax.md) draw.
That's not a criticism — it's a precise placement. The deepest cost of the SPA
model (synchronization) comes from the *location of the truth*, not the *size of
the runtime*, and that's an architectural choice Svelte leaves to you.

---

## 8.6 Where Svelte sits among the five approaches

Putting all the tools from these docs on one axis — *where does the work happen,
and where does the truth live?*

| Approach | DOM update strategy | Where reactivity/work happens | Where truth lives |
|---|---|---|---|
| **React** | Virtual DOM diff at run time | Browser runtime | Browser (+ server) |
| **Svelte** | Compiled, direct DOM ops; signals (v5) | **Build-time compiler** | Browser (+ server) |
| **Elm** | Virtual DOM, but compiled + pure | Build-time compiler (types) + runtime | Browser (`Model`), as islands here |
| **Datastar** | Morph by id; signals in attributes | Browser, **server can push** | **Server** (in the reference demo) |
| **HTMX** | Swap server-rendered HTML fragments | **Server** | **Server** |

Two independent axes are doing the work here, and Svelte is interesting precisely
because it sits at a distinctive corner:

1. **Compile-time vs run-time** — Svelte and Elm compile; React and Datastar
   interpret in the browser; HTMX needs almost no client logic at all.
2. **Truth in browser vs truth on server** — React, Svelte, and (locally) Elm lean
   browser; HTMX and the reference demo's Datastar usage lean server.

Svelte's "interesting technology" is its answer to axis 1 (compile the framework
away). This project's architecture is mostly a strong opinion about axis 2 (keep
the truth on the server). They're *orthogonal* — which is why you could absolutely
build a Svelte island, like the Elm islands here, that subscribes to the same Go
SSE stream and renders a compiled, signals-driven view of server-owned truth. The
compiler approach and the server-authoritative approach are not in tension; they
answer different questions.

---

## 8.7 What to take away

- **Svelte is a compiler, not a runtime.** It turns components into direct,
  imperative DOM-update code at build time — **no virtual DOM, no run-time
  diffing** — so the framework largely *disappears* from what ships.
- This shares Elm's "do the work at build time" philosophy, applied to a different
  goal: eliminating the runtime and shrinking the bundle.
- **Svelte 5 runes** (`$state`, `$derived`, `$effect`) are a **signals** system —
  the same fine-grained reactive primitive as Datastar's signals, Solid, and
  others. The whole industry converged on signals; seeing them arrive via a
  compiler, a library, and HTML attributes shows how an idea outlives any one
  implementation.
- Svelte strongly reduces the **bundle/runtime tax**, but a default SvelteKit app
  still lives on the **truth-in-the-browser** side, so it still faces the
  **synchronization** and **hydration** taxes ([doc 7](./07-react-and-the-spa-tax.md)).
- The two axes — *compile-time vs run-time* and *truth-in-browser vs
  truth-on-server* — are independent. Svelte is a great answer to the first; this
  project is a strong opinion about the second.

Back to the [index →](./README.md)
