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

- `SymbolRVOL`, `SymbolSpread`
- `SymbolIgnitionBarRate`, `SymbolIgnitionRateBaseline`
- `SymbolReady`, `SymbolMaturity`

Directional outputs use exported `SymbolAlpha...` and `SymbolBeta...` slots:
per-side `rvol`, `precursor`, and `exhaustion`. The tape measures and stores;
fusion, winning sides, and categories are downstream decisions.

## Internal state

Causal bar state uses the same legacy names under `window/`, now represented by
interned offsets. Bounded history occupies fixed slots named
`history/<family>/sample/<slot>`.

Deltas use the engine's generic sample range. Rates, returns, and precursors
use reserved Ignition ranges. Returns and precursors remain separate:
zero moves are valid return observations but are not retained as precursors.

## Transaction model

`Ignition(state, input)` edits a local candidate frame. Validation, temporal
regression, or calculation failure returns the original state and an error.
`nomagique.Stream.Step` commits the candidate only when no error is returned.
No algorithm field, hidden rollback structure, mutex, or mutable error slot is
required.
