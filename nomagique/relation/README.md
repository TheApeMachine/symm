# Relation

Relation measures directed temporal predictive contribution between Measurement
coordinates. It answers:

> Did knowing Source history improve prediction of Target beyond Target's own
> history and explicitly supplied Controls?

It does not prove physical causality, does not decide actions, and never deletes
measurements.

## Primitives

Everything is a `core.Primitive` over the `unsafe.Pointer` wire:

| Primitive | Role |
| --- | --- |
| `ObservationStore` | Bounded chronological ring per coordinate |
| `Project` | Split Measurements into store batches |
| `Planner` | Compile one plan into candidate adapters |
| `Align` | Walk lagged series |
| `Influence` | Prequential predictive contribution on an adapter |

A coordinate key joins identity fields with `|`:

```text
symbol|source|metric|side|peer|unit|timescale|epoch
```

A selector is `[3]string{source, metric, side}`; an empty field is a wildcard.
Observations are `{at, raw}` with `at` in Unix nanoseconds. Series flatten to
`{at0, raw0, at1, raw1, ...}`. Lags are nanoseconds.

## Influence contract

`Influence` receives `**data.Adapter` roles:

- text `"source"`, `"target"`, `"control.<i>"` — coordinate keys
- number `"controls"` — control count
- number `"control.<i>.lag"` — control lag (ns); `<= 0` aligns at the source lag
- number `"min_lag"`, `"max_lag"` — candidate lag domain (ns); `0` derives it

It publishes status (`Fit*` constants), lag provenance, coefficient statistics,
residual variances, `predictive_gain`, maturity, and lag surface entries. Math
undefined is `NaN`; undefined is never zero.

## Rules that stay

1. Relate coordinates, not whole signal packages.
2. Prefer signed causal standardized residual of a Measurement; do not substitute SNR for signed state.
3. Never use future Source observations; require `lag > 0` for Influence.
4. Evaluate prequentially: fit on the past, predict the present, then update.
5. Do not silently regularize singular fits or threshold Influence into an edge boolean.
6. Preserve Source/Target/lag provenance on every estimate.
