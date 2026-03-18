# Native Rule Engine and Caching – Tasks

## Phase 1: Infrastructure
- [x] Add `EvaluatorCache` implementation with LRU, TTL, background cleanup and invalidation.
- [x] Write unit tests for `EvaluatorCache` (insertion, eviction, expiry, thread safety).
- [x] Add `cache.InvalidateAll` call to existing `Evaluator.reload`.

## Phase 2: Native Rule Engine
- [x] Implement `domainSuffixTrie` (insert, match) with case‑insensitivity and deduplication.
- [x] Implement `NativeRuleEngine` struct and constructor (`NewNativeRuleEngine`).
- [x] Integrate exact map, suffix trie, host glob dedup + precompilation.
- [x] Skip `MatchURLGlob` rules with warning log.
- [x] Write unit tests:
  - Exact map: duplicates, case, port stripping.
  - Suffix trie: simple match, longest‑match, case, overlapping patterns, duplicates.
  - Host globs: ordering, dedup, early exit.
  - Full engine: mixed rules, verify earliest index wins.
- [x] Add fuzz test for trie insertion and matching consistency.

## Phase 3: Evaluator Integration
- [x] Modify `Evaluator` to hold `engineType` and `current` as `interface{}` (or separate fields).
- [x] Implement `Evaluator.Evaluate` using cache and engine dispatch.
- [x] Ensure `reload()` builds native engine when `__PAC_RULES__` present; falls back to JS on error.
- [x] Verify that malformed native build does not crash proxy (JS fallback).

## Phase 4: Performance
- [x] Write benchmark `BenchmarkEvaluator_Native_Uncached` with 4100‑rule PAC.
- [x] Write benchmark `BenchmarkEvaluator_Native_Cached` hot cache.
- [x] Write benchmark `BenchmarkEvaluator_JS` for comparison.
- [x] Measure latency distribution and cache hit‑rate impact.

## Phase 5: Observability & Admin
- [x] Add `/api/cache` endpoint to admin UI (JSON stats).
- [x] Add cache stats to regular `/api/status` response optionally.
- [x] Log cache hit‑rate periodically or on signal.

## Phase 6: Documentation & Config
- [x] Update README: describe native engine, cache parameters, and URL‑glob limitation.
- [x] Add YAML config options under `pac.cache`: `enabled`, `max_size`, `ttl`.
- [x] Document design decisions in comments.

## Phase 7: Testing & Release
- [x] Run full integration test suite (including GFW integration tests).
- [x] Manual smoke test: start proxy, verify PAC works, check admin UI cache stats.
- [x] Load test: simulate 1000 concurrent requests, compare CPU usage before/after.
- [x] Create release notes.
