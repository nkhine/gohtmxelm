// Package gohtmxelm wires a Go server to browser islands built with HTMX,
// Datastar, and Elm over a single versioned message contract.
//
// The package is deliberately small: it owns the plumbing that every
// Go+HTMX+Elm app would otherwise reimplement — server-sent events, Datastar
// patches, the broker envelope, asset mounting, and the HTML conventions that
// connect them — and leaves application policy (routing, storage, auth,
// copy, catalogue loading) to the host.
//
// # The wire contract
//
// One envelope shape crosses three languages. Go constructs and validates it
// with [Envelope]; the embedded broker runtime speaks it in JavaScript; and
// Elm islands speak it through the BrokerPort module returned by
// [ElmBrokerPort]. All three stamp and check [ProtocolVersion]; a test in this
// package fails if the three copies disagree. Bumping ProtocolVersion is a
// deliberate, breaking change to the wire format.
//
// # Server-sent events
//
// [Stream] bundles the ResponseWriter, its Flusher, and the request context so
// handlers stop repeating the type-assert/set-headers/flush ritual:
//
//	s, err := gohtmxelm.NewStream(w, r)
//	if err != nil {
//		http.Error(w, err.Error(), http.StatusInternalServerError)
//		return
//	}
//	s.Send("update", payload)
//
// [Serve] runs the common subscribe/hydrate/forward lifecycle against a
// [Broadcaster], a thread-safe fan-out hub that slow subscribers cannot block.
//
// # Browser integration
//
// [New] returns a [Kit] configured from [Options]. Mount [Kit.Assets] under a
// stable prefix, render [Kit.BrowserScript] (and [Kit.InteractionScript] for
// the overlay convention) into your pages, and mount Elm islands with
// [ElmIsland]. [BrowserScript] serialises the SSE [Source] list the broker
// connects to on boot.
//
// # Localization
//
// [LocaleProps] is a neutral locale payload for island flags. [LocalePropsFrom]
// builds it from a host [MessageCatalog], and [LocalizedProps] merges it with
// application-specific props. The library owns no catalogue loading, fallback,
// or pluralisation policy.
//
// # Stability
//
// The Go API and the wire contract are versioned independently: the Go surface
// follows module semver, while the on-the-wire format is gated by
// [ProtocolVersion]. The simnet subpackage is a testing aid and is not part of
// the stability promise yet.
package gohtmxelm
