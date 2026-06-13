# 5. Elm: types, the update loop, and impossible states

> HTMX and Datastar reduce the *amount* of client logic. Elm is for the cases
> where client logic is genuinely necessary and you want it to be **correct**.
> Elm is a pure functional language that compiles to JavaScript and is famous for
> a startling claim it actually delivers on: **no runtime exceptions.**

---

## 5.1 Why a typed language changes what bugs are possible

Most front-end bugs are not exotic. They are:

- `undefined is not a function`
- reading a field off `null`
- forgetting to handle the "loading" or "error" case
- a value that's a string here and a number there

Every one of these is a **type error** that a sufficiently strict compiler can
catch before the code ever runs. JavaScript and TypeScript catch *some*. Elm's
type system, combined with the language having **no `null`, no `undefined`, and no
exceptions**, catches essentially *all* of this category. When an Elm program
compiles, an enormous class of bug is already gone — not "less likely," gone.

This is not a small quality-of-life gain. It changes how you work: refactoring
becomes fearless (the compiler finds every site you must update), and "it
compiles" carries real information.

---

## 5.2 The Elm Architecture (TEA): the pattern everyone copied

Every Elm program is the same three pieces. You will recognize them because Redux,
the Elm-inspired state container, popularized a watered-down version across the
React world:

```elm
type alias Model = { ... }              -- ALL state, in one immutable value

type Msg = SomethingHappened | ...      -- every possible event, enumerated

update : Msg -> Model -> (Model, Cmd Msg)   -- the ONLY way state changes
view   : Model -> Html Msg                   -- a pure function: state → DOM
```

The rules are strict and that's the point:

- **State is one immutable value** (`Model`). There is no scattered, mutable state
  hiding in component instances.
- **State changes only in `update`.** Every change flows through one function as a
  `Msg`. There is exactly one place to look to understand how anything changes.
- **`view` is pure.** Same `Model` in, same HTML out, every time. No side effects,
  no surprises.
- **Side effects are values** (`Cmd`), returned from `update` and performed by the
  Elm runtime — not executed inline. Talking to the outside world is explicit and
  controlled.

Compare this to a typical React component with several `useState` hooks, a couple
of `useEffect`s, and some refs: state is spread across hooks, changes happen in
many handlers, and effects fire as a side consequence of rendering. TEA takes the
*good idea* React's ecosystem reached for with Redux and bakes it into the
language, enforced by the compiler.

---

## 5.3 Making impossible states unrepresentable

This is Elm's signature technique, and this app demonstrates it twice. The idea:
**design your types so that invalid states cannot even be written down.** If a
state is unrepresentable, no code — not even buggy code — can produce it, and you
never need to handle it defensively.

### Example 1: the typed draft editor (App A)

[`demo/elm/AppA.elm`](../demo/elm/AppA.elm) is a message editor. A draft can be empty, too
long, or valid. Instead of tracking that with a string plus a couple of booleans
(which permits nonsense like "empty *and* valid"), it uses a single type with one
constructor per legal state:

```elm
type Draft
    = Empty
    | TooLong Int
    | Valid String

classifyDraft : String -> Draft
classifyDraft raw =
    let trimmed = String.trim raw in
    if String.isEmpty trimmed then Empty
    else if String.length trimmed > maxDraftLength then TooLong (String.length trimmed)
    else Valid trimmed
```

Now watch what this buys in `update`:

```elm
SubmitDraft ->
    case classifyDraft model.draft of
        Valid trimmed ->
            ( { model | draft = "" }
            , sendStateSet brokerOut "broadcast" "message" (Encode.string trimmed)
            )
        _ ->
            ( model, Cmd.none )  -- structurally impossible to send an invalid draft
```

A write to the server can **only** be produced in the `Valid` branch — and the
`Valid` constructor *carries the validated string*. There is no code path that
sends an empty or over-long message, because the only place a message-to-send
exists is inside `Valid`. The "Save" button is also disabled unless the draft is
`Valid`, but even if that UI guard were wrong, the type makes the bad write
impossible. **The compiler enforces the business rule.** That's a different kind
of safety than a runtime `if (valid) { ... }` check you might forget.

### Example 2: lap analytics with no sentinel values (LapStats)

[`demo/elm/LapStats.elm`](../demo/elm/LapStats.elm) computes fastest/slowest/average lap
times. The naive version returns zeros when there are no laps — and then every
reader has to remember that "0" might mean "no data." Elm models the absence
explicitly:

```elm
type Stats
    = NoLaps
    | Stats { count : Int, fastest : Int, slowest : Int, average : Int, lastDelta : Maybe Int }
```

The `view` is then forced by the compiler to handle both cases — you literally
cannot render the stats without first deciding what "no laps" looks like:

```elm
case computeStats model.laps of
    NoLaps -> div [ class "elm-hint" ] [ text "No laps recorded..." ]
    Stats s -> div [ class "lap-stats-grid" ] [ stat "Fastest" (formatMs s.fastest), ... ]
```

And `lastDelta : Maybe Int` encodes "there might not be a previous lap to compare
to" *in the type*. A `Maybe` must be unwrapped (`case ... of Just d -> ... ;
Nothing -> ...`) before use — you cannot forget the empty case, because the
compiler won't let you. There is no `null` to slip through.

This is the deep lesson: **push correctness into the type system so the compiler
checks it, instead of relying on yourself to remember runtime checks.** A whole
genre of "we forgot to handle X" bugs simply cannot occur.

---

## 5.4 Ports: Elm's airlock to the messy outside world

Elm is pure and sealed — so how does it talk to a JavaScript broker, `fetch`, or
SSE? Through **ports**: typed, asynchronous message channels that are the *only*
way data crosses the Elm/JS boundary.

```elm
port brokerOut : Encode.Value -> Cmd msg          -- Elm → JavaScript
port brokerIn  : (Decode.Value -> msg) -> Sub msg -- JavaScript → Elm
```

- `brokerOut` sends a JSON value out to JS as a **command**. Elm doesn't perform
  the side effect; it hands a value to the runtime.
- `brokerIn` **subscribes** to messages coming in from JS, turning each into a
  `Msg` that flows through `update` like any other event.

Crucially, **data coming in from JS is untrusted and must be decoded.** Elm won't
let you assume the shape of foreign JSON; you write a decoder that either produces
a well-typed value or a typed error. From [`demo/elm/BrokerPort.elm`](../demo/elm/BrokerPort.elm):

```elm
decodeStoreChange : Decode.Decoder StoreChange
decodeStoreChange =
    Decode.map4 StoreChange
        (Decode.at [ "payload", "key" ] Decode.string)
        (Decode.oneOf [ Decode.at [ "payload", "value" ] Decode.string, Decode.succeed "" ])
        (Decode.oneOf [ Decode.at [ "payload", "source" ] Decode.string, Decode.succeed "unknown" ])
        (Decode.oneOf [ Decode.at [ "payload", "deleted" ] Decode.bool, Decode.succeed False ])
```

The `oneOf [ ..., Decode.succeed default ]` pattern says: "try to read this field;
if it's missing or the wrong type, fall back to this default." The boundary is
where reality is messy, so the boundary is exactly where Elm forces you to be
explicit about every field and every fallback. **Bad data from JS becomes a
handled default, never a crash.** The port + decoder pair is an *airlock*:
untyped chaos goes in one side, only well-typed values come out the other.

---

## 5.5 Elm islands: typed cores in an un-typed page

Elm doesn't own the whole page here. It runs as **islands** — small `Browser.element`
apps mounted into specific `<div>`s, each managed by the broker
([`demo/static/broker.js`](../demo/static/broker.js)) which calls `Elm.AppA.init({ node, flags })`.
Three islands run: the draft editor (App A), an event log (App B), and the lap
analyzer (LapStats). Each is a self-contained TEA program with its own `Model`,
`update`, and `view`.

This "islands" approach is the pragmatic sweet spot: you get Elm's guarantees
*exactly where client logic is complex enough to deserve them*, without forcing
the entire page through one framework. The rest of the page is HTMX and Datastar.
Elm earns its bundle only on the surfaces that genuinely need a typed state
machine. (How the islands coexist with the other tools without conflict is the
subject of doc 6.)

---

## 5.6 What about TypeScript?

A fair question: TypeScript also adds types to the front-end. Why Elm? The honest
comparison:

- TypeScript types are **erased** and *advisory*. They describe what JavaScript
  *should* do but don't change its runtime; `any`, casts, and untyped libraries
  let unsoundness leak in, and `null`/`undefined` are everywhere unless you fight
  them.
- Elm has **no escape hatch**. No `any`, no `null`, no exceptions, no casting your
  way around the checker. The guarantee is total, which is why "no runtime
  exceptions" is a real property and not a slogan.

TypeScript is the right call when you must live inside the JS ecosystem and adopt
types incrementally — which is most teams. Elm is the right call when a piece of
UI is important enough that you want *guarantees*, not *suggestions*. Using Elm as
islands lets you spend that stricter (and less ecosystem-compatible) tool precisely
where it pays off, and use lighter tools elsewhere.

---

## 5.7 What to take away

- Elm's type system + **no null/undefined/exceptions** eliminates the most common
  category of front-end bug *at compile time*. "It compiles" means something.
- **The Elm Architecture**: all state in one immutable `Model`, all changes
  through one `update`, a pure `view`, side effects as returned `Cmd` values.
  Redux is the diluted, unenforced copy.
- **Make impossible states unrepresentable**: model legal states as a type
  (`Draft`, `Stats`) so invalid ones can't be written, and the compiler enforces
  your rules (e.g. you cannot send an invalid draft).
- **`Maybe` instead of null** forces you to handle absence; you can't forget it.
- **Ports + decoders** are a typed airlock to JavaScript: untrusted data is
  decoded into well-typed values or handled defaults — never a crash.
- **Islands** let you apply Elm's strictness only where client logic is complex
  enough to deserve it.

Next: [how all three coexist — the fusion pattern →](./06-fusion-pattern.md)
