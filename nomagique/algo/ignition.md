# Ignition map contract

Ignition has no domain-specific input, output, side, or window structs. Its IO
payload is the existing generic composition:

```go
types.Pair[
    string,
    types.Map[
        string,
        types.Map[string, types.Value[float64]],
    ],
]
```

The pair key selects the active stream. The outer map retains every keyed
stream. Each stream map contains the latest observation, retained volume-clock
state, bounded histories, and computed outputs.

## Observation keys

A staged active stream requires:

- `capacity`: positive integer retention bound
- `volume`: positive executed quantity
- `last`: positive trade price
- `bid`: positive executable bid
- `ask`: positive executable ask above `bid`
- `unix_sec`, `unix_nsec`: optional normalized event-time coordinates; either
  both are present or neither is present

## Public output keys

The active stream map always exposes:

- `value`, `rvol`, `precursor`, `spread`, `compression`
- `ignition`, `trend`, `exhaustion`, `strength`, `category`
- `ready`, `maturity`

Directional outputs use `buy/` and `sell/` prefixes, for example
`buy/precursor`, `sell/exhaustion`, and `buy/strength`.

## Internal map namespaces

`window/` contains causal bar state. `history/<family>/` contains the bounded
ring state produced by `transport.Window`; retained values use
`history/<family>/sample/<slot>`.

These namespaces are data, not fields on `Ignition`, so they can be projected,
transported, persisted, or supplied to another primitive without adapters.

## Legacy decomposition

| Legacy ignition component | New composition |
| --- | --- |
| `Squash` | `calculus.Squash` |
| `Inverse` | `calculus.Inverse` |
| `Ratio` | `calculus.Ratio` |
| `Mean` | `probability.Geomean` followed by `logic.Gate` |
| `RatioScale` | `statistic.Median` followed by `calculus.Ratio` |
| `Exhaustion` | `Difference` → `Positive` → `Ratio`, multiplied by `Squash` |
| `ignitionWindow` | `transport.Window` plus map-carried causal clock state |
| `IgnitionInput`, `IgnitionOutput`, `IgnitionSideOutput` | removed; all values use the map contract |

A failed candidate is exposed through `next.Error()` while `initial` remains the
last committed collection. This makes time-regression and validation failures
transactional without an `err` field or a hidden rollback structure.
