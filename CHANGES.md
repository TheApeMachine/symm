# symm improvement pass

This archive was reconstructed from `symm(14).txt` and updated with a focus on optimizer depth, dynamic thresholds, correctness, and cleanup.

## Optimizer

- Replaced the MCTS hard-coded `{0.25, 0.5, 0.75}` quantile grid with `Profile.AdaptiveValues`, which derives compact threshold candidates from each category/unit's observed replay distribution.
- Changed the default hybrid search from a full-depth shallow scan to a genuinely shallow seed pass (`DefaultHybridShallowDepth = 2`) followed by a larger MCTS budget (`DefaultMCTSIterations = 1024`). This gives deep branch exploration more of the candidate budget.
- Made beam survivor dedupe use full recursive branch fingerprints instead of only category names and depth, so distinct thresholds/actions are no longer collapsed into the same seed.
- Ranked branchers by gate selectivity as well as pass count, favoring gates that actually split the replay tape instead of mostly-always-true thresholds.
- Added category-stratified brancher selection so dominant categories cannot crowd out deeper multi-signal paths.
- Weighted MCTS expansion and rollout by selectivity and category novelty, which encourages deeper branches that add new behavioral context.
- Wired the existing nested-gate affinity index so successful survivor/gate combinations bias later deepening.
- Avoided a duplicate canonicalization pass during scan result acceptance.

## Correctness and performance

- Replaced `sync.WaitGroup.Go` usage with explicit `Add`/`Done` goroutines for broader Go toolchain compatibility.
- Fixed chart viewport tracking so programmatic follow-range updates do not mark the user as controlling the viewport, while real user range changes do.
- Added cleanup return values for registered trade charts to avoid stale chart sinks after unmount/remount.
- Capped OHLC history buffers per symbol to prevent unbounded frontend memory growth.
- Fixed `Flex.Row` and `Flex.Column` to forward `className`, and normalized the import alias.
- Fixed `use-symm-ui` stubs so the Decisions panel no longer dereferences `undefined`, and derives scan progress/evaluations from live telemetry when present.
- Expanded event typings to match fields already consumed by the UI.
- Adjusted the trades data provider to show wallet-derived open positions only, preventing duplicate transient fill rows.

## Cleanup

- Removed the unused duplicate frontend financial-chart helper file at `frontend/src/components/symm/utils.ts`.
- Added `tools/slice_symm_archive.py`, a reusable slicer for this text-archive format.
- Added minimal embedded config files required by the existing `go:embed` directives so a fresh checkout has the files the binary expects.

## Validation notes

- Ran `gofmt` over the Go source tree.
- `go test ./...` could not run from the reconstructed archive because the uploaded text did not include a `go.mod` file: `directory prefix . does not contain main module or its selected dependencies`.
- Frontend test/build commands could not be run because the uploaded text did not include `package.json`, lockfiles, or TypeScript/Vite config files.
