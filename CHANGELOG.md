# Changelog

## v2.1.6-ms.16

Libraries rebuilt from the zen fork at commit 794be1f (linux amd64 and arm64; C header unchanged):

- Expression list functions: `sort`, `sortBy`, `minBy`, `maxBy`, `unique`, `uniqueBy`, `countDistinct`,
  `groupBy`, `keyBy`, `join`, `first`, `last`, `reduce`, `scan`.
- `executionMode: "sequential"` for decision, decision-table and expression steps (state carried item to item).
- Database node on StarRocks: every query is prepared, run and closed, because StarRocks re-executes a
  cached prepared statement containing a subquery with its first parameters (wrong rows).

## Unreleased — Rule Engine v2

Tags are `v2.1.6-ms.N` (engine 2.1.6, fork release N). Base: zen-go `v2.1.2`.
See [FORK.md](FORK.md) for files and reasons.

### Added

- `libzen_ffi` for linux amd64 and arm64 built from the MobileSentrix zen engine.
- Database node: `NewEngineWithDatabase(s)`, `DatabaseConfig`, `DatabaseSupported`,
  `DatabaseNodeUnavailableHandler`. Platforms without the new library fail database nodes with
  `ErrDatabaseNodeUnavailable`.
- Compiled-decision cache control: `zen.DecisionCache` (`InvalidateDecision`, `ClearDecisions`), an
  optional interface of the engine (type-assert it, like `RawEvaluator`); `Engine` is upstream's
  interface. They do nothing where the library has no cache.
- `DatabaseConnection.MaxLoopItems` (plug-in default 1000) bounds a loop-mode database node.
- `*Error` with `ErrorCode` (12 database config, 13 panic). `Error()` is still the engine's details
  string.
- `EvaluateRaw` and `EvaluateResult` (`RawEvaluator`, `ResultEvaluator`, `RawResult`): JSON bytes in,
  response bytes out, read from engine memory without a Go copy.
- Parallel evaluation, early database steps and the allocator of the zen fork come with the library.

### Changed

- The module path is `github.com/Mobilesentrix/ms-zen-go/v2`.
- `Dispose` is idempotent and waits for calls in flight.
- Strings containing a NUL byte are rejected, because C strings would end there.

### Fixed

- A Go panic in a loader or custom-node callback is recovered and no longer unwinds through Rust
  frames.

### Removed

- `MS-ABI.md`; `FORK.md` replaces it.
