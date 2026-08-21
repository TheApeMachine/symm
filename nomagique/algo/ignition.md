# Ignition Frame contract

Ignition has no domain-specific input, output, side, keyed-collection, or window
struct. One ordered stream is represented by a `nomagique.Frame`, and key
ownership is handled by `nomagique.KeyedStreams` or by the caller's transport
layer.

## Observation symbols

A valid input frame requires:

- `SymbolCapacity`: integer retention bound from 1 through
  `MaxIgnitionHistory`
- `SymbolVolume`: positive finite executed quantity
- `SymbolLast`: positive finite trade price
- `SymbolBid`: positive finite executable bid
- `SymbolAsk`: positive finite executable ask above bid
- optional paired `SymbolUnixSec` and `SymbolUnixNsec`

## Public output symbols

Every initialized state exposes:

- raw `SymbolRVOL` and its baseline-ratio squash in `SymbolRVOLNormalized`
- `SymbolMidpoint`, raw `SymbolSpread`, and midpoint-relative
  `SymbolSpreadNormalized`
- prior retained `SymbolSpreadBaseline` and current `SymbolCompression`,
  calculated as `max(0, 1 - spread/spreadBaseline)`
- `SymbolIgnitionBarRate`, `SymbolIgnitionRateBaseline`
- `SymbolReady`, `SymbolMaturity`, `SymbolIgnitionHypothesisSeparation`
- `SymbolIgnitionObservedFromSec` and `SymbolIgnitionObservedFromNsec`, which
  preserve the opening event time of the bar that produced the current score

Directional outputs use exported `SymbolAlpha...` and `SymbolBeta...` slots:
per-side `rvol`, `precursor`, normalized precursor, and `exhaustion`. Ratio
normalization uses the natural ratio baseline of one, so an estimator reading
at its own median maps to one half without a fitted or global threshold. The
tape measures and stores; fusion, winning sides, and categories are downstream
decisions.

## Internal state

Causal bar state uses the same legacy names under `window/`, now represented by
interned offsets. Bounded history occupies fixed slots named
`history/<family>/sample/<slot>`.

Deltas use the engine's generic sample range. Rates, returns, and spreads use
reserved Ignition ranges. The positive-only precursor baseline is derived from
the returns range; zero moves remain valid return observations but do not enter
that positive median.

## Transaction model

`Ignition(state, input)` edits a local candidate frame. Validation, temporal
regression, or calculation failure returns the original state and an error.
`nomagique.Stream.Step` commits the candidate only when no error is returned.
No algorithm field, hidden rollback structure, mutex, or mutable error slot is
required.
