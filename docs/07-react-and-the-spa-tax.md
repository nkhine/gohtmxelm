# 7. React and the SPA tax

> This document is the contrast case. It is **not** "React bad." React is a
> remarkable piece of engineering that solved real problems and earned its
> dominance. The goal here is to name — precisely and fairly — what the
> mainstream single-page-application (SPA) model *costs*, so you can recognize
> when you're paying that cost for no benefit. Most of the costs trace back to one
> root decision, and once you see it you can't unsee it.

---

## 7.1 The root decision: move the truth into the browser

Everything downstream flows from a single architectural choice the SPA era made:

> **The browser becomes the source of truth. The server becomes a JSON API.**

In the [fusion architecture](./06-fusion-pattern.md), the server owns the truth
and the browser holds a cache that the server refreshes. The SPA inverts this: the
client fetches JSON, builds its *own* authoritative state, renders from it, and
syncs back to the server. That inversion is powerful — and it creates four
recurring taxes. Let's take them one at a time.

---

## 7.2 Tax #1 — the synchronization problem

If the browser holds authoritative state and the server holds authoritative
state, you now have **two copies of the truth that must be kept in sync.** This is
not a minor chore; it is, empirically, the single largest source of complexity and
bugs in modern front-end apps:

- *Cache invalidation.* When the server's data changes, how does the client's copy
  find out? Entire popular libraries — React Query / TanStack Query, SWR, Apollo's
  cache, RTK Query — exist almost entirely to manage this one problem. Their
  documentation is dominated by *staleness*, *refetching*, and *invalidation*
  strategies.
- *Optimistic updates and rollback.* To feel fast, the client mutates its copy
  before the server confirms, then must reconcile or roll back on failure.
- *Race conditions.* Response B arrives before response A; which wins? You write
  request-cancellation and ordering logic by hand.

Notice that **this entire tax does not exist in the fusion app.** The client holds
no authoritative state, so there is nothing to keep in sync. "What if the server's
data changed?" is answered by *the server told everyone over SSE.* The hardest
problem in the SPA model was defined out of existence by refusing to duplicate the
truth. (Compare [doc 2's](./02-go-backend.md) hydrate-then-stream and `seq`
ordering — that *is* the sync strategy, and it lives in one place on the server.)

---

## 7.3 Tax #2 — bundle bloat and its compounding interest

To hold state and render on the client, you must ship the machinery to do so. The
baseline is not small, and it **grows with every feature**:

- React + ReactDOM: ~45 KB gzipped before you write a line.
- A router, a state library, a data-fetching/cache library, a form library, a
  date library, a component kit — each adds tens to hundreds of KB.
- Real-world SPA bundles routinely reach **several hundred KB to multiple MB** of
  JavaScript.

Why this matters more than the number suggests:

- **JavaScript is the most expensive kind of byte.** An image of the same size
  just decodes. A megabyte of JS must be **downloaded, parsed, compiled, and
  executed** on the main thread before the app is interactive — and on a mid-range
  phone that is seconds, not milliseconds. The cost is CPU, not just bandwidth.
- **It compounds.** Every feature, every dependency, adds to the bundle the *next*
  user must download. There is a ratchet: bundles rarely shrink.

Contrast the fusion app: **HTMX (~14 KB) and Datastar load once and are cached
forever. Adding a feature adds a server route and a template — the client bundle
does not grow at all.** Elm is shipped only as islands, only where complex client
logic genuinely lives — and Elm's compiler is aggressive about dead-code
elimination, so an Elm island is typically smaller than the equivalent React
component tree plus its share of the framework. The asymmetry is structural:
server-rendered approaches put incremental cost on the server (cheap, shared,
upgradeable); client-rendered approaches put it on every visitor's device.

---

## 7.4 Tax #3 — hydration, the awkward seam

To improve first-paint, SPAs server-render the initial HTML (SSR) and then
**hydrate** it: ship the same component tree as JavaScript, re-run it on the
client, and attach event listeners to the already-rendered DOM. This creates a
genuinely awkward seam:

- The work is, in a sense, **done twice** — rendered on the server, then re-run on
  the client to "claim" the markup.
- Between HTML-visible and JS-hydrated, the page **looks ready but isn't** — clicks
  do nothing, inputs drop keystrokes. This gap is a documented UX hazard.
- A **mismatch** between server-rendered and client-rendered output (a stray date,
  a `Math.random`, a locale difference) throws hydration errors and can blow away
  the server HTML.
- The entire frontier of "React Server Components," streaming SSR, and selective/
  progressive hydration is, fundamentally, **an industry-wide effort to manage
  complexity that the architecture created in the first place.**

In the fusion app there is **no hydration step**. HTMX and Datastar enhance
server-rendered HTML that is *already* interactive via attributes — there's no
second render to reconcile. Elm islands mount independently and own their own
small DOM, so there's no whole-page hydration handshake to get wrong. The seam
doesn't exist because the page was never split into "the HTML" and "the JS that
must reclaim the HTML."

---

## 7.5 Tax #4 — state-management sprawl

Because client state is central *and* React gives you only a low-level primitive
(`useState`/`useReducer`) plus an escape hatch (`useEffect`), real apps accrete
layers to tame it:

- Local state: `useState`, `useReducer`.
- Cross-component state: Context, or a store — Redux, Zustand, Jotai, Recoil,
  MobX, Valtio…
- Server-cache state: React Query, SWR, RTK Query, Apollo…
- Form state: React Hook Form, Formik…
- And the connective tissue: `useEffect` dependency arrays, memoization
  (`useMemo`, `useCallback`), and the perennial question *why did this re-render?*

None of these are bad libraries. But step back: a large amount of effort goes into
**managing state that only needs to be client-side because the architecture put it
there.** `useEffect` in particular is a notorious foot-gun — it conflates "sync
with an external system," "fetch data," and "respond to a change," and its
dependency-array model causes stale closures, double-fires, and infinite loops
that even experienced developers hit.

Compare the discipline elsewhere in this project:

- **HTMX**: there is essentially no client state to manage. The server holds it.
- **Datastar**: local state is *signals*, declared in markup; the reactive graph
  is automatic and fine-grained — no dependency arrays, no manual memoization, no
  "why did this re-render."
- **Elm**: state is *one immutable `Model`*, changed *only* in `update`, with side
  effects as returned `Cmd` values. The pattern Redux imitates is here built into
  the language and enforced by the compiler. There is exactly one place state
  changes and one place effects are described. (See [doc 5](./05-elm-types.md).)

The contrast isn't "React has no pattern." It's that React leaves the pattern to
you and the ecosystem, so each app reinvents it — usually as several
half-overlapping libraries — whereas these tools either remove the state or impose
one disciplined model.

---

## 7.6 Types: advisory vs guaranteed

The front-end's answer to "too many runtime type bugs" was TypeScript, and it's a
real improvement. But it's worth being precise about what it does and doesn't give
you, because it's a different *kind* of guarantee than Elm's
([doc 5](./05-elm-types.md)):

- TypeScript types are **erased at build time** and **advisory**. They constrain
  what you *write*, not what *runs*. `any`, type assertions (`as`), non-null
  assertions (`!`), untyped third-party libraries, and `JSON.parse` returning
  `any` are all routine holes through which unsoundness — and `undefined` — re-
  enters at runtime.
- Elm has **no escape hatch**: no `any`, no `null`/`undefined`, no exceptions, no
  casts. Foreign data must pass through a **decoder** at the boundary
  ([doc 5, §5.4](./05-elm-types.md)) and emerge well-typed or as a handled
  default. The guarantee is *total*, which is why "no runtime exceptions" is a
  property Elm actually has and TypeScript cannot promise.

This isn't a verdict that TypeScript is bad — for incrementally typing a large JS
codebase inside the npm ecosystem, it's the pragmatic and correct choice. It's a
reminder that "we have types" spans a wide range of strength, and that *where
correctness really matters*, a guaranteed type system (applied as islands) buys
something an advisory one cannot.

---

## 7.7 Where React is genuinely the right tool

A fair document names this clearly. Reach for React (or a similar SPA framework)
when:

- The app is a **genuinely stateful client application** — a design tool, an IDE,
  a spreadsheet, a complex builder — where the *interesting state is inherently in
  the browser* and a server round-trip per interaction would be absurd.
- You need **offline-first / local-first** behaviour, where the client *must* be a
  source of truth.
- The interaction is **highly stateful and latency-sensitive** in ways that local
  reactivity serves far better than the network (rich drag-and-drop, live
  multi-element manipulation).
- You have a large team and ecosystem investment where React's gravity — hiring,
  libraries, tooling — is itself a decisive practical advantage.

Notice these are the *same* cases where the reference demo reaches for **Elm's local
state**. The boundary the fusion app draws inside one page — request/response vs
server-push vs genuinely-client-stateful — is the *same* boundary you should draw
when deciding whether a whole app wants HTMX-style server rendering or a React-
style SPA. React's mistake was never *existing*; it was being applied by default
to the enormous majority of apps that are mostly CRUD, forms, and pages — paying
all four taxes above for interactivity those apps could get from hypermedia and a
sprinkle of signals.

---

## 7.8 The honest summary

| Concern | Mainstream SPA (React-style) | The reference demo's approach |
|---|---|---|
| Source of truth | Browser (+ server) → must sync | Server only; client is a cache |
| State sync | Major problem; whole libraries for it | Doesn't exist — SSE broadcasts truth |
| Client JS | Grows per feature; 100s of KB–MBs | Fixed small core; grows on the server |
| Hydration | Required; an awkward, error-prone seam | None |
| Local state mgmt | Many overlapping libraries; `useEffect` traps | None (HTMX) / signals (Datastar) / one `Model` (Elm) |
| Types | Advisory, erasable, escape hatches | Guaranteed where it matters (Elm islands) |
| Best fit | Genuinely client-stateful apps | CRUD, forms, pages, dashboards, server-push |

The takeaway is not "abandon React." It is: **the SPA model is one point on a
spectrum, optimized for genuinely client-stateful apps, and it was over-applied to
apps that aren't.** When you keep the truth on the server and give the browser a
cache plus bounded, well-chosen enhancement, an entire constellation of
problems — sync, bundle growth, hydration, state sprawl — simply doesn't arise.
That isn't nostalgia for the old web; it's a deliberate, modern architecture that
this little four-framework app demonstrates is not only viable but, for most of
what we build, *better*.

---

## 7.9 What to take away

- The SPA's costs nearly all descend from one choice: **putting the source of
  truth in the browser.**
- That choice creates **state synchronization** as a first-class problem — the
  thing most front-end complexity and tooling exists to manage.
- It forces an ever-growing **client bundle**, the most expensive kind of byte
  (download + parse + compile + execute on the main thread).
- It requires **hydration**, an awkward and error-prone seam, and much recent
  framework work exists to paper over it.
- It pushes **state-management sprawl** and `useEffect`-class hazards onto every
  team.
- **TypeScript is advisory**; a guaranteed type system (Elm, as islands) gives a
  stronger property where correctness is critical.
- React remains right for genuinely client-stateful apps — the *same* boundary
  this project draws between its surfaces. The discipline is to **draw that
  boundary deliberately** instead of defaulting the whole app to the client.

Next: [Svelte — the framework that compiles itself away →](./08-svelte-the-compiler.md)
