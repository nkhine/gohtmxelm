# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the Go API follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The on-the-wire broker format is versioned independently of the Go API by
`ProtocolVersion`; a change islands or the broker must interpret differently
bumps that constant rather than the module version.

## [Unreleased]

### Added

- `gohtmxelm init` now scaffolds a complete, runnable project instead of writing
  an unused config file. In an empty directory it generates a chi + templ server,
  an SSE-backed `Broadcaster`, and a sample Elm island wired through the vendored
  `BrokerPort` contract, then runs `go get` / `templ generate` / `elm make` to
  leave a buildable app. Flags: `--minimal` (SSE-only, no Elm/build step),
  `--module`, `--no-build`, `--force`.
- Run inside an existing Go module and `init` adds a self-contained, mountable
  `gohtmxelmkit/` package (and prints the chi wiring snippet) without touching
  the host `main.go`.
- `gohtmxelm vendor-elm [dir]` (re)writes `BrokerPort.elm` to re-sync the Elm
  contract after a library upgrade.
- `gohtmxelm doctor` now distinguishes required (`go`) from optional tools.

## [0.1.0] - 2026-06-16

First tagged release. The Go API is now importable at the module root.

### Added

- `ElmBrokerPort()` and `ElmContractHandler()` expose the canonical, embedded
  `BrokerPort.elm` so importers can vendor an Elm-side contract that matches the
  broker they run, instead of hand-copying a file that silently drifts.
- A package test enforces that `ProtocolVersion` (Go), `PROTOCOL_VERSION`
  (broker runtime), and `BrokerPort.protocolVersion` (Elm) agree, so bumping the
  constant forces all three copies in step.
- Package documentation (`doc.go`) and runnable, output-verified examples
  (`ExampleNew`, `ExampleElmIsland`, `ExampleStream`, `ExampleBroadcaster`).

### Changed

- **Breaking (import path):** the package moved from
  `github.com/nkhine/gohtmxelm/pkg` to the module root
  `github.com/nkhine/gohtmxelm`; `pkg/simnet` moved to `simnet`. Update imports
  and drop the `gohtmxelm "…/pkg"` alias.
- The canonical `BrokerPort.elm` now lives at `elm/BrokerPort.elm` (embedded);
  the demo sources it via `../elm` so there is a single source of truth.

### Public API

- **Streaming:** `Stream`, `NewStream`, `Serve`, `Broadcaster`, `NewBroadcaster`,
  `PrepareSSE`, `WriteSSE`, `Flush`, `ErrStreamingUnsupported`.
- **Datastar:** `WriteDatastarPatchElements`, `WriteDatastarPatchElementsMode`,
  `WriteDatastarPatchSignals` (and `Stream.PatchElements`/`PatchElementsMode`/
  `RemoveElements`/`PatchSignals`/`Ping`).
- **HTMX:** `Trigger`, `NoContent`.
- **Browser glue:** `New`, `Kit`, `Options`, `Source`, `Assets`, `BrowserScript`,
  `ElmIsland`, `InteractionRoot`, `InteractionScript`, `InteractionOpenAttrs`,
  `InteractionResultAttrs`, `InteractionEvent`, `MarshalInteractionEvent`.
- **Wire contract:** `Envelope`, `ProtocolVersion`, `Type*`, `Target*`,
  `ElmBrokerPort`, `ElmContractHandler`.
- **Localization:** `MessageCatalog`, `LocaleProps`, `LocalePropsFrom`,
  `LocalizedProps`.
- **`simnet`** is shipped as a testing aid and is not yet part of the stability
  promise.

[Unreleased]: https://github.com/nkhine/gohtmxelm/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/nkhine/gohtmxelm/releases/tag/v0.1.0
