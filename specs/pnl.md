# Improving the Kraken PnL Implementation

## Purpose

This document reviews the current `pnl` implementation and proposes a safer, more precise design for calculating estimated and realized profit and loss for Kraken Spot positions.

The current formula is broadly correct for estimating the net PnL of a **long** position:

```text
gross PnL = (exit price - entry price) × quantity

net PnL = gross PnL
          - entry fee
          - estimated exit fee
```

The main areas for improvement are:

1. distinguish estimated PnL from realized PnL;
2. use Kraken instrument metadata deliberately;
3. avoid `float64` for financial inputs;
4. support position direction explicitly;
5. prevent nil-pointer and non-finite-number failures;
6. return errors instead of logging and returning an ambiguous zero;
7. define a clear rounding policy;
8. use actual execution fees whenever they are available.

---

## 1. Kraken precision metadata

Kraken WebSocket v2 publishes reference data through the `instrument` channel.

For each trading pair, relevant fields include:

| Field | Kraken description | Recommended use |
|---|---|---|
| `price_precision` | Maximum precision used for order prices | Validate or normalize order prices |
| `price_increment` | Minimum price increment for new orders | Validate that an order price lies on the permitted tick |
| `qty_precision` | Maximum precision used for order quantities | Validate or normalize order quantities |
| `qty_increment` | Minimum quantity increment for new orders | Validate that a quantity lies on the permitted step |
| `cost_precision` | Maximum precision used for cost prices | Normalize Kraken pair-cost/order-cost values |
| `ws_display_price_precision` | Recommended display precision for WebSocket prices | User-interface formatting only |

For each asset, Kraken also provides:

| Field | Kraken description | Recommended use |
|---|---|---|
| `precision` | Maximum precision for asset ledgers and balances | Quote-asset accounting and balance-like values |
| `precision_display` | Recommended display precision | User-interface formatting only |

References:

- [Kraken WebSocket v2 instrument channel](https://docs.kraken.com/exchange/api-reference/spot-websocket-v2/instrument)
- [Kraken WebSocket v2 ticker channel](https://docs.kraken.com/exchange/api-reference/spot-websocket-v2/ticker)
- [Kraken WebSocket v2 executions channel](https://docs.kraken.com/exchange/api-reference/spot-websocket-v2/executions)

### Do not infer precision from one ticker value

This approach is fragile:

```go
ticker.Bid.GetScale()
```

It reports the scale of the value as decoded by the client, not necessarily the pair's configured precision. A particular ticker update may contain fewer decimal places than the pair permits.

Similarly, this is not a reliable financial rounding policy:

```go
max(
    entryPrice.GetScale(),
    bid.GetScale(),
    feeRate.GetScale(),
)
```

It ignores quantity precision and assigns meaning to the incidental representation of the inputs.

### `cost_precision` versus quote-asset `precision`

These fields should not be treated as interchangeable.

`cost_precision` is pair metadata for cost values. It is appropriate when producing a Kraken-compatible pair cost or validating order-related calculations.

PnL, however, is a quote-currency accounting value. For internally accumulated PnL, the quote asset's ledger/balance `precision` may be the more appropriate final storage scale.

A practical policy is:

```text
Exact intermediate arithmetic:
    no rounding

Internal PnL/accounting result:
    quote asset precision

Kraken pair-cost or order-facing result:
    pair cost_precision

Display:
    explicit application/UI policy
```

The application should encode this choice directly rather than deriving it from the operands.

---

## 2. Fee data is mandatory

The current function estimates both entry and exit fees from one fee rate:

```go
entryFee = entryPrice * quantity * feeRate
exitFee  = exitPrice  * quantity * feeRate
```

That is acceptable only as an estimate when the actual fees are unavailable.

Kraken's authenticated `executions` channel reports trade-event fees as fee objects containing the fee asset and quantity. It also reports execution cost, execution quantity, execution price, and whether the fill was maker or taker.

Therefore:

- store the **actual entry fee** from the entry fills;
- use an estimated exit fee only while the position remains open;
- once the position is closed, use the **actual exit fee** from the closing fills;
- aggregate fees by asset rather than assuming forever that every fee is represented by one rate.

Kraken currently documents execution fees as being expressed in quote currency, but retaining the fee asset in the internal model is still safer.

Recommended terminology:

```go
estimatedPnL(...)
realizedPnL(...)
```

Avoid naming both operations simply `pnl`, because their guarantees differ.

---

## 3. Avoid `float64` for quantity and fee rate

The current implementation accepts:

```go
feeRate float64
position.data.Qty float64
```

This introduces two problems.

First, `float64` cannot represent many decimal fractions exactly. Converting it to `big.Rat` preserves the exact binary approximation, not necessarily the decimal value originally received or intended.

Second, this can return `nil`:

```go
new(big.Rat).SetFloat64(value)
```

when `value` is NaN or infinite. Calling `Sign()` on that result can panic.

For financial values, prefer the existing decimal type:

```go
type Position struct {
    EntryPrice decimal.Decimal
    Qty        decimal.Decimal
    EntryFee   decimal.Decimal
}
```

and accept:

```go
exitFeeRate decimal.Decimal
```

Parse numeric WebSocket values without an intermediate `float64` where the Kraken client library permits it. If the client library exposes only `float64`, validate finiteness immediately and convert once at the API boundary.

---

## 4. Represent position direction explicitly

Using the best bid and this formula:

```text
(exit - entry) × quantity
```

is correct for estimating the liquidation value of a long position.

A short position closes by buying, so it should use the best ask:

```text
(entry - exit) × quantity
```

The ticker channel exposes both the best bid and best ask. The implementation should select the executable side explicitly.

```go
type PositionSide uint8

const (
    PositionLong PositionSide = iota + 1
    PositionShort
)
```

Then:

```go
switch position.Side {
case PositionLong:
    exitPrice = ticker.Bid
case PositionShort:
    exitPrice = ticker.Ask
default:
    return zero, ErrInvalidPositionSide
}
```

This also makes the function's assumptions testable.

---

## 5. Validate all inputs before arithmetic

At minimum, validate:

- `position != nil`;
- entry price is positive;
- exit price is positive;
- quantity is positive;
- fees are non-negative;
- fee rate is non-negative;
- the ticker symbol matches the position symbol;
- pair metadata exists for the symbol;
- the metadata identifies the expected quote asset;
- position side is known.

If any values still enter as `float64`, validate them with `math.IsNaN` and `math.IsInf` before conversion.

A symbol mismatch is especially dangerous because the arithmetic can succeed while producing a meaningless result.

---

## 6. Return an error

The current function logs an error and returns:

```go
decimal.Decimal{}
```

This makes a failed calculation indistinguishable from a legitimate zero PnL.

Prefer:

```go
func (...) (decimal.Decimal, error)
```

The caller should decide whether to log, retry, omit the value, or surface an error. A low-level calculation method should generally not both report an error globally and return a normal-looking value.

---

## 7. Keep arithmetic exact and round once

The existing use of `big.Rat` is reasonable, provided decimal values are converted without first passing through binary floating point.

Keep all intermediate operations exact:

```text
entry notional
exit notional
gross PnL
entry fee
exit fee
net PnL
```

Apply rounding once, when converting the final result into the selected accounting representation.

Do not round the entry notional and exit notional independently unless Kraken's accounting rules specifically require per-fill rounding. For realized PnL, using Kraken's reported execution costs and fees avoids having to reproduce exchange-side rounding.

### Choose and document a rounding mode

`big.Rat.FloatString(scale)` rounds to the requested number of digits. The codebase should state whether this is the intended policy.

Accounting systems commonly need an explicit rule such as:

- half away from zero;
- half even;
- truncate toward zero;
- floor;
- ceiling.

The correct choice depends on how the value is used. Display rounding and accounting rounding should be separate operations.

---

## 8. Recommended data model

A stronger model stores decimal values and actual entry information.

```go
type PositionSide uint8

const (
    PositionLong PositionSide = iota + 1
    PositionShort
)

type Position struct {
    Symbol     string
    Side       PositionSide
    EntryPrice decimal.Decimal
    Qty        decimal.Decimal
    EntryFee   decimal.Decimal
}

type PairMetadata struct {
    Symbol          string
    BaseAsset       string
    QuoteAsset      string
    PricePrecision  int32
    QtyPrecision    int32
    CostPrecision   int32
}

type AssetMetadata struct {
    Asset            string
    LedgerPrecision  int32
    DisplayPrecision int32
}
```

For positions built from multiple fills, a single entry price should be a volume-weighted average, and `EntryFee` should be the sum of the actual entry-fill fees.

An even stronger model stores fills and derives the position aggregates from them.

---

## 9. Recommended PnL API

The function should receive the metadata or an already selected result scale. Passing the scale directly keeps the calculator independent from the metadata cache.

```go
func (price *Price) estimatedPnL(
    position *Position,
    ticker kraken.TickerData,
    estimatedExitFeeRate decimal.Decimal,
    resultScale int,
) (decimal.Decimal, error)
```

The caller can select:

```go
resultScale := int(quoteAssetMetadata.LedgerPrecision)
```

for accounting, or:

```go
resultScale := int(pairMetadata.CostPrecision)
```

for a pair-cost representation.

---

## 10. Example revised implementation with mandatory fee data

The following preserves the rational-arithmetic approach and assumes the decimal type exposes `Rat()`, `Sign()`, and `NewFromString()` similarly to the original code.

```go
func (price *Price) estimatedPnL(
    position *Position,
    ticker kraken.TickerData,
    estimatedExitFeeRate decimal.Decimal,
    resultScale int,
) (decimal.Decimal, error) {
    var zero decimal.Decimal

    if position == nil {
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: position must not be nil",
            nil,
        )
    }

    if position.Symbol == "" || ticker.Symbol != position.Symbol {
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: ticker symbol does not match position symbol",
            nil,
        )
    }

    if resultScale < 0 {
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: result scale must be non-negative",
            nil,
        )
    }

    if position.EntryPrice.Sign() <= 0 {
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: entry price must be positive",
            nil,
        )
    }

    if position.Qty.Sign() <= 0 {
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: quantity must be positive",
            nil,
        )
    }

    if position.EntryFee.Sign() < 0 {
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: entry fee must be non-negative",
            nil,
        )
    }

    if estimatedExitFeeRate.Sign() < 0 {
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: exit fee rate must be non-negative",
            nil,
        )
    }

    var exitPrice decimal.Decimal

    switch position.Side {
    case PositionLong:
        exitPrice = ticker.Bid
    case PositionShort:
        exitPrice = ticker.Ask
    default:
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: unsupported position side",
            nil,
        )
    }

    if exitPrice.Sign() <= 0 {
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: executable exit price must be positive",
            nil,
        )
    }

    entryRat := position.EntryPrice.Rat()
    exitRat := exitPrice.Rat()
    qtyRat := position.Qty.Rat()

    entryNotionalRat := new(big.Rat).Mul(entryRat, qtyRat)
    exitNotionalRat := new(big.Rat).Mul(exitRat, qtyRat)

    var grossRat *big.Rat

    switch position.Side {
    case PositionLong:
        grossRat = new(big.Rat).Sub(exitNotionalRat, entryNotionalRat)
    case PositionShort:
        grossRat = new(big.Rat).Sub(entryNotionalRat, exitNotionalRat)
    }

    estimatedExitFeeRat := new(big.Rat).Mul(
        exitNotionalRat,
        estimatedExitFeeRate.Rat(),
    )

    totalFeeRat := new(big.Rat).Add(
        position.EntryFee.Rat(),
        estimatedExitFeeRat,
    )

    netRat := new(big.Rat).Sub(grossRat, totalFeeRat)

    net, err := decimal.NewFromString(netRat.FloatString(resultScale))
    if err != nil {
        return zero, errnie.Err(
            errnie.Validation,
            "broker price: invalid PnL result",
            err,
        )
    }

    return *net, nil
}
```

### Notes about this example

1. It uses the stored actual entry fee.
2. It estimates only the future exit fee.
3. It supports both long and short positions.
4. It chooses bid for longs and ask for shorts.
5. It receives an explicit result scale.
6. It returns errors instead of emitting an error and returning zero.
7. It validates the ticker symbol.
8. It avoids `float64` in the calculator.

Adapt field and method names to the exact Kraken and decimal types in the project.

---

## 11. Realized PnL implementation

For a fully closed position, avoid estimating fees and, where possible, avoid reconstructing exchange execution values.

The `executions` channel provides trade-event fields including:

- execution `cost`;
- execution quantity;
- execution price;
- fee asset and fee amount;
- maker/taker liquidity indicator;
- side;
- symbol.

A realized calculation can therefore use aggregates from the actual fills:

```text
long realized PnL =
    total closing proceeds
    - total opening cost
    - actual opening fees
    - actual closing fees
```

For a short or margin position, cash flows and borrowing costs require a model appropriate to Kraken margin trading. The simple spot-long formula should not silently be reused for that case.

A possible API is:

```go
func realizedPnL(
    openingCost decimal.Decimal,
    closingProceeds decimal.Decimal,
    openingFees decimal.Decimal,
    closingFees decimal.Decimal,
    resultScale int,
) (decimal.Decimal, error)
```

For spot longs:

```text
net = closingProceeds - openingCost - openingFees - closingFees
```

This is more reliable than recalculating every fill from an assumed fee rate.

---

## 12. Fee-rate considerations

A single fee rate may not match the eventual closing fee because:

- maker and taker rates can differ;
- the exit order may fill partly as maker and partly as taker;
- fee tiers can change with account volume;
- multiple fills can occur;
- Kraken can report the actual fee directly after execution.

For unrealized PnL, define the estimate conservatively. For example, use the expected taker fee if the displayed PnL assumes immediate liquidation at the top of book.

Name the input accordingly:

```go
estimatedExitFeeRate
```

Document that it is a fraction:

```text
0.0026 means 0.26%
```

Validate the upper bound as a domain sanity check if the application has a known maximum.

---

## 14. Fail-closed trading integration

The calculator returning an error is not sufficient by itself. The trading workflow must guarantee that the error blocks order creation.

```go
pnl, err := price.pnl(position, ticker, feeRate, resultScale)
if err != nil {
    return errnie.Err(
        errnie.Dependency,
        "broker strategy: cannot evaluate trade without Kraken fee data",
        err,
    )
}

// Only reachable when all required fee data is present and valid.
return broker.SubmitOrder(...)
```

The following behavior is prohibited:

```go
pnl, err := price.pnl(...)
if err != nil {
    pnl = decimal.Decimal{} // prohibited
}
```

The system should expose this condition through logs and metrics so missing or stale fee data is operationally visible, but observability must not convert the failure into a successful trade path.

Recommended error categories include:

```go
var (
    ErrFeeRateUnavailable = errors.New("Kraken fee rate unavailable")
    ErrFeeRateStale       = errors.New("Kraken fee rate stale")
    ErrFeeRateInvalid     = errors.New("Kraken fee rate invalid")
)
```

A stale fee value should be treated the same as a missing value unless the application has an explicitly documented and tested validity period based on a successful `TradeVolume` refresh.

## 14. Market-price limitations

Using the ticker best bid or ask produces a top-of-book estimate, not a guarantee that the entire quantity can be closed at that price.

The ticker also provides best-level quantity. If the position quantity exceeds that amount, the estimate ignores slippage and deeper book levels.

Possible levels of sophistication are:

1. **Top-of-book estimate**  
   Use bid/ask and clearly label the result as estimated.

2. **Liquidity warning**  
   Compare position quantity with `bid_qty` or `ask_qty`.

3. **Order-book liquidation estimate**  
   Walk Kraken's level-2 book and calculate the volume-weighted executable price for the full quantity.

The current function implements the first model. Its name and UI should reflect that.

---

## 15. Metadata cache design

Subscribe to the `instrument` channel and maintain a cache keyed by Kraken symbol.

```go
type InstrumentCache interface {
    Pair(symbol string) (PairMetadata, bool)
    Asset(asset string) (AssetMetadata, bool)
}
```

On each calculation:

1. retrieve pair metadata by symbol;
2. retrieve quote-asset metadata using the pair's quote asset;
3. choose the result scale based on the calculation's purpose;
4. reject calculations when metadata is unavailable or stale according to the application's policy.

Instrument updates should replace cached values atomically so calculations do not observe partially updated metadata.

Do not store only the precision values. Retaining base asset, quote asset, increments, minimums, and status supports validation elsewhere.

---

## 16. Tests to add

### Validation tests

- nil position;
- blank symbol;
- mismatched ticker and position symbols;
- zero or negative entry price;
- zero or negative quantity;
- negative entry fee;
- negative exit fee rate;
- zero or negative exit bid/ask;
- unknown position side;
- negative result scale.

### Calculation tests

- profitable long;
- losing long;
- break-even long before fees;
- profitable short;
- losing short;
- zero fee;
- entry fee plus estimated exit fee;
- very small quantity;
- high-precision quantity;
- negative net PnL caused only by fees;
- rounding exactly at a half-unit boundary;
- result scale of zero;
- result scale equal to quote-asset ledger precision.

### Market-side tests

- long uses bid, never ask;
- short uses ask, never bid;
- symbol mismatch is rejected;
- quantity exceeding top-level liquidity triggers a warning or alternate path, if implemented.

### Property-style tests

Useful invariants include:

```text
With zero fees:
    long PnL increases as bid increases

With zero fees:
    short PnL decreases as ask increases

With all other values fixed:
    increasing a fee must never increase net PnL

At exit price equal to entry price:
    net PnL equals negative total fees
```

### Realized-PnL reconciliation

For closed positions, compare the application's result with aggregates from Kraken execution cost and fee events. This is the most valuable integration test because it verifies both arithmetic and event aggregation.

---

## 17. Suggested migration sequence

1. Change quantity and fee-rate fields from `float64` to the decimal type.
2. Change the function to return `(decimal.Decimal, error)`.
3. Store actual entry fees on the position.
4. Rename the function to `estimatedPnL`.
5. add explicit position side and bid/ask selection.
6. Validate ticker/position symbols.
7. Cache `instrument` pair and asset metadata.
8. pass an explicit result scale selected by the caller.
9. add a separate realized-PnL path based on execution cost and fee data.
10. add reconciliation and precision-boundary tests.
11. optionally add order-book-based slippage estimation.

---

## 18. Final recommendation

The most important design decision is to stop treating PnL precision as a property that can be inferred from the scale of the current bid, entry price, and fee-rate operands.

Instead:

- preserve exact intermediate arithmetic;
- use Kraken pair precision and increments for market/order validation;
- use quote-asset ledger precision for internal quote-currency accounting when appropriate;
- use pair `cost_precision` for Kraken pair-cost representations;
- use an explicit display policy for the UI;
- round once at a deliberate boundary;
- use actual execution fees for realized PnL;
- reserve fee-rate multiplication for estimates;
- return errors rather than silently returning zero.

With those changes, the implementation becomes safer, easier to test, clearer about its guarantees, and better aligned with the data Kraken actually publishes.
