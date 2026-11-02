# 2. Fork code is laid out apart from upstream names

## Status

Accepted

## Context

`raw.go` mixed a stateful reader with the evaluation entry points. Callers reach `RawEvaluator`, `ResultEvaluator`, `DecisionCache`, `RawDecision` and `ResultDecision` by type assertion, so a dropped method fails only at run time.

## Decision

`RawResult` and its methods live in `raw_result.go`; evaluators and C-memory helpers stay in `raw.go`. No code path changes.

`var _` assertions pin those interfaces at compile time, at no runtime cost.

Upstream names stay, to keep merges from upstream small: `extractJsonFromAny`, `loaderHandlerIdPtr`, `customNodeHandlerIdPtr`, `resultJson`, the `deps/*` package names, and `NewEngine`'s structure. The exported `ErrorCodeJsonSerializationFailed` and `ErrorCodeJsonDeserialization` keep their spelling because renaming them changes the API.

## Consequences

- Linters report those upstream and exported names.
