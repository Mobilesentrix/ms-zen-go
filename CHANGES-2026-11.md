# Rules Engine changes, 7–12 November 2026 (ms-zen-go)

This one commit replaces PRs #2, #3 and #4 on `master`. It holds the rebuilt Linux `libzen_ffi`
libraries, one new Go test file, and the CHANGELOG and PROVENANCE updates. The code is identical
to the old merge `ae36a7e3` (release `v2.1.6-ms.17`), which is what was deployed. We collapsed the
three PRs into one commit so the history shows one reviewable change instead of a release, its
revert and its re-release.

## A note to the Rules Engine team

These PRs were merged with admin rights by the PO Plan owner (`wjavedMS`) for the PO Plan release.
They did not get your review. That was not the right process, and we are sorry for it. This file
explains each change so you can review it now. If you want anything changed or reverted, say so
and we will do it.

Merge times (US Eastern): PR #2 on Fri 6 Nov 2026, 22:26. PRs #3 and #4 on Sat 7 Nov 2026, 11:16.

## Changes

The Go code of the binding did not change. Every engine change below arrives through the two
rebuilt static libraries (`deps/linux_amd64/libzen_ffi.a`, `deps/linux_arm64/libzen_ffi.a`). They
were built from the MobileSentrix zen fork at commit `5f2abea` (branch `release/engine-2026-11-08`;
PR #4 says it was merged to zen `master` as `1f89872` in zen PR #12 with an identical tree).
The engine source changes themselves live in the zen fork, not in this repo.

### 1. Expression list functions

- **What changed:** the expression language gains `sort`, `sortBy`, `minBy`, `maxBy`, `unique`,
  `uniqueBy`, `countDistinct`, `groupBy`, `keyBy`, `join`, `first`, `last`, `reduce`, `scan`.
  Implemented in the zen fork; exposed here through the rebuilt libraries.
- **Why:** PO Plan rules had to sort, group, de-duplicate and accumulate over lists of SKUs and
  warehouses. Without these functions that logic sat in JavaScript Function nodes.
- **Benefit:** rules can drop Function (JS) nodes for list work. Expressions are faster and run
  inside the engine, without the JS runtime.
- **Downside / risk / what to watch:** these are new reserved function names. An existing
  expression that already used one of these names in another way may now parse differently. We did
  not audit all existing rules for this. The functions are a fork extension: decisions that use
  them will not run on upstream GoRules Zen or on the darwin/windows libraries in `deps/`.
- **Blocker removed:** PO Plan rules could not be written without JS Function nodes.
- **Tests run:** `engine_functions_test.go` → `TestExpression_ListFunctions` checks `sortBy`,
  `reduce` and `unique` through the Go binding. PR #2 also reports a manual smoke test from Go of
  `sortBy`, `reduce` and `join`/`unique`.
- **From PRs:** #2 (first release), #4 (re-release).

### 2. `executionMode: "sequential"`

- **What changed:** decision, decision-table and expression steps accept
  `executionMode: "sequential"`. When the step loops over `inputField`, it carries a state object
  from item to item (`stateField` sets its start value). The final state is returned next to the
  output (in the test: `allocations` and `allocationsState`).
- **Why:** PO Plan allocates a limited budget across warehouses in order. Each item depends on
  what earlier items took. The old loop mode treats items as independent.
- **Benefit:** running totals and allocations can be modelled in the graph, without JS.
- **Downside / risk / what to watch:** it is opt-in per node; nodes without `executionMode` behave
  as before. Sequential steps cannot run items in parallel, so large lists are slower than in the
  default mode. The extra `<outputPath>State` key appears in the output of a sequential step;
  callers that validate output shape strictly should expect it.
- **Blocker removed:** budget allocation rules needed carried state.
- **Tests run:** `TestDecision_SequentialStep` runs a four-warehouse allocation with budget 100
  and checks each take (30, 50.5, 19.5, 0) and the final state (`remaining: 0`).
- **From PRs:** #2, #4.

### 3. Database node on StarRocks: prepare, run and close every query

- **What changed:** the Database (Lookup) node now prepares, runs and closes each query instead of
  reusing a cached prepared statement. Implemented in the zen fork.
- **Why:** StarRocks re-executes a cached prepared statement that contains a subquery with its
  first parameters. Later lookups returned the rows of the first lookup (wrong data, no error).
- **Benefit:** correct rows on every lookup.
- **Downside / risk / what to watch:** one extra prepare and close round-trip per query. Watch
  Database-node latency and StarRocks connection or statement counts in busy runs. The change is
  for the StarRocks path; we did not measure its effect on other databases.
- **Blocker removed:** PO Plan rules that read StarRocks returned stale rows.
- **Tests run:** no test in this repo covers it (it needs a StarRocks server). The PR bodies do not
  report a StarRocks test from this repo; any test of it lives in the zen fork.
- **From PRs:** #2, #4.

### 4. `extract()` and `matches()` reuse compiled regular expressions

- **What changed:** the two functions cache compiled patterns instead of compiling the pattern on
  every call. Implemented in the zen fork.
- **Why:** rules that call them per SKU spent time recompiling the same pattern.
- **Benefit:** less CPU per evaluation for regex-heavy rules. Results do not change.
- **Downside / risk / what to watch:** a cache holds memory. We have not documented a size limit
  here; rules that build many different patterns at run time (for example from input data) are
  the case to watch.
- **Blocker removed:** speed only; nothing was blocked.
- **Tests run:** none in this repo. This was the only engine difference between ms.16 and ms.17.
- **From PRs:** #4 only (not in #2).

### 5. Library build and provenance

- **What changed:** `deps/PROVENANCE` now names zen commit `5f2abea` and new sha256 values for both
  Linux libraries. The build line changed from `cargo build --locked --release` to
  `cargo build --release ... (Cargo.lock as committed)`. The darwin and windows libraries were not
  rebuilt; they are still upstream zen-go v2.1.2 and are for local development only.
- **Why:** the libraries had to come from the fork commit that holds changes 1–4.
- **Benefit:** each library file is traceable to a commit, compiler and checksum.
- **Downside / risk / what to watch:** without `--locked`, cargo may update `Cargo.lock` during the
  build if it is out of date, so a rebuild may not be bit-for-bit the same. Please confirm the zen
  fork's `Cargo.lock` at `5f2abea` is complete, or ask us to rebuild with `--locked`. On macOS and
  Windows none of changes 1–4 exist, so tests in `engine_functions_test.go` fail there.
- **Blocker removed:** none on its own; it records where the libraries come from.
- **Tests run:** the checksums in `deps/PROVENANCE` match the committed files (checked with
  `sha256sum` while writing this file).
- **From PRs:** #2, #4.

### 6. New Go tests

- **What changed:** new file `engine_functions_test.go` (package `zen_test`, 101 lines) with
  `TestExpression_ListFunctions` and `TestDecision_SequentialStep`.
- **Why:** the fork features had no test in the binding, so a library built without them would
  pass CI.
- **Benefit:** a library built without the fork features now fails `go test`.
- **Downside / risk / what to watch:** the tests only pass with the fork's Linux libraries.
- **Tests run:** PR #4 reports `go vet` and `go test` passing on linux amd64 with cgo in Docker
  `golang:1.25-bookworm`. No arm64 test run is reported.
- **From PRs:** #4.

## Behaviour changes and compatibility

- **Go API:** no change. No Go source file except the new test was touched. `zen_engine.h` (the C
  header) is unchanged. `go.mod` is unchanged.
- **Expressions:** 14 new functions (topic 1). Decisions that use them need this fork's Linux
  libraries.
- **Decision JSON:** new optional node field `executionMode: "sequential"` (with `stateField`).
  Without it, nodes behave as before. Sequential steps add an `<outputPath>State` key to the result.
- **Database node:** same results API; each StarRocks query now does prepare, execute and close.
- **Platforms:** the changes exist only on linux amd64 and arm64. darwin and windows libraries are
  upstream v2.1.2.
- **Config flags:** none added in this repo.
- **Versions:** see below. Consumers pinned to `v2.1.6-ms.17` keep working; the tag still exists.

## Replaced PRs and old merge commits

| PR | Title | Old merge SHA | What happened to it |
|----|-------|---------------|---------------------|
| #2 | Release v2.1.6-ms.16: libzen_ffi from zen 794be1f | `b50ecd5c` | Backed out by #3 and re-released in #4. Its content is in this commit. |
| #3 | Revert weekend engine release (re-released as one combined release) | `2494b195` | A revert, not a feature. It returned `master` to `14ea1299` so #4 could land as one release. Nothing of it remains. |
| #4 | Release v2.1.6-ms.17: libzen_ffi from the combined engine release | `ae36a7e3` | Kept. This commit has exactly its final tree. |

Other commits that leave the branch history: `7150821` (ms.16 release commit), `8383589`
(revert commit), `214e186` (ms.17 release commit).

## Versions

Tags made in this window. Both are superseded. Their commits leave the `master` history, but the
tags still exist and still resolve (the Go module proxy has them cached). Do not move or delete
them.

| Tag | Commit | Status |
|-----|--------|--------|
| `v2.1.6-ms.16` | `b50ecd5c` | Superseded (no regex cache). |
| `v2.1.6-ms.17` | `ae36a7e3` | Superseded by the collapsed commit; same code. |

Last tag before the window: `v2.1.6-ms.15` at `28dae776`.

**Proposal:** tag the collapsed merge commit `v2.1.6-ms.18`. Its tree is identical to
`v2.1.6-ms.17`; the new tag only gives the code a commit that is on `master`. Consumers can move
from ms.17 to ms.18 with no code change. The Rules Engine team decides the final number.

## How to verify

```bash
# 1. The collapsed tree equals the deployed release
git diff --stat ae36a7e3 HEAD -- . ':!CHANGES-2026-11.md'   # expect no output

# 2. The full change against the pre-window base
git diff --stat 14ea1299 HEAD

# 3. Library checksums match deps/PROVENANCE
sha256sum deps/linux_amd64/libzen_ffi.a deps/linux_arm64/libzen_ffi.a
#   eb76c8b7809bb0ec11b2022a1fb8f054dd371156d911e5a2f870ae11f3f75556  linux_amd64
#   97db2aa9566933bba2a0470fe1d03e8d84ee8bfe945c0362440d07a65f9e7143  linux_arm64

# 4. Vet and test on linux amd64 with cgo (as in FORK.md and PR #4)
docker run --rm -v "$PWD":/src -w /src golang:1.25-bookworm \
  sh -c 'CGO_ENABLED=1 go vet ./... && CGO_ENABLED=1 go test ./...'

# 5. Only the new fork-feature tests
docker run --rm -v "$PWD":/src -w /src golang:1.25-bookworm \
  sh -c 'CGO_ENABLED=1 go test -run "TestExpression_ListFunctions|TestDecision_SequentialStep" -v .'

# 6. arm64 (not reported in the PRs; please run it)
docker run --rm --platform linux/arm64 -v "$PWD":/src -w /src golang:1.25-bookworm \
  sh -c 'CGO_ENABLED=1 go test ./...'
```

To review the engine source behind the libraries, compare the zen fork commits
`d37a89b..5f2abea` (the previous library source and the new one) in the zen repo.
