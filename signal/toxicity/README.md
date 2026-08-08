# `signal/toxicity` — did that liquidity get eaten, or did it just run away?

> The touch shrinking after a trade looks the same whether it filled or fled.
> Only bracketing it against the trade tape tells you which.

## What this package is

Toxicity measures the **honesty of near-touch liquidity** — when the best
bid or ask changes between two observations, was that because trades actually
executed against it (a sincere fill), because the resting order was pulled as
price moved away from it (a defensive retreat), or because it vanished for no
visible reason at the same price and quantity that didn't trade (a
cancellation — the tell for spoofing or fading liquidity that was never meant
to be hit)?

This package is entirely self-contained — there is no `nomagique` engine
behind it. Every accounting rule here (fill attribution, retreat detection,
cancellation inference) is implemented directly against the reconstructed
Level 3 order book (`websocket.BookSource`) and the public trade tape. No
category, no trading gate — `logic/category` decides what a reading means.

## The core method: bracket two touch snapshots against the trade tape

Every tick, `observedTouch` reads the book's current best bid/ask
(`touch.go`). If a *prior* toxicity touch measurement exists for this symbol
(`latestTouch`, read back from the thesis's own measurement history — this
package's state is not held in memory, it's read from what it published
last), the two touches bracket a time window `(previous.asOf, current.asOf]`.
`bracketedTrades` collects every valid trade in that window, and
`toxicityMeasurement` accounts for what happened to the *previous* touch's
resting liquidity using only that bracketed trade evidence.

If there is no prior touch, or no trades occurred in the bracket, only a bare
`touchMeasurement` is emitted (current best price/quantity, no
attribution) — toxicity accounting requires two touches and the trades
between them; anything less has nothing to attribute.

## Fill attribution

A trade **fills** the previous touch if it executed on the correct side, at
exactly the previous touch's price:

```
bidFill = Σ sell trades where price == previous.bid.price
askFill = Σ buy  trades where price == previous.ask.price
```

(A **sell** trade hits the **bid** — the aggressor is selling into the
resting buy order; a **buy** trade lifts the **ask**.) Fill is capped at the
resting quantity that was actually there (`math.Min(bidFill,
previous.bid.quantity)`) — a trade tape recording more volume at that price
than was resting cannot inflate the fill reading above what could physically
have executed against that specific touch.

## Retreat vs. cancellation: same shrinkage, different cause

Once fill is known, `touchChange` classifies what happened to whatever
resting quantity *wasn't* filled:

- **Retreat** — the touch price itself moved away in the defensive direction
  (bid price dropped, or ask price rose). The remaining unfilled quantity
  (`previous.quantity - executed`) is retreat, not cancellation: an order
  resting at a stale price that gets replaced by a fresh order at a better
  price for the market maker is ordinary quoting behavior, not deception.
- **Cancellation** — the price is **unchanged** but the resting quantity
  *shrank* beyond what fills explain (`disappeared = previous.quantity -
  current.quantity`, then `cancelled = max(0, disappeared - executed)`).
  Liquidity that was sitting at the same price, wasn't traded against, and is
  now gone is liquidity that was pulled — this is the toxic reading: size
  that was displayed but never intended to be hit.

If the price is unchanged and quantity did **not** shrink (or grew), neither
retreat nor cancellation applies — `touchChange` returns `(0, 0)`. Liquidity
sitting still or growing at the same price is neither retreating nor being
withdrawn.

## Every metric this package produces

All published under `source=toxicity`. The **bare** touch measurement (no
prior bracket) carries only `best_price`/`touch_quantity`, both sides, with no
`Normalized` (no ratio can be formed without a previous state to compare
against). The **full** toxicity measurement additionally carries:

| Metric                | Sides     | Meaning                                                                                                                                                                                                                                                                                                                                                       |
|-----------------------|-----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `trade_volume`        | none      | Total executed quantity across all bracketed trades, both sides pooled. `Normalized`: fraction of the previous touch's combined bid+ask resting quantity that traded.                                                                                                                                                                                         |
| `fill_volume`         | buy, sell | Quantity that filled the previous bid / previous ask at its exact resting price, capped at what was resting. `Normalized`: fraction of that side's previous resting quantity that was actually filled — the "how sincere was this touch" reading. High fill fraction on a shrinking touch means the liquidity did what displayed liquidity is supposed to do. |
| `best_price`          | buy, sell | Current best bid / ask. `Normalized` (full measurement only): signed log return from the previous touch's price on that side — a genuine zero if the price is unchanged, not merely omitted.                                                                                                                                                                  |
| `touch_quantity`      | buy, sell | Current resting quantity at the best bid / ask. `Normalized`: ratio against the previous touch's quantity on that side — `>1` growing, `<1` shrinking, unitless multiple.                                                                                                                                                                                     |
| `retreating_quantity` | buy, sell | Unfilled quantity that disappeared *because the touch price itself moved away* — a defensive requote, not necessarily toxic. `Normalized`: fraction of the previous resting quantity that retreated.                                                                                                                                                          |
| `cancelled_quantity`  | buy, sell | Unfilled quantity that disappeared with the price **unchanged** — displayed liquidity withdrawn without being traded against. `Normalized`: fraction of the previous resting quantity that was cancelled outright — the single most direct "was this touch honest" number this package produces.                                                              |

All quantities are in base currency (`Unit: base_currency`); prices in quote
currency. `ObservedFrom`/`Horizon` on the full measurement record the exact
bracket window this attribution covers.

### Readiness

`Normalized` on the full toxicity measurement requires a positive previous
resting quantity on that side (`normalizedTouchRatio`) and finite,
positive, correctly-ordered (`ask > bid`) prices on both touches
(`normalizedTouchPrice`). A touch with zero prior resting quantity on a side
cannot produce a fill/retreat/cancellation ratio for that side — there was
nothing there to account for.

## Files

| File        | Responsibility                                                                             |
|-------------|--------------------------------------------------------------------------------------------|
| `signal.go` | `Signal` lifecycle, per-symbol fan-out, trade bracketing, publish gating.                  |
| `touch.go`  | Touch snapshot extraction, fill/retreat/cancellation accounting, measurement construction. |

## What this package deliberately does not decide

A high `cancelled_quantity` reading is not "this is a spoof, veto the trade"
— it is "displayed liquidity at this price was withdrawn without being
traded against, this bracket." Whether that pattern is spoofing, routine
market-making requoting caught at an unlucky moment, or something else is
`logic/category`'s question, weighed against everything else measured on the
same tick — see the `SpoofedPump` trap tape in `tests/conditions`, which
exercises exactly this kind of evidence.
