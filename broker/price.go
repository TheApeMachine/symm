package broker

import (
	"math"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

var (
	// two is the mid divisor reused on the mark hot path.
	two = decimal.NewFromInt64(2)
	// hundred converts Kraken's percentage fee into a unit fraction.
	hundred = decimal.NewFromInt64(100)

	floorQuoRounding = func(integer *big.Int, factor *big.Int) *big.Int {
		return new(big.Int).Quo(integer, factor)
	}
)

/*
floorScale truncates toward zero rather than banker's rounding so a buy cannot
inflate past the funded budget.
*/
func (price *Price) floorScale(value *decimal.Decimal, scale int64) *decimal.Decimal {
	return value.Copy().SetRounding(floorQuoRounding).SetScale(scale)
}

/*
Price is the broker price surface for symm. It is the single source of
truth for all pricing and fee information, and calculations. No monetary
value should be calculated outside of this package.

Important considerations:

  - Kraken has specific requirements around decimal precision, which can
    be found on the Instrument data. This is the only correct source of
    precision information.
  - The Kraken SDK already provides us with a decimal.Decimal type, which
    is the correct type to use for all monetary values. No monetary calculation
    may ever be performed using Float64.
*/
type Price struct {
	status  atomic.Value
	api     *websocket.API
	mu      sync.RWMutex
	fees    map[string]kraken.TradeVolumeFee
	tickers map[string]*kraken.TickerData
}

/*
NewPrice wires the price stream to the shared Kraken API.
*/
func NewPrice(
	api *websocket.API,
) *Price {
	price := &Price{
		api:     api,
		fees:    make(map[string]kraken.TradeVolumeFee),
		tickers: make(map[string]*kraken.TickerData),
	}

	price.status.Store(types.INITIALIZING)

	return price
}

func (price *Price) Initialize() error {
	price.status.Store(types.READY)

	return nil
}

func (price *Price) Status() types.Status {
	status := price.status.Load()

	if status == nil {
		return types.INITIALIZING
	}

	return status.(types.Status)
}

/*
Last returns the cached last trade price for symbol.
*/
func (price *Price) Last(symbol string) (*decimal.Decimal, error) {
	if price == nil || price.tickers == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "price: ticker cache unavailable", nil,
		))
	}

	price.mu.RLock()
	ticker, ok := price.tickers[symbol]
	price.mu.RUnlock()

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "price: no ticker for "+symbol, nil,
		))
	}

	if ticker == nil || ticker.Last == nil || ticker.Last.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "price: last unavailable for "+symbol, nil,
		))
	}

	return ticker.Last.Copy(), nil
}

/*
TickerAck refreshes the mark cache from a typed ticker frame. Desk routes marks
to open lots after this cache update.
*/
func (price *Price) TickerAck(ticker *kraken.Ticker) {
	if errnie.Error(kraken.Validate(ticker)) != nil {
		return
	}

	price.mu.Lock()

	for _, item := range ticker.Data {
		copyItem := item
		price.tickers[item.Symbol] = &copyItem
	}

	price.mu.Unlock()
	price.status.Store(types.READY)
}

/*
Snapshot returns ticker rows for the requested identity set and names every
symbol that has not produced a ticker row yet.
*/
func (price *Price) Snapshot(
	symbols []string,
) ([]kraken.TickerData, []string) {
	rows := make(
		[]kraken.TickerData,
		0,
		len(symbols),
	)

	missing := make([]string, 0)

	for _, symbol := range symbols {
		price.mu.RLock()
		value, ok := price.tickers[symbol]
		price.mu.RUnlock()

		if !ok {
			missing = append(
				missing,
				symbol,
			)

			continue
		}

		rows = append(
			rows,
			*value,
		)
	}

	return rows, missing
}

/*
EachLast visits cached last prices so test harnesses can align paper marks
with the same quotes Allocator used for sizing.
*/
func (price *Price) EachLast(visit func(symbol string, last float64)) {
	if price == nil || price.tickers == nil || visit == nil {
		return
	}

	price.mu.RLock()
	defer price.mu.RUnlock()

	for symbol, ticker := range price.tickers {
		if ticker == nil || ticker.Last == nil || ticker.Last.Sign() <= 0 {
			continue
		}

		visit(symbol, ticker.Last.Float64())
	}
}

/*
Get returns the latest cached ticker row for symbol. Missing or not-ready
lookups return an error without logging — subscription lag is expected and
must not flood the UI error overlay through errnie.Error.
*/
func (price *Price) Get(
	symbol string,
) (*kraken.TickerData, error) {
	price.mu.RLock()
	ticker, ok := price.tickers[symbol]
	price.mu.RUnlock()

	if !ok {
		return nil, errnie.Err(
			errnie.NotFound,
			"ticker not found for symbol "+symbol,
			nil,
		)
	}

	return ticker, nil
}

/*
GetFees loads TradeVolume taker fee tiers for symbols.
*/
func (price *Price) GetFees(
	symbols []string,
) error {
	errnie.Info("getting fees for symbols: " + strings.Join(symbols, ", "))

	tradeVolumeResult, err := price.api.TradeVolume(symbols)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trade volume",
			err,
		))
	}

	fees := make(map[string]kraken.TradeVolumeFee, len(symbols))

	for _, symbol := range symbols {
		fee, ok := tradeVolumeResult.Fees[symbol]

		if !ok || fee.Fee == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"trade volume response missing taker fee for "+symbol,
				nil,
			))
		}

		fees[symbol] = fee
	}

	for symbol, fee := range fees {
		price.fees[symbol] = fee
	}

	price.status.Store(types.READY)
	return nil
}

/*
RememberFee stores one already-validated taker fee tier. Paper sessions and
tests use it when TradeVolume was resolved out of band; live Subscribe still
loads tiers through GetFees.
*/
func (price *Price) RememberFee(
	symbol string,
	fee kraken.TradeVolumeFee,
) error {
	if price == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"price is required",
			nil,
		))
	}

	if symbol == "" || fee.Fee == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"price: symbol and taker fee are required",
			nil,
		))
	}

	price.fees[symbol] = fee
	price.status.Store(types.READY)

	return nil
}

/*
Fraction returns the cached taker fee as a unitless decimal fraction for
strategy utility math. Kraken stores Fee as a percent; this is the single
place that converts percent to fraction so callers never divide by 100.
*/
func (price *Price) Fraction(symbol string) (*decimal.Decimal, error) {
	tradeVolume, err := price.FeeRate(symbol)

	if err != nil {
		return nil, err
	}

	if tradeVolume.Fee == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"fee rate missing fee for symbol "+symbol,
			nil,
		))
	}

	feePrecision := tradeVolume.Fee.GetScale() + 2

	return tradeVolume.Fee.SetScale(feePrecision).Div(hundred), nil
}

/*
FeeRate returns the cached TradeVolume taker fee tier for symbol.

Kraken returns the Fee field as a percentage.

For example:

	"0.1000" means 0.1 percent.
*/
func (price *Price) FeeRate(
	symbol string,
) (kraken.TradeVolumeFee, error) {
	if price.Status() != types.READY {
		return kraken.TradeVolumeFee{}, errnie.Error(
			errnie.Err(
				errnie.NotImplemented,
				"price not ready",
				nil,
			),
		)
	}

	price.mu.RLock()
	rate, ok := price.fees[symbol]
	price.mu.RUnlock()

	if !ok {
		return kraken.TradeVolumeFee{}, errnie.Error(
			errnie.Err(
				errnie.NotFound,
				"fee rate not found for symbol "+symbol,
				nil,
			),
		)
	}

	return rate, nil
}

/*
Notional converts price and quantity into quote-currency value.

For example:

	Price:    50,000 USD per BTC
	Quantity: 0.1 BTC
	Notional: 5,000 USD

The instrument supplies the only valid price and cost precision. Price is the
receiver so a quantity's executable floor-rounding policy cannot leak into
quote-currency rounding. The result remains at authoritative cost precision.
*/
func (price *Price) Notional(
	instrument *kraken.InstrumentPair,
	rate *decimal.Decimal,
	quantity *decimal.Decimal,
) *decimal.Decimal {
	costScale := int64(instrument.CostPrecision)

	return price.Mul(rate, quantity).SetScale(costScale)
}

/*
Fee calculates the taker fee for a quote-currency amount.

For example:

	Notional: 5,000 USD
	Fee rate: 0.1 percent
	Fee:      5 USD
*/
func (price *Price) Fee(
	instrument *kraken.InstrumentPair,
	amount *decimal.Decimal,
) *decimal.Decimal {
	tradeVolume, err := price.FeeRate(instrument.Symbol)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trade volume fee rate for "+instrument.Symbol,
			err,
		))

		return nil
	}

	feePrecision := tradeVolume.Fee.GetScale() + 2
	fraction := tradeVolume.Fee.SetScale(feePrecision).Div(hundred)
	costScale := int64(instrument.CostPrecision)
	calculationScale := max(costScale, feePrecision)

	return amount.SetScale(calculationScale).
		Mul(fraction).
		SetScale(costScale)
}

/*
Taker returns the estimated all-in taker purchase cost for a quantity at the
current ask, which is the executable boundary for a market buy.

The result is:

	current notional + one taker fee
*/
func (price *Price) Taker(
	instrument *kraken.InstrumentPair,
	quantity *decimal.Decimal,
) (*decimal.Decimal, error) {
	ticker, err := price.Get(instrument.Symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get ticker for "+instrument.Symbol,
			err,
		))
	}

	if ticker.Ask == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"ticker has no ask price for "+instrument.Symbol,
			nil,
		))
	}

	notional := price.Notional(
		instrument,
		ticker.Ask,
		quantity,
	)

	fee := price.Fee(instrument, notional)

	if fee == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate taker fee for "+instrument.Symbol,
			nil,
		))
	}

	return notional.Add(fee), nil
}

/*
RecordFill appends one immutable fill and derives entry/exit economics from the
ledger. Wallet Qty stays under Balance.syncWallet — fills never write inventory.
*/
func (price *Price) RecordFill(
	instrument *kraken.InstrumentPair,
	holding *types.Holding,
	data kraken.ExecutionData,
	ledger *[]Fill,
) Fill {
	fill := Fill{
		ExecID: data.ExecID,
		Side:   strings.ToLower(data.Side),
	}

	if data.LastQty != nil {
		fill.Qty = data.LastQty.Copy()
	}

	if data.LastPrice != nil {
		fill.Price = data.LastPrice.Copy()
		holding.Mark = data.LastPrice.Copy()
	}

	if data.FeeUsdEquiv != nil && instrument.Quote == "USD" {
		fill.Fee = data.FeeUsdEquiv.Copy()
	}

	// Wallet cash already paid the venue fee. Without a fee on the fill,
	// EntryFee stays nil, PnL omits entry friction, and UI equity undershoots
	// initial+PnL by roughly the paid fee (measured ~entry fee on live lots).
	if fill.Fee == nil && fill.Price != nil && fill.Qty != nil {
		errnie.Warn(
			"execution missing fee for " + instrument.Symbol + "; deriving taker fee",
		)

		notional := price.Notional(instrument, fill.Price, fill.Qty)
		fill.Fee = price.Fee(instrument, notional)

		if fill.Fee == nil {
			errnie.Warn(
				"price.Fee returned nil: no taker fee schedule for " + instrument.Symbol,
			)
		}
	}

	*ledger = append(*ledger, fill)
	price.deriveEconomics(holding, *ledger)

	if holding.Qty != nil && holding.Qty.Sign() > 0 {
		_ = price.Mark(instrument, holding)
	}

	return fill
}

/*
Fill applies one execution print onto a holding without a caller-owned ledger.
*/
func (price *Price) Fill(
	instrument *kraken.InstrumentPair,
	holding *types.Holding,
	data kraken.ExecutionData,
) {
	var ledger []Fill
	price.RecordFill(instrument, holding, data, &ledger)
}

/*
deriveEconomics recomputes weighted entry, cumulative fees, and exit marks from
immutable fills. Inventory quantity is never written here.
*/
func (price *Price) deriveEconomics(holding *types.Holding, fills []Fill) {
	var (
		entryNotional *decimal.Decimal
		entryQty      *decimal.Decimal
		entryFee      *decimal.Decimal
		exitNotional  *decimal.Decimal
		exitQty       *decimal.Decimal
		exitFee       *decimal.Decimal
	)

	for _, fill := range fills {
		if fill.Qty == nil || fill.Price == nil {
			continue
		}

		notional := price.Mul(fill.Price, fill.Qty)

		switch fill.Side {
		case "buy":
			entryNotional = addNullable(entryNotional, notional)
			entryQty = addNullable(entryQty, fill.Qty)
			entryFee = addNullable(entryFee, fill.Fee)
		case "sell":
			exitNotional = addNullable(exitNotional, notional)
			exitQty = addNullable(exitQty, fill.Qty)
			exitFee = addNullable(exitFee, fill.Fee)
		}
	}

	if entryQty != nil && entryQty.Sign() > 0 && entryNotional != nil {
		holding.EntryPrice = price.Div(entryNotional, entryQty)
	}

	if entryFee != nil {
		holding.EntryFee = entryFee
	}

	if exitQty != nil && exitQty.Sign() > 0 && exitNotional != nil {
		holding.ExitPrice = price.Div(exitNotional, exitQty)
		at := time.Now().UTC()
		holding.ExitAt = &at
	}

	if exitFee != nil {
		holding.ExitFee = exitFee
	}
}

func addNullable(left, right *decimal.Decimal) *decimal.Decimal {
	if right == nil {
		return left
	}

	if left == nil {
		return right.Copy()
	}

	return left.Add(right)
}

/*
WithFriction returns flatten-now PnL for a remaining lot:

	(bid notional − exit taker fee) − (entry notional + entry fee)

Incomplete lots return nil without logging — zero qty after a full exit is
expected and must not flood the UI through errnie.Error.
*/
func (price *Price) WithFriction(
	instrument *kraken.InstrumentPair,
	quantity *decimal.Decimal,
	entry *decimal.Decimal,
	entryFee *decimal.Decimal,
) (*decimal.Decimal, error) {
	if quantity == nil || entry == nil || quantity.Sign() <= 0 {
		return nil, nil
	}

	ticker, err := price.Get(instrument.Symbol)

	if err != nil {
		return nil, err
	}

	if ticker.Bid == nil {
		return nil, errnie.Err(
			errnie.NotFound,
			"ticker has no bid price for "+instrument.Symbol,
			nil,
		)
	}

	return price.frictionAt(instrument, quantity, entry, entryFee, ticker.Bid)
}

/*
frictionAt scores flatten-now PnL at an explicit executable bid.
*/
func (price *Price) frictionAt(
	instrument *kraken.InstrumentPair,
	quantity *decimal.Decimal,
	entry *decimal.Decimal,
	entryFee *decimal.Decimal,
	bid *decimal.Decimal,
) (*decimal.Decimal, error) {
	if quantity == nil || entry == nil || bid == nil || quantity.Sign() <= 0 {
		return nil, nil
	}

	exitNotional := price.Notional(instrument, bid, quantity)
	exitFee := price.Fee(instrument, exitNotional)

	if exitFee == nil {
		return nil, errnie.Err(
			errnie.Internal,
			"failed to calculate exit fee for "+instrument.Symbol,
			nil,
		)
	}

	cost := price.Notional(instrument, entry, quantity)

	if entryFee != nil {
		cost = cost.Add(entryFee)
	}

	return exitNotional.Sub(exitFee).Sub(cost), nil
}

/*
Mark stamps executable bid (flatten-now PnL), mid/last StopMark for stop
geometry, and return onto a holding. The UI labels Mark as bid so it is not
confused with Kraken chart last. When the ticker book is not yet warm after a
fill, the last trade mark on the holding is used so the desk does not publish
a zero-PnL shell until the next tick.
*/
func (price *Price) Mark(
	instrument *kraken.InstrumentPair,
	holding *types.Holding,
) error {
	if price == nil || instrument == nil || holding == nil {
		return nil
	}

	holding.PnL = nil
	holding.ReturnPct = nil

	bid := (*decimal.Decimal)(nil)
	ticker, tickerErr := price.Get(instrument.Symbol)

	if tickerErr == nil && ticker != nil && ticker.Bid != nil {
		bid = ticker.Bid
		holding.Mark = ticker.Bid.Copy()
	}

	if bid == nil {
		bid = holding.Mark
	}

	holding.StopMark = price.geometryMark(ticker, holding)

	if bid == nil || holding.EntryPrice == nil ||
		holding.Qty == nil || holding.Qty.Sign() <= 0 {
		return tickerErr
	}

	pnl, err := price.frictionAt(
		instrument,
		holding.Qty,
		holding.EntryPrice,
		holding.EntryFee,
		bid,
	)

	if err != nil || pnl == nil {
		return err
	}

	holding.PnL = pnl
	basis := price.Notional(instrument, holding.EntryPrice, holding.Qty)

	if holding.EntryFee != nil {
		basis = basis.Add(holding.EntryFee)
	}

	if basis.Sign() <= 0 {
		return nil
	}

	pct := price.Div(pnl, basis).Float64()

	if math.IsNaN(pct) || math.IsInf(pct, 0) {
		return nil
	}

	holding.ReturnPct = &pct

	return nil
}

/*
geometryMark prefers touch mid for stop geometry so bid–ask cross after an ask
entry is not treated as adverse alpha. Falls back to last only — never bid —
so a missing ask cannot invent a spread-wide stop breach.
*/
func (price *Price) geometryMark(
	ticker *kraken.TickerData,
	_ *types.Holding,
) *decimal.Decimal {
	if price != nil && ticker != nil && ticker.Bid != nil && ticker.Ask != nil &&
		ticker.Bid.Sign() > 0 && ticker.Ask.Sign() > 0 {
		sum := ticker.Bid.Add(ticker.Ask)

		if mid := price.Div(sum, two); mid != nil && mid.Sign() > 0 {
			return mid
		}
	}

	if ticker != nil && ticker.Last != nil && ticker.Last.Sign() > 0 {
		return ticker.Last.Copy()
	}

	return nil
}

/*
Prorate scales an amount by remain/total on the Price arithmetic surface.
*/
func (price *Price) Prorate(
	amount, remain, total *decimal.Decimal,
) *decimal.Decimal {
	if price == nil || amount == nil || remain == nil || total == nil ||
		total.Sign() <= 0 {
		return amount
	}

	return price.Div(price.Mul(amount, remain), total)
}

/*
ReferencePrice returns the live ask price for a symbol.
*/
func (price *Price) ReferencePrice(
	pair *kraken.InstrumentPair,
) (*decimal.Decimal, error) {
	ticker, err := price.Get(pair.Symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get ticker for "+pair.Symbol,
			err,
		))
	}

	if ticker.Ask == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"ticker has no ask price for "+pair.Symbol,
			nil,
		))
	}

	return ticker.Ask, nil
}

/*
Quantity converts a quote budget into an instrument-quantized base quantity at
the live ask after a mandatory taker fee, using integer binary search on the
qty_increment lattice for the largest affordable lot.
*/
func (price *Price) Quantity(
	pair *kraken.InstrumentPair,
	budget *decimal.Decimal,
) (*decimal.Decimal, error) {
	ticker, err := price.Get(pair.Symbol)

	if err != nil || ticker.Ask == nil || ticker.Ask.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quantity: ask unavailable for "+pair.Symbol,
			err,
		))
	}

	if budget == nil || budget.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"quantity: non-positive budget for "+pair.Symbol,
			nil,
		))
	}

	if pair.CostMin != nil && pair.CostMin.Sign() > 0 && budget.Cmp(pair.CostMin) < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"quantity: budget "+budget.String()+" below cost_min "+
				pair.CostMin.String()+" for "+pair.Symbol,
			nil,
		))
	}

	fee, feeErr := price.Fraction(pair.Symbol)

	if feeErr != nil || fee == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quantity: taker fee mandatory for "+pair.Symbol,
			feeErr,
		))
	}

	if pair.QtyMin == nil || pair.QtyMin.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"quantity: instrument qty_min unavailable for "+pair.Symbol,
			nil,
		))
	}

	if pair.QtyIncrement == nil || pair.QtyIncrement.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"quantity: instrument qty_increment unavailable for "+pair.Symbol,
			nil,
		))
	}

	minimum := price.Quantize(pair, pair.QtyMin)

	if minimum == nil || minimum.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"quantity: instrument qty_min is not executable for "+pair.Symbol,
			nil,
		))
	}

	minCost, minErr := price.Taker(pair, minimum)

	if minErr != nil || minCost == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"quantity: taker cost unavailable for "+pair.Symbol,
			minErr,
		))
	}

	if minCost.Cmp(budget) > 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"quantity: budget "+budget.String()+" below instrument minimum cost "+
				minCost.String()+" for "+pair.Symbol,
			nil,
		))
	}

	unit := price.Mul(ticker.Ask, decimal.NewFromInt64(1).Add(fee))
	upper := price.Quantize(pair, price.Div(budget, unit))

	if upper == nil || upper.Cmp(minimum) < 0 {
		upper = minimum.Copy()
	}

	minSteps := price.DivFloor(minimum, pair.QtyIncrement, 0)
	maxSteps := price.DivFloor(upper, pair.QtyIncrement, 0)

	if maxSteps.Cmp(minSteps) < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"quantity: empty increment lattice for "+pair.Symbol,
			nil,
		))
	}

	low := minSteps.Int64()
	high := maxSteps.Int64()

	for low < high {
		mid := (low + high + 1) / 2
		candidate := price.Mul(decimal.NewFromInt64(mid), pair.QtyIncrement)
		cost, costErr := price.Taker(pair, candidate)

		if costErr != nil || cost == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Internal,
				"quantity: taker cost unavailable for "+pair.Symbol,
				costErr,
			))
		}

		if cost.Cmp(budget) <= 0 {
			low = mid
			continue
		}

		high = mid - 1
	}

	quantity := price.Mul(decimal.NewFromInt64(low), pair.QtyIncrement)
	cost, costErr := price.Taker(pair, quantity)

	if costErr != nil || cost == nil || cost.Cmp(budget) > 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"quantity: could not fit budget "+budget.String()+" for "+pair.Symbol,
			costErr,
		))
	}

	return quantity, nil
}

/*
Quantize floors quantity to the instrument qty_increment and qty_precision so
exchange lots never round up past the funded budget.
*/
func (price *Price) Quantize(
	pair *kraken.InstrumentPair,
	quantity *decimal.Decimal,
) *decimal.Decimal {
	if quantity == nil || quantity.Sign() <= 0 {
		return quantity
	}

	result := quantity.Copy()

	if pair.QtyIncrement != nil && pair.QtyIncrement.Sign() > 0 {
		steps := price.floorScale(price.Div(result, pair.QtyIncrement), 0)
		result = price.Mul(steps, pair.QtyIncrement)
	}

	if pair.QtyPrecision >= 0 {
		result = price.floorScale(result, int64(pair.QtyPrecision))
	}

	return result
}

/*
Mul multiplies at the sum of the operand scales, which exactly represents a
finite fixed-point product. Integer products retain one fractional place
because Kraken decimal's scale-zero banker rounding misclassifies exact odd
integers as half-way values.
*/
func (price *Price) Mul(left, right *decimal.Decimal) *decimal.Decimal {
	scale := max(int64(1), left.GetScale()+right.GetScale())

	return left.SetScale(scale).Mul(right)
}

/*
Div divides at the greater of the operands' precision and Decimal's documented
default precision. Callers that need an exchange-executable scale quantize the
result against the instrument after division.
*/
func (price *Price) Div(left, right *decimal.Decimal) *decimal.Decimal {
	scale := max(
		int64(decimal.DefaultScale),
		left.GetScale(),
		right.GetScale(),
	)

	return left.SetScale(scale).Div(right)
}

/*
DivFloor divides fixed-point values and floors the result at the caller's
executable scale. This is used for sell quantities where rounding upward would
exceed the money or liquidity boundary that funded the order.
*/
func (price *Price) DivFloor(
	left *decimal.Decimal,
	right *decimal.Decimal,
	scale int64,
) *decimal.Decimal {
	workingScale := max(scale, left.GetScale(), right.GetScale())
	dividend := left.SetRounding(
		func(integer *big.Int, factor *big.Int) *big.Int {
			return new(big.Int).Quo(integer, factor)
		},
	).SetScale(workingScale)
	quotient := dividend.Div(right)

	return price.floorScale(quotient, scale)
}
