# Stability policy

`gohtmxelm` versions three things on independent tracks. Knowing which track a
change belongs to tells you whether it can break you.

## 1. The Go API — semantic versioning

The exported surface of the `github.com/nkhine/gohtmxelm` package is the public
API. It follows [Semantic Versioning](https://semver.org):

- **Patch** (`v1.0.x`) — bug fixes, no API change.
- **Minor** (`v1.x.0`) — additive only: new exported symbols, new optional
  fields on existing option structs. Existing code keeps compiling.
- **Major** (`vX.0.0`) — anything that can break a compile or change documented
  behaviour.

Within a major version, upgrading is safe. Deprecations are announced in the
CHANGELOG and in doc comments (`Deprecated:`) at least one minor release before
removal in the next major.

## 2. The wire format — `ProtocolVersion`

The broker envelope crosses three languages (Go `Envelope`, the JavaScript
broker runtime, and the Elm `BrokerPort` contract). It is versioned by the
`ProtocolVersion` constant, **independently of the module version**. A change
that islands or the broker must interpret differently bumps `ProtocolVersion`; a
test fails if the three copies disagree. Re-vendor `BrokerPort.elm`
(`gohtmxelm vendor-elm`) after any upgrade that changes it.

## 3. Things with no stability promise

- **`cmd/gohtmxelm`** — a development tool. Its flags and output may change in
  minor releases. The code it scaffolds is a starting point you own and edit;
  it is not part of the API surface.
- **`internal/`** — including the `simnet` simulation harness. Not importable by
  design and may change at any time. (`simnet` may be promoted to a public,
  versioned package later — that would be an additive change.)
- **The `demo/` application** — a reference example, not a supported artifact.

## Supported Go version

The minimum Go version is the one declared in `go.mod`. It may be raised in a
minor release when a new language or standard-library feature is adopted; this
is not treated as a breaking change.
