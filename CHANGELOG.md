# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the Go API follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The on-the-wire broker format is versioned independently of the Go API by
`ProtocolVersion`; a change islands or the broker must interpret differently
bumps that constant rather than the module version.

## [Unreleased]

## [1.0.0] - 2026-06-16

First stable release. The exported API of the `github.com/nkhine/gohtmxelm`
package is now committed under semantic versioning (see
[STABILITY.md](STABILITY.md)): additive within the 1.x line, breaking changes
only on a future 2.0. The wire format remains versioned independently by
`ProtocolVersion`.

### Changed

- **Breaking:** the `simnet` simulation harness moved to `internal/simnet` and
  is no longer importable. It is a testing/demonstration aid, not part of the
  integration API; keeping it out of the public surface keeps the 1.0 promise
  minimal. (It may return as a public, versioned package later — that would be
  additive.) The integration API is otherwise unchanged from 0.2.0.

### Added

- A written stability policy ([STABILITY.md](STABILITY.md)) defining the three
  tracks: the Go API (semver), the wire format (`ProtocolVersion`), and the
  no-promise surfaces (the `gohtmxelm` CLI, `internal/`, and the `demo/`).
- A Go CI workflow (`.github/workflows/ci.yml`): `templ generate`, gofmt, `go
  vet`, and `go test ./... -race` on every push and pull request.
- Runnable examples for the remaining public helpers (`LocalizedProps`,
  `InteractionOpenAttrs`, `WriteDatastarPatchSignals`, `ElmBrokerPort`).
- MIT `LICENSE` (© Norman Khine) and a License section in the README.
- The `docs/` notes are now published as a site built with
  [Zensical](https://zensical.org) and deployed to GitHub Pages by
  `.github/workflows/docs.yml` (on pushes touching `docs/**` or `zensical.toml`).
  Added a Getting Started page (CLI scaffolder + deploy), a `zensical.toml` with
  grouped navigation, and converted cross-directory source links to absolute
  GitHub URLs so they resolve on the published site.

### Fixed

- Corrected a stale docs link to `BrokerPort.elm` (moved to the repository root
  in 0.1.0) and a lingering `pkg/` package reference in the docs index.

## [0.2.0] - 2026-06-16

A CLI release: `gohtmxelm init` becomes a real scaffolder, with optional deploy
scaffolding and a polished terminal experience. The Go API and wire contract are
unchanged from 0.1.0.

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
- `gohtmxelm deploy` (and `gohtmxelm init --deploy`) emit optional, template-only
  deploy scaffolding: a multi-stage distroless `Dockerfile`, `.dockerignore`,
  `docker-compose.yml`, a GitHub Actions workflow that builds/tests and pushes
  the image to GitHub Container Registry via the built-in `GITHUB_TOKEN`, and a
  `DEPLOY.md` documenting the SSE-specific proxy/timeout/HTTP-2 considerations.
  Full and minimal scaffolds get matching variants. Nothing is ever deployed and
  no credentials are handled.
- The CLI now has structured help (`gohtmxelm help`, `gohtmxelm help init`,
  `init -h`) and a polished init experience: a banner, grouped phases (Creating
  files / Installing dependencies / Building assets), and an animated spinner
  with ✓/✗ status per step. Output degrades to plain, ANSI-free lines when
  stdout is not a terminal or `NO_COLOR` is set, so logs and CI stay clean.

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

[Unreleased]: https://github.com/nkhine/gohtmxelm/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/nkhine/gohtmxelm/compare/v0.2.0...v1.0.0
[0.2.0]: https://github.com/nkhine/gohtmxelm/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/nkhine/gohtmxelm/releases/tag/v0.1.0
