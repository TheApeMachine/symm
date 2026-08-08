# `signal/cvd` — is aggressive flow moving price, or being eaten by it?

> The same net-buy pressure means opposite things depending on what price did
> with it. Moved it: drive. Went nowhere: absorption.

## What this package is

CVD (cumulative volume delta) is the **Absorption perspective** — it classifies
signed aggressor flow (buy notional minus sell notional) against the price
response that flow actually produced. It answers one question per trade
window: given how much directional pressure just executed, did price move a
proportionate amount, move less than it should have (absorption — someone is
soaking up the aggression without giving ground), or move more or less
depending on whether flow was even present at all (starvation)?

This package is the market adapter. Windowing lives in
[`nomagique/algorithm/trade_flow_sample.go`](../../../nomagique/algorithm/trade_flow_sample.go),
classification in
[`nomagique/equation/flow.go`](../../../nomagique/equation/flow.go). This
package turns Kraken trades and tickers into that engine's input and projects
its output onto the measurement wire — it does not decide what a reading
means; `logic/category` does.

## Two prices, deliberately kept separate

Every trade carries two prices that must not be conflated:

- **Execution price** (`row.Price`) — what the aggressor actually paid.
  `Price × Quantity` is the trade's signed notional.
- **Response price** — the quote **midpoint** at the moment of the trade
  (`observeMidpoint`, from ticker bid/ask), not the execution price itself.

Using execution price to measure "price response" would measure bid-ask
bounce, not directional movement — an aggressor crossing the spread always
"moves" the tape by the spread width whether or not the market actually
shifted. The midpoint is not contaminated by which side of the book the
trade executed against, so its drift between the window's first and last
trade is the market's actual directional response, isolated from execution
mechanics.

`midpointAt` binary-searches backward through retained midpoint observations
to find the one in force at each trade's timestamp — trades are matched to
the midpoint that existed *at* that moment, not the most recent one when the
batch is processed.

## The adaptive window

`TradeFlowSample` retains up to 128 recent trades per symbol
(`flowSampleHistoryCap`) and resolves two nested windows from that history via
`statistic.ResolveWindowSet`, rather than using a fixed lookback:

- **Active window** — `max(ShortWindow, ReturnLag+1)`, capped at `LongWindow`.
  The most recent trades scored on this tick.
- **Baseline window** — the full `LongWindow`, used only to establish what
  "normal" notional looks like for this symbol right now.

Both windows are sized from the observed notional history itself
(`ResolveWindowSet`), so an illiquid symbol with sparse, chunky trades and a
liquid symbol with a steady stream of small trades get windows scaled to their
own tape rather than sharing one global trade count.

## Flow classification (`equation.Flow`)

Given the active window's buy/sell notional split and price drift, `Flow`
computes:

```
net           = buyNotional - sellNotional
netFraction   = |net| / gross                     (0..1, how one-sided the flow was)
priceResponseBps = |lastPrice/firstPrice - 1| × 10000
flowPressure  = gross / medianNotional             (this window vs. typical size)
impactEfficiency = priceResponseBps / flowPressure  (bps of move per unit of pressure)
```

`driveThreshold = 1/√TradeCount` is the bar `netFraction` must clear to count
as "high net" — it shrinks as the window holds more trades, since a lopsided
split is less likely to be noise with more observations backing it.

Four outcomes fall out of comparing net direction against price direction and
against the symbol's own typical move size (`medianAbsoluteMove`, the median
absolute step between consecutive prices in the window):

| Outcome        | Condition                                                                                                                                                                                    | What it means                                                                                                                                                                                                                                          |
|----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Drive**      | High net, price moved in the same direction as net, not flat                                                                                                                                 | Aggression is moving price roughly proportionately — flow and price agree.                                                                                                                                                                             |
| **Absorption** | High net but price stayed flat, or net and price disagree in direction                                                                                                                       | Aggression happened and price did not follow — this is the "someone is standing in front of the tape" reading. Includes **hidden absorption**: high net, low or zero impact efficiency, flat price — flow that produced literally no directional dent. |
| **Starvation** | Gross notional falls below the window's own empirical floor (median minus median absolute deviation of the baseline window), or pressure is otherwise below that reference with net not high | There simply wasn't enough flow this window to say anything about drive or absorption — the reading is "the tape went quiet," not a directional claim.                                                                                                 |
| **Balance**    | None of the above; also the reflexive first-observation case                                                                                                                                 | Net flow was not one-sided enough, or (with only one price observed so far) there is no price response to compare against yet, so only the buy/sell split itself is scored.                                                                            |

Only one of Absorption/Drive/Starvation is nonzero per window (Balance is
computed regardless as `max(0, 1-netFraction)`, and zeroed out only when
Starvation wins). `Value`/`Strength` is `max` across all four — "how strong was
whatever this window turned out to be," independent of which category won.

## Every metric this package produces

All published under `source=cvd`, keyed `type:none` (CVD does not split by
side — the buy/sell asymmetry is already folded into `net`/`net_fraction`).

| Metric         | Meaning                                                                                                                                                                                                                                                                                                                                                                                                                              |
|----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `absorption`   | How much of this window's net flow price failed to respond to. `0` unless the window classified as absorption or hidden absorption. Under hidden absorption, scaled by how far actual impact efficiency falls below the symbol's own expected efficiency (`impactEfficiencyScale`) — near-zero impact scores close to the full `netFraction`; impact approaching the expected scale scores less.                                     |
| `drive`        | `netFraction` when net flow and price moved together and price was not flat, else `0`. How much of the window's imbalance is confirmed by the price response it produced.                                                                                                                                                                                                                                                            |
| `balance`      | `max(0, 1 - netFraction)` — how evenly split the window's buy/sell notional was, independent of price response. `1` for perfectly balanced flow, `0` for fully one-sided. This is the only metric defined on the very first trade of a symbol's history, before any price response exists to compare against (`Prices` length < 2) — see the reflexive-boundary comment in `flow.go`.                                                |
| `starvation`   | How far gross notional falls below this window's expected floor, as `1 - pressure` where `pressure = gross/reference` (reference is the baseline window's empirical floor, or median notional if that floor is non-positive). `0` unless the window was classified as starved.                                                                                                                                                       |
| `strength`     | `max(absorption, drive, balance, starvation)` — the winning category's own reading, so a consumer that only wants "how strong was this window" doesn't need to know which category produced it.                                                                                                                                                                                                                                      |
| `net_fraction` | `\|net\| / gross` — how one-sided the window's flow was, unsigned, `0..1`.                                                                                                                                                                                                                                                                                                                                                           |
| `net`          | Signed net notional in quote currency (`buyNotional - sellNotional`). `Unit: quote_currency`, not dimensionless — this is the one CVD metric with real economic units, everything else is a bounded ratio. `Normalized` restores sign to `net_fraction` (`normalizedSignedNet`): the signed share of gross notional that was net, so a consumer can read direction and magnitude from `Normalized` alone without also needing `Raw`. |

### Readiness

`absorption`, `drive`, and `starvation` require at least two price
observations in the window (`minimumPriceResponseObservations`) — normalized
is `nil` before that, since a price-response category is undefined without a
response to measure. `balance` and `net_fraction` are defined from the first
trade onward. All metrics require at least one trade in the evidence count.

## Concurrency and ordering

Each symbol's trades are measured on an independent goroutine
(`errgroup`), but *within* one symbol, trades are strictly sorted by
timestamp (ties broken by trade ID) before measurement, and `seenTrade` /
`commitTrade` track a per-symbol high-water mark so a trade already folded
into the window is never double-counted if the same batch is observed twice.
`midpointAt` and `commitMidpoint` are similarly ordered: only midpoints at or
before a trade's timestamp are eligible, and midpoints strictly older than the
last matched one are pruned so the retained set does not grow unbounded.

## Files

| File        | Responsibility                                                                              |
|-------------|---------------------------------------------------------------------------------------------|
| `signal.go` | `Signal` lifecycle, trade/midpoint bookkeeping, per-symbol fan-out, measurement projection. |

The windowing and classification engines live in
`nomagique/algorithm/trade_flow_sample.go` and `nomagique/equation/flow.go`,
not here.
