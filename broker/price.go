package broker

import (
	"fmt"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Direction string

const (
	BUY  Direction = "buy"
	SELL Direction = "sell"
)

/*
feeRateScale is the working precision for a fee expressed as a fraction. Kraken
quotes tiers to two decimals of a percent, so four more digits keep the
converted rate exact.
*/
const feeRateScale = 8

/*
Price is the broker price surface for symm. It owns fee tiers, ticker cache,
and all money math so the rest of the broker never drifts from Kraken's
precision and executable boundaries.
*/
type Price struct {
	status     types.Status
	api        *websocket.API
	fees       *sync.Map
	tickers    *sync.Map
	normalizer *spot.Normalizer
}

/*
NewPrice wires the price surface to the shared Kraken API.
*/
func NewPrice(api *websocket.API) *Price {
	return &Price{
		status:     types.READY,
		api:        api,
		fees:       &sync.Map{},
		tickers:    &sync.Map{},
		normalizer: api.Normalizer(),
	}
}

/*
Status reports whether the fee surface is ready for executable calculations.
*/
func (price *Price) Status() types.Status {
	return price.status
}

/*
Update refreshes the cached ticker for a symbol and returns the new row.
*/
func (price *Price) Update(ticker *kraken.TickerData) {
	price.tickers.Store(price.api.Normalizer().Name(ticker.Symbol), ticker)
}

/*
Mark returns the price with the taker fee applied.
*/
func (price *Price) Mark(
	symbol string,
	direction Direction,
) *decimal.Decimal {
	var (
		out *decimal.Decimal
		err error
	)

	tick, fee, err := price.getTickAndFee(symbol)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"could not get tick and fee for symbol",
			err,
		))

		return nil
	}

	switch direction {
	case BUY:
		out, err = price.normalizer.FormatPrice(
			symbol, tick.Ask.OffsetPercent(fee.Fee),
		)
	case SELL:
		exitFee := fee.Fee.Mul(tick.Bid)
		out, err = price.normalizer.FormatPrice(
			symbol, tick.Bid.Sub(exitFee),
		)
	default:
		return nil
	}

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid price",
			err,
		))
	}

	return out
}

/*
RiskPlan derives stop geometry for a lot the strategy did not size.

Recovered wallet inventory and any entry that reaches the desk without a plan
still need a boundary. What cannot be reconstructed here is the sizing half of
the coupling — the quantity is already held, so there is no loss budget left to
solve against and MaxLoss is left absent. The distances come from the live book,
which is where the allocator reads them from too, so a recovered lot ends up
defended on the same terms as one this process entered.

An absent plan means the book has not been seen yet, which happens on every
recovered position: the desk adopts wallet inventory before the first ticker
arrives. The regulator holds no geometry until it can be derived, and the
position adopts it on the first tick that prices the symbol.
*/
func (price *Price) RiskPlan(pair kraken.InstrumentPair) types.RiskPlan {
	tick := price.Tick(pair.Symbol)

	if tick == nil || tick.Bid == nil || tick.Ask == nil || tick.Bid.Sign() <= 0 {
		return types.RiskPlan{}
	}

	feeRate, err := price.Fee(pair.Symbol)

	if err != nil {
		return types.RiskPlan{}
	}

	var spread *decimal.Decimal

	if tick.Ask.Cmp(tick.Bid) > 0 {
		spread = tick.Ask.SetScale(feeRateScale).Sub(tick.Bid)
	}

	size := pair.TickSize

	if size.Sign() <= 0 {
		size = pair.PriceIncrement
	}

	return types.NewRiskPlan(types.RiskInputs{
		ReferencePrice: tick.Bid,
		Spread:         spread,
		TickSize:       size.Copy(),
		ExitFeeRate:    feeRate,
		Multiples:      types.DefaultRiskMultiples(),
	})
}

/*
ExecutableMark is the price selling this quantity would realise, and whether the
visible book can actually say.

The top bid is a price for one unit. A position larger than what rests at the
touch does not liquidate there: the part the touch cannot absorb walks down the
book to levels this process cannot see. There is no maintained order book here —
the ticker carries the touch and its size, and nothing below it — so when the
position exceeds that size the honest answer is the touch price plus an explicit
admission that it is only true of the part that fits.

An earlier version charged the unabsorbed fraction one spread and returned the
result as though it were a sweep. That number is not a bound: the next resting
level may be one tick lower or fifty, and a floor defended by an invented VWAP
is defended by liquidity that was never observed. The limited flag exists so
callers refuse to build profit geometry on it rather than quietly trusting it.

The exit fee is left out because the lines this is compared against divide it
out of the price they solve for; charging it in both places would demand a
profit the position has to earn twice.
*/
func (price *Price) ExecutableMark(
	pair kraken.InstrumentPair,
	quantity *decimal.Decimal,
) (mark *decimal.Decimal, limited bool) {
	tick := price.Tick(pair.Symbol)

	if tick == nil || tick.Bid == nil || tick.Bid.Sign() <= 0 {
		return nil, false
	}

	if quantity == nil || quantity.Sign() <= 0 {
		return tick.Bid.Copy(), false
	}

	size := quantity.Float64()

	if size <= 0 || tick.BidQty >= size {
		return tick.Bid.Copy(), false
	}

	return tick.Bid.Copy(), true
}

/*
PnL returns the PnL for a holding, which means the profit or
loss of the holding, including entry fee, and current exit fee.
*/
func (price *Price) PnL(
	pair kraken.InstrumentPair,
	holding *types.Holding,
) *decimal.Decimal {
	if holding == nil || holding.Qty == nil || holding.Qty.Sign() <= 0 || holding.Mark == nil ||
		holding.EntryPrice == nil || holding.EntryFee == nil {
		return nil
	}

	_, fee, err := price.getTickAndFee(pair.Symbol)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"could not get tick and fee for holding",
			err,
		))

		return nil
	}

	/*
		What the lot is worth is what closing it would realise. When the touch
		cannot absorb the whole position that price is unknowable from here, and
		the touch is used anyway — an unrealisable valuation is still the best
		this process can state, and reporting no PnL at all would blank the
		equity line whenever a lot outgrew the top of book. The optimism is
		disclosed rather than hidden: the same limit stops the regulator from
		acting on it.
	*/
	liquidation, _ := price.ExecutableMark(pair, holding.Qty)

	if liquidation == nil {
		return nil
	}

	costScale := int64(pair.CostPrecision)
	grossProceeds := liquidation.SetScale(costScale).Mul(holding.Qty)
	exitFee := fee.Fee.Mul(grossProceeds).SetScale(costScale)
	entryValue := holding.EntryPrice.SetScale(costScale).Mul(holding.Qty)
	entryFee := holding.EntryFee.SetScale(costScale)

	return grossProceeds.
		Sub(exitFee).
		Sub(entryValue).
		Sub(entryFee)
}

/*
ReturnPct returns the return percentage for a holding, which is the PnL divided by the entry value.
*/
func (price *Price) ReturnPct(
	pair kraken.InstrumentPair,
	holding *types.Holding,
) float64 {
	pnl := price.PnL(pair, holding)

	if pnl == nil || holding == nil || holding.EntryPrice == nil || holding.Qty == nil ||
		holding.Qty.Sign() <= 0 {
		return 0
	}

	entryValue := decimal.ExactMul(holding.EntryPrice, holding.Qty)
	return pnl.Div(entryValue).Float64()
}

/*
WithFriction returns the price with the taker fee applied.
Pass the direction of the trade to get the correct fee for buys and sells.
*/
func (price *Price) WithFriction(
	symbol string,
	direction Direction,
	volume *decimal.Decimal,
) *decimal.Decimal {
	var (
		out *decimal.Decimal
		err error
	)

	tick, fee, err := price.getTickAndFee(symbol)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"could not get tick and fee for symbol",
			err,
		))

		return nil
	}

	switch direction {
	case BUY:
		out, err = price.normalizer.FormatPrice(
			symbol, tick.Ask.Mul(volume).OffsetPercent(fee.Fee),
		)
	case SELL:
		grossProceeds := tick.Bid.Mul(volume)
		exitFee := fee.Fee.Mul(grossProceeds)
		out, err = price.normalizer.FormatPrice(
			symbol, grossProceeds.Sub(exitFee),
		)
	default:
		return nil
	}

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid price",
			err,
		))
	}

	return out
}

/*
Quantity returns the quantity that can be purchased for a given notional amount
at the current market price with taker fees applied.
*/
func (price *Price) Quantity(
	symbol string,
	notional *decimal.Decimal,
) (*decimal.Decimal, error) {
	tick, fee, err := price.getTickAndFee(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"could not get tick and fee for symbol",
			err,
		))
	}

	return price.normalizer.FormatSize(
		symbol, notional.Div(tick.Ask.OffsetPercent(fee.Fee)),
	)
}

/*
Fee returns the taker fee for a symbol.
*/
func (price *Price) Fee(symbol string) (*decimal.Decimal, error) {
	fee, err := price.getFee(symbol)

	if err != nil {
		return nil, err
	}

	return fee.Fee, nil
}

/*
Tick returns the latest cached ticker row for a symbol.
*/
func (price *Price) Tick(symbol string) *kraken.TickerData {
	value, ok := price.tickers.Load(price.api.Normalizer().Name(symbol))

	if !ok {
		return nil
	}

	return value.(*kraken.TickerData)
}

/*
asRate converts a Kraken percent-quoted fee tier into the fraction every
consumer here expects, since OffsetPercent multiplies by (1 + fee) and the
strategy adds fees straight onto notional.

The scale is widened before dividing because division keeps the receiver's
scale, and "0.26" carries only two decimals: dividing it as-is rounds a 0.26%
taker fee to zero rather than to 0.0026.
*/
func asRate(percent *decimal.Decimal) *decimal.Decimal {
	if percent == nil {
		return nil
	}

	return percent.SetScale(feeRateScale).Div(decimal.NewFromInt64(100))
}

/*
GetFees loads TradeVolume taker fee tiers for the requested symbols and makes
them executable for later quantity, fee, and PnL calculations.
*/
func (price *Price) GetFees(symbols []string) error {
	errnie.Info(fmt.Sprintf("getting fees for %d symbols", len(symbols)))

	tradeVolumeResult, err := price.api.TradeVolume(symbols)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"trade volume: failed to fetch",
			err,
		))
	}

	for symbol, fee := range tradeVolumeResult.Fees {
		fee.Fee = asRate(fee.Fee)
		fee.Minfee = asRate(fee.Minfee)
		fee.Maxfee = asRate(fee.Maxfee)

		price.fees.Store(price.api.Normalizer().Name(symbol), fee)
	}

	price.status = types.READY
	return nil
}

func (price *Price) getTickAndFee(symbol string) (
	*kraken.TickerData, *kraken.TradeVolumeFee, error,
) {
	fee, err := price.getFee(symbol)

	if err != nil {
		return nil, nil, err
	}

	tick := price.Tick(symbol)

	if tick == nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"ticker not found",
			nil,
		))
	}

	return tick, fee, nil
}

func (price *Price) getFee(symbol string) (*kraken.TradeVolumeFee, error) {
	normName := symbol

	if price.api != nil && price.api.Normalizer() != nil {
		normName = price.api.Normalizer().Name(symbol)
	}

	found, ok := price.fees.Load(normName)

	if (!ok || found == nil) && price.api != nil {
		_ = price.GetFees([]string{symbol})
		found, ok = price.fees.Load(normName)
	}

	if !ok || found == nil {
		return nil, errnie.Err(
			errnie.NotFound,
			"fee not found for "+symbol,
			nil,
		)
	}

	fee, ok := found.(kraken.TradeVolumeFee)

	if !ok {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"invalid fee type",
			nil,
		)
	}

	return &fee, nil
}
