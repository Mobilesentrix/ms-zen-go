# MobileSentrix fork of gorules/zen-go

Module `github.com/Mobilesentrix/ms-zen-go/v2`, based on upstream zen-go `v2.1.2`. Tags are
`v2.1.6-ms.N`: engine 2.1.6 (the MobileSentrix fork of gorules/zen), fork release N.

Upstream ships new engine versions by replacing `deps/*/libzen_ffi.a`; its Go sources for 2.1.6
are the v2.1.2 sources. The C header the Go code compiles against is upstream's
`bindings/c/zen_engine.h` of `go-v2.1.6` plus additions only (no upstream declaration changed),
so every upstream call keeps its signature. The C contract of the additions is `MS-FFI.md` in the
zen fork.

## What it adds, and why

| Change | Files | Reason |
|---|---|---|
| `deps/linux_amd64`, `deps/linux_arm64`: libzen_ffi built from the zen fork for every release | `deps/`, `deps/PROVENANCE`, `zen_engine.h`, `cgo.go` | Database node, decision cache and the engine changes listed in the zen fork's FORK.md |
| `NewEngineWithDatabase(s)`, `DatabaseConfig`, `DatabaseSupported`, `DatabaseNodeUnavailableHandler` | `database*.go` | Rules run database nodes natively in the engine; other platforms fail those nodes with `ErrDatabaseNodeUnavailable` |
| `DecisionCache` (`InvalidateDecision` / `ClearDecisions`), an optional interface of `Engine` like `RawEvaluator` | `decision_cache_*.go`, `zen.go` | The engine caches compiled decisions per key; the embedder says when a rule changed (no-ops where the lib has no cache). `Engine` itself is upstream's interface |
| `*Error` with `ErrorCode` (12 database config, 13 panic); `Error()` is the engine's details string unchanged | `errors.go` | Callers can branch on the code and still parse the message |
| `EvaluateRaw` / `EvaluateResult` (`RawEvaluator`, `ResultEvaluator`, `RawResult`) | `raw.go` | The context goes in as JSON bytes and the response comes out as the engine wrote it; `RawResult` reads it from engine memory without a Go copy |
| Go panics in loader/custom-node callbacks are recovered; `Dispose` is idempotent and waits for in-flight calls (so a loader or handler must not call `Dispose` on the engine that called it); strings with a NUL byte are rejected | `engine.go`, `decision.go` | A panic must not unwind through Rust frames; no use of a freed engine; C strings would be truncated at the NUL |
| Module path `github.com/Mobilesentrix/ms-zen-go/v2` | `go.mod`, examples | Published separately from upstream |

## Supported platforms and libraries

linux amd64 and linux arm64 are the supported platforms: their `libzen_ffi.a` is rebuilt from
the zen fork for every release. `deps/darwin_*` and `deps/windows_amd64` hold upstream
gorules/zen-go v2.1.2's libraries, unchanged: engine 2.1.2 without the fork's changes (no
database node, no decision cache). They let the module compile on those platforms for local
work and are not supported for production. `deps/PROVENANCE` records, for every library file,
where it was built from, with which compiler and target, and its sha256.

## Build

The libs are built in the zen fork (`MS-BUILD.md`: `cargo build --release -p zen-ffi` for
`x86_64-unknown-linux-gnu` and `aarch64-unknown-linux-gnu` in `rust:1-bookworm`, then
`strip --strip-debug`). Copy `libzen_ffi.a` into `deps/linux_amd64/` and `deps/linux_arm64/`
and `bindings/c/zen_engine.h` to `zen_engine.h`. Link in an image with glibc >= 2.36.

```bash
CGO_ENABLED=1 go test ./...          # linux, cgo
```

## Updating from upstream

1. Merge or rebase onto the new upstream zen-go tag. Upstream releases mostly change only `deps/`:
   keep the linux libs from the zen fork, take darwin/windows from upstream.
2. Diff upstream's new `zen_engine.h` against the zen fork's header: the fork must add
   declarations only. Rebase the zen fork first if upstream changed the C API.
3. `go vet ./...` and `go test ./...` on linux amd64 (and arm64), then tag `v<engine>-ms.<N>`.
