# 12. Local SSO Login Simulator

The local SSO demo shows the browser mechanics of an SSO redirect flow without
requiring Alcove, Cognito, Dex, or a real external identity provider.

It is intentionally small:

- `/examples/sso-local`: the demo card.
- `/api/sso/start`: creates state, sets an HttpOnly state cookie, and redirects
  to the local identity-provider form.
- `/api/sso/idp/login`: a fixture IdP approval page.
- `/api/sso/callback`: validates state and one-use code, sets an HttpOnly
  session cookie, and redirects back to the demo.
- `/api/sso/session`: renders the current session/claims fragment.
- `/api/sso/logout`: clears the session.
- `/api/auth/presence-dot`: renders the small red/orange/green HTMX presence
  dot used in every card header.
- `/api/auth/presence-stream`: streams Datastar signal patches for the richer
  presence panel on the SSO card.
- `/api/stream`: hydrates and broadcasts the `auth-presence` broker event for
  Elm and HTMX.

The fixture users mirror Alcove's local Dex examples:

| Email | Role profile |
|---|---|
| `org-admin@customer-a.local` | `OrgAdmin`, `ProjectAdmin` |
| `deal-reader@customer-a.local` | `DealReader` |
| `platform-admin@alcove.local` | `PlatformAdmin` |

## What It Proves

The point is not to reimplement an identity platform. The point is that the
Go-first pattern handles login boundaries cleanly:

- Login starts as a normal browser navigation, not an SPA fetch.
- The callback is server-owned and writes an HttpOnly cookie.
- HTMX can rehydrate the signed-in panel from `/api/sso/session`.
- The same server-owned auth state can fan out to every UI style:
  HTMX refreshes header dots, Datastar patches `authPresence`/`authEmail`
  signals, and Elm receives a typed `auth-presence` broker event.
- The rest of the app stays server-authoritative.

The local IdP page uses an "Approve sign-in" button instead of a password field
because the demo app should not appear to own external credentials. In a real
deployment, that page would belong to Entra, Okta, Dex, or another IdP.

## Auth Presence

The demo keeps presence separate from SSO mechanics in
`demo/internal/presence`. A successful callback calls `Online(email)`, logout
calls `Logout()`, and session activity calls `Touch()`.

Presence has three states:

| State | UI colour | Meaning |
|---|---|---|
| `online` | Green | The local SSO callback created a session. |
| `idle` | Orange | The user is still signed in, but no session activity has happened for the idle window. |
| `offline` | Red | There is no active demo session. |

The tracker publishes each state change through the same `gohtmxelm.Broadcaster`
pattern as the other demo domains. That keeps the implementation consistent:
server truth first, then HTMX/Datastar/Elm converge from the event stream.

## Boundary With Alcove

Alcove's local rig is broader: floci, cognito-local, Dex runtime, seeded identity
data, Lambda handler adapters, and auth/session machinery. This repo's demo
copies the shape of the flow, not Alcove internals.

That keeps `make dev` lightweight. It starts only the Go demo server; no floci,
Docker sidecars, or Pulumi stack are required for this SSO simulator.
