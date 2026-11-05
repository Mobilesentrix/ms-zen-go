# 1. Helpers on the evaluate path are added only when the compiler inlines them

## Status

Accepted

## Context

`evaluateC` and `evaluateCLocked` run on every evaluation. A helper there that is not inlined adds a real call.

## Decision

`cView` holds the `unsafe.Slice` conversion: `-gcflags=-m` reports it inlined at every call site, so it adds no call or allocation and replaces four repeated expressions.

`evaluateC` and `evaluateCLocked` keep their two inline error and nil checks. A shared `resultText` helper costs 82 to 90 against the inliner's budget of 80, so it would stay a real call.

`fillCBatchRequest` holds the fork-added loop body of a batch request and makes its partial-failure ownership explicit. The upstream deferred free stays in `EvaluateBatchWithOpts`. Error order and text are unchanged, and it adds one plain call per batch item.

## Consequences

- A new helper on this path is checked with `-gcflags=-m`.
