package broker

import (
	"math/big"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Pricing owns exact taker economics and venue lot constraints for both live
quotes and virtual execution. Percentages are converted once; rational
arithmetic preserves all input precision until the presentation boundary.
*/
type Pricing struct {
	Rate, Lot, Minimum, CostMinimum big.Rat
}

/* Configure validates the supplied venue contract before an account uses it. */
func (pricing *Pricing) Configure(pair kraken.InstrumentPair, percent *decimal.Decimal) error {
	if pair.QtyIncrement == nil || pair.QtyMin == nil || pair.CostMin == nil ||
		pair.QtyIncrement.Sign() <= 0 || pair.QtyMin.Sign() <= 0 || pair.CostMin.Sign() <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "pricing: positive venue lot and minimums required", nil))
	}

	if err := pricing.SetFee(percent); err != nil {
		return err
	}
	pricing.Lot.Set(pair.QtyIncrement.Rat())
	pricing.Minimum.Set(pair.QtyMin.Rat())
	pricing.CostMinimum.Set(pair.CostMin.Rat())
	return nil
}

/* SetFee accepts Kraken percentage units, including a zero-fee schedule. */
func (pricing *Pricing) SetFee(percent *decimal.Decimal) error {
	if percent == nil || percent.Sign() < 0 || percent.Cmp(decimalHundred) >= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "pricing: taker percentage must be in [0, 100)", nil))
	}
	pricing.Rate.Quo(percent.Rat(), big.NewRat(100, 1))
	return nil
}

/* Fee calculates the exact quote-currency fee, allowing output to alias gross. */
func (pricing *Pricing) Fee(output, gross *big.Rat) *big.Rat {
	return output.Mul(gross, &pricing.Rate)
}

/* Total adds the entry fee or subtracts the exit fee, allowing input aliasing. */
func (pricing *Pricing) Total(output, gross *big.Rat, buy bool) *big.Rat {
	var fee big.Rat
	pricing.Fee(&fee, gross)

	if buy {
		return output.Add(gross, &fee)
	}
	return output.Sub(gross, &fee)
}

/* Floor rounds down to a venue lot without a binary floating-point conversion. */
func (pricing *Pricing) Floor(output, quantity *big.Rat) *big.Rat {
	output.Quo(quantity, &pricing.Lot)
	output.Num().Quo(output.Num(), output.Denom())
	output.SetInt(output.Num())
	return output.Mul(output, &pricing.Lot)
}

/*
Sweep prices displayed depth in execution order. Cash is optional for an
unbudgeted quote; a budgeted buy requires configured venue lots. Observed and
limit retain and constrain causal depth for virtual IOC fills. Partial depth
is returned explicitly as a quantity, never priced as a complete fill.
*/
func (pricing *Pricing) Sweep(
	book *spotbook.Book, requested, cash *big.Rat, buy bool,
	observed, limit *DepthLadder,
) (quantity, gross *big.Rat) {
	quantity, gross = new(big.Rat), new(big.Rat)
	level := book.Bids.High

	if buy {
		level = book.Asks.Low
	}
	var remaining, available, fill, cost, affordable big.Rat
	remaining.Set(requested)

	if cash != nil {
		available.Set(cash)
	}

	for level != nil && remaining.Sign() > 0 {
		next := level.Lower

		if buy {
			next = level.Higher
		}
		unit := level.Price.Rat()
		fill.Set(level.Quantity.Rat())
		observed.Record(unit, &fill)
		fill.Set(limit.Surviving(unit, &fill))

		if fill.Sign() <= 0 {
			level = next
			continue
		}

		if remaining.Cmp(&fill) < 0 {
			fill.Set(&remaining)
		}

		if buy && cash != nil {
			pricing.Total(&cost, unit, true)
			affordable.Quo(&available, &cost)
			pricing.Floor(&affordable, &affordable)

			if affordable.Sign() == 0 {
				break
			}

			if affordable.Cmp(&fill) < 0 {
				fill.Set(&affordable)
			}
		}
		cost.Mul(unit, &fill)
		quantity.Add(quantity, &fill)
		gross.Add(gross, &cost)
		remaining.Sub(&remaining, &fill)

		if buy && cash != nil {
			pricing.Total(&cost, &cost, true)
			available.Sub(&available, &cost)
		}
		level = next
	}
	return quantity, gross
}

/*
EntryCost derives fees and break-even from the exact swept entry notional.
It never uses a future price. Conversion to Decimal happens only for the
existing published quote contract; account arithmetic remains rational.
*/
func (pricing *Pricing) EntryCost(notional, ask, bid, quantity *decimal.Decimal) *types.EntryCost {
	gross := notional.Rat()
	entry := UnitPrice(notional, quantity)
	entryFee := pricing.Fee(new(big.Rat), gross)
	total := pricing.Total(new(big.Rat), gross, true)
	exitFactor := new(big.Rat).Sub(big.NewRat(1, 1), &pricing.Rate)
	breakEvenGross := new(big.Rat).Quo(total, exitFactor)
	exitFee := pricing.Fee(new(big.Rat), breakEvenGross)
	midpoint := PriceDecimal(new(big.Rat).Quo(new(big.Rat).Add(ask.Rat(), bid.Rat()), big.NewRat(2, 1)))
	impact := subtractAmount(entry, ask)

	if impact.Sign() < 0 {
		impact = decimalZero.Copy()
	}
	return &types.EntryCost{
		EntryPrice: entry.Copy(), BestAsk: ask.Copy(), BestBid: bid.Copy(), Midpoint: midpoint,
		GrossNotional: PriceDecimal(gross), EntryFee: PriceDecimal(entryFee),
		ExitFeeAtBreakEven: PriceDecimal(exitFee), RoundTripFees: PriceDecimal(new(big.Rat).Add(entryFee, exitFee)),
		Spread: subtractAmount(ask, midpoint), Impact: impact,
		BreakEven: PriceDecimal(new(big.Rat).Quo(breakEvenGross, quantity.Rat())),
	}
}

/*
PriceDecimal preserves finite decimal results exactly. Repeating quotients use
at least the SDK's published DefaultScale, with its bankers rounding. This is
a quote/presentation boundary; execution budgets stay rational.
*/
func PriceDecimal(value *big.Rat) *decimal.Decimal {
	precision, _ := value.FloatPrec()
	scale := int64(max(precision, decimal.DefaultScale))
	return decimal.NewFromBigInt(value.Num()).SetScale(scale).Div(decimal.NewFromBigInt(value.Denom()).SetScale(scale))
}

/* Notional multiplies full input precision before publishing a decimal amount. */
func Notional(unit, quantity *decimal.Decimal) *decimal.Decimal {
	return PriceDecimal(new(big.Rat).Mul(unit.Rat(), quantity.Rat()))
}

/* UnitPrice computes a whole-order VWAP without rounding the cumulative cost first. */
func UnitPrice(cost, quantity *decimal.Decimal) *decimal.Decimal {
	return PriceDecimal(UnitPriceRat(cost.Rat(), quantity.Rat()))
}

/* Prorate retains the share of an authoritative cost or fee after a partial sale. */
func Prorate(amount, remaining, original *decimal.Decimal) *decimal.Decimal {
	value := new(big.Rat).Mul(amount.Rat(), remaining.Rat())
	return PriceDecimal(value.Quo(value, original.Rat()))
}

/* ReturnFraction compares net profit with fee-inclusive committed capital. */
func ReturnFraction(profit, capital *decimal.Decimal) *decimal.Decimal {
	return PriceDecimal(new(big.Rat).Quo(profit.Rat(), capital.Rat()))
}

/* NotionalRat calculates a rational quote notional, allowing output aliasing. */
func NotionalRat(output, unit, quantity *big.Rat) *big.Rat {
	return output.Mul(unit, quantity)
}

/* Affordable bounds requested units by cash before an ascending ask sweep. */
func (pricing *Pricing) Affordable(cash, unit *big.Rat) *big.Rat {
	total := pricing.Total(new(big.Rat), unit, true)
	return new(big.Rat).Quo(cash, total)
}

func addAmount(left, right *decimal.Decimal) *decimal.Decimal {
	return PriceDecimal(new(big.Rat).Add(left.Rat(), right.Rat()))
}

func subtractAmount(left, right *decimal.Decimal) *decimal.Decimal {
	return PriceDecimal(new(big.Rat).Sub(left.Rat(), right.Rat()))
}

/* Surface reports total visible capacity and prices only a complete liquidation. */
func (pricing *Pricing) Surface(
	book *spotbook.Book, requested, floor *decimal.Decimal, surface *types.ExecutionSurface,
) {
	surface.FullyExecutable = false
	surface.ExecutableValue, surface.ExecutableVWAP = nil, nil
	var capacity, coverage big.Rat

	for bid := book.Bids.High; bid != nil; bid = bid.Lower {
		capacity.Add(&capacity, bid.Quantity.Rat())

		if floor != nil && bid.Price.Cmp(floor) >= 0 {
			coverage.Add(&coverage, bid.Quantity.Rat())
		}
	}
	surface.ExecutableQty = PriceDecimal(&capacity)
	surface.FloorCoverageQty = PriceDecimal(&coverage)
	filled, gross := pricing.Sweep(book, requested.Rat(), nil, false, nil, nil)

	if filled.Cmp(requested.Rat()) < 0 {
		return
	}
	surface.FullyExecutable = true
	surface.ExecutableValue = PriceDecimal(pricing.Total(new(big.Rat), gross, false))
	surface.ExecutableVWAP = PriceDecimal(gross.Quo(gross, filled))
}

/*
executionVWAP resolves the authoritative whole-order realized VWAP for a
Kraken ExecutionData. It prefers the explicit AvgPrice field (Kraken's own
whole-order average) and falls back to the cumulative equivalent
CumCost/CumQty only when AvgPrice is absent. It never uses the individual
LastPrice, which is a single fill and not the whole-order economics a closed
position's exit must be marked by.
*/
func executionVWAP(execution kraken.ExecutionData) *decimal.Decimal {
	if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
		return execution.AvgPrice.Copy()
	}

	if execution.CumCost == nil || execution.CumCost.Sign() <= 0 ||
		execution.CumQty == nil || execution.CumQty.Sign() <= 0 {
		return nil
	}

	return UnitPrice(execution.CumCost, execution.CumQty)
}

/* UnitPriceRat retains the exact cumulative quotient for execution feedback. */
func UnitPriceRat(cost, quantity *big.Rat) *big.Rat {
	return new(big.Rat).Quo(cost, quantity)
}

/*
OrderQuantity floors an exact cash/unit quotient to the REST pair's venue lot.
The SDK FormatSize rounds to nearest; applying it before this floor can spend
more than the supplied budget at a lot boundary. Unit includes the taker fee.
*/
func OrderQuantity(cash, unit *decimal.Decimal, pair *spot.AssetPair) (*decimal.Decimal, error) {
	if pair.LotDecimals < 0 || pair.LotMultiplier <= 0 || unit.Sign() <= 0 || cash.Sign() < 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "pricing: positive unit and venue lot, and nonnegative cash required", nil))
	}
	var pricing Pricing
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(pair.LotDecimals)), nil)
	pricing.Lot.SetFrac(big.NewInt(int64(pair.LotMultiplier)), denominator)
	quantity := UnitPriceRat(cash.Rat(), unit.Rat())
	pricing.Floor(quantity, quantity)
	return PriceDecimal(quantity).SetScale(int64(pair.LotDecimals)), nil
}
