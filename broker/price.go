package broker

import (
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
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

var (
	decimalZero    = decimal.NewFromInt64(0)
	decimalOne     = decimal.NewFromInt64(1)
	decimalTwo     = decimal.NewFromInt64(2)
	decimalHundred = decimal.NewFromInt64(100)
)

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
	if api == nil {
		panic("broker: api required")
	}

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
Mark returns the unit price with the taker fee applied.
*/
func (price *Price) Mark(
	symbol string,
	direction Direction,
) *decimal.Decimal {
	tick := price.Tick(symbol)

	if err := errnie.Error(errnie.Require(map[string]any{
		"symbol":    symbol,
		"direction": direction,
		"tick":      tick,
	})); err != nil {
		return nil
	}

	switch direction {
	case BUY:
		return price.WithFee(symbol, tick.Ask, BUY)
	case SELL:
		return price.WithFee(symbol, tick.Bid, SELL)
	}

	return nil
}

/* PnL returns holding profit or loss at the authoritative economic mark, including fees. */
func (price *Price) PnL(
	pair kraken.InstrumentPair,
	holding *types.Holding,
) *decimal.Decimal {
	if holding == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"price: holding required for PnL",
			nil,
		))

		return nil
	}

	fee := price.Fee(pair.Symbol)

	if fee == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"price: fee required for PnL",
			nil,
		))

		return nil
	}

	if err := errnie.Error(errnie.Require(map[string]any{
		"holding":    holding,
		"qty":        holding.Qty,
		"entryPrice": holding.EntryPrice,
		"entryFee":   holding.EntryFee,
		"mark":       holding.Mark,
		"fee":        fee.Fee,
	})); err != nil {
		return nil
	}

	exitValue := price.ExitValue(pair, holding)

	if exitValue == nil {
		return nil
	}

	return exitValue.Sub(
		decimalZero.Add(holding.EntryPrice).Mul(holding.Qty),
	).Sub(holding.EntryFee)
}

/* ExitValue returns holding value at the authoritative economic mark after the taker fee. */
func (price *Price) ExitValue(
	pair kraken.InstrumentPair,
	holding *types.Holding,
) *decimal.Decimal {
	if holding == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"price: holding required for exit value",
			nil,
		))

		return nil
	}

	if holding.Mark == nil || holding.Mark.Sign() <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"price: holding mark required for exit value",
			nil,
		))

		return nil
	}

	if err := errnie.Error(errnie.Require(map[string]any{
		"holding":    holding,
		"qty":        holding.Qty,
		"entryPrice": holding.EntryPrice,
		"entryFee":   holding.EntryFee,
		"mark":       holding.Mark,
	})); err != nil {
		return nil
	}

	return price.WithFee(
		pair.Symbol,
		decimalZero.Add(holding.Mark).Mul(holding.Qty),
		SELL,
	)
}

/*
WithFriction returns the value with the friction of exiting a holding applied.
This can be used for strategy calculations to account for the difference between
the best bid and the actual exit value when selling a holding.
*/
func (price *Price) WithFriction(
	pair kraken.InstrumentPair,
	holding *types.Holding,
	value *decimal.Decimal,
) (*decimal.Decimal, error) {
	if holding == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"price: holding required for exit friction",
			nil,
		))
	}

	tick := price.Tick(pair.Symbol)
	fee := price.Fee(pair.Symbol)

	if tick == nil || fee == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"price: ticker and fee required for exit friction",
			nil,
		))
	}

	if err := errnie.Error(errnie.Require(map[string]any{
		"holding": holding,
		"qty":     holding.Qty,
		"value":   value,
		"bid":     tick.Bid,
		"fee":     fee.Fee,
	})); err != nil {
		return nil, err
	}

	var adjusted *decimal.Decimal
	var err error
	bookFound := false
	price.api.Book(price.api.Normalizer().Name(pair.Symbol), func(managed *spotbook.Book) {
		bookFound = true
		if managed == nil || managed.Bids == nil || managed.Bids.High == nil {
			err = errnie.Err(
				errnie.NotFound,
				"price: visible bid book required for exit friction",
				nil,
			)
			return
		}

		zero := decimalZero
		remaining := decimalZero.Add(holding.Qty)
		bookGross := zero

		for bid := managed.Bids.High; bid != nil; bid = bid.Lower {
			if remaining.Cmp(zero) <= 0 {
				break
			}

			fillQty := bid.Quantity

			if fillQty.Cmp(remaining) > 0 {
				fillQty = remaining
			}

			bookGross = bookGross.Add(
				decimalZero.Add(bid.Price).Mul(fillQty),
			)

			remaining = remaining.Sub(fillQty)
		}

		if remaining.Cmp(zero) > 0 {
			err = errnie.Err(
				errnie.UnprocessableContent,
				"insufficient bid liquidity to exit holding",
				nil,
			)

			return
		}

		bestBidNet := price.WithFee(
			pair.Symbol,
			decimalZero.Add(tick.Bid).Mul(holding.Qty),
			SELL,
		)

		bookNet := price.WithFee(
			pair.Symbol,
			bookGross,
			SELL,
		)

		adjusted = decimalZero.Add(value).Sub(
			bestBidNet.Sub(bookNet),
		)
	})

	if !bookFound {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"price: visible bid book required for exit friction",
			nil,
		))
	}

	if err != nil {
		return nil, errnie.Error(err)
	}

	if adjusted == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"price: exit friction calculation did not complete",
			nil,
		))
	}

	return adjusted, nil
}

/*
Surface derives the exact executable-liquidation surface for the held
SellableQty from the resident BookManager bids under RLock. It returns a
surface with BookComplete and FullyExecutable set truthfully; it never
synthesizes a VWAP for insufficient depth and never falls back to ticker.
*/
func (price *Price) Surface(
	symbol string,
	sellableQty *decimal.Decimal,
	floor *decimal.Decimal,
	fee *kraken.TradeVolumeFee,
	at time.Time,
) *types.ExecutionSurface {
	surface := &types.ExecutionSurface{
		Symbol:      symbol,
		At:          at,
		SellableQty: sellableQty,
	}

	if price == nil || price.api == nil || sellableQty == nil || sellableQty.Sign() <= 0 ||
		fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 ||
		fee.Fee.Cmp(decimalHundred) >= 0 {
		return surface
	}

	price.api.Book(price.api.Normalizer().Name(symbol), func(managed *spotbook.Book) {
		if managed == nil || managed.Bids == nil || managed.Bids.High == nil {
			return
		}

		if managed.Asks != nil && managed.Asks.Low != nil &&
			managed.Bids.High.Price != nil && managed.Asks.Low.Price != nil &&
			managed.Bids.High.Price.Cmp(managed.Asks.Low.Price) >= 0 {
			return
		}

		surface.BookComplete = true
		if managed.Bids.High.Price != nil {
			surface.BestBid = managed.Bids.High.Price.Copy()
		}

		executableQty := decimalZero.Copy()
		floorCoverageQty := decimalZero.Copy()
		grossProceeds := decimalZero.Copy()
		remaining := sellableQty.Copy()

		for bid := managed.Bids.High; bid != nil; bid = bid.Lower {
			if bid.Price == nil || bid.Quantity == nil ||
				bid.Price.Sign() <= 0 || bid.Quantity.Sign() <= 0 {
				continue
			}

			executableQty = executableQty.Add(bid.Quantity)

			if floor != nil && bid.Price.Cmp(floor) >= 0 {
				floorCoverageQty = floorCoverageQty.Add(bid.Quantity)
			}

			if remaining.Sign() <= 0 {
				continue
			}

			fill := bid.Quantity
			if remaining.Cmp(fill) < 0 {
				fill = remaining
			}

			grossProceeds = grossProceeds.Add(decimalZero.Add(bid.Price).Mul(fill))
			remaining = remaining.Sub(fill)
		}

		surface.ExecutableQty = executableQty
		surface.FloorCoverageQty = floorCoverageQty

		if remaining.Sign() > 0 || executableQty.Cmp(sellableQty) < 0 {
			return
		}

		feeRate := decimalZero.Add(fee.Fee).Div(decimalHundred)
		surface.FullyExecutable = true
		surface.ExecutableVWAP = grossProceeds.Div(sellableQty)
		surface.ExecutableValue = grossProceeds.Sub(grossProceeds.Mul(feeRate))
	})

	return surface
}

/* ReturnPct returns the holding's fee-inclusive percentage return at the authoritative economic mark. */
func (price *Price) ReturnPct(
	pair kraken.InstrumentPair,
	holding *types.Holding,
) float64 {
	if err := errnie.Error(errnie.Require(map[string]any{
		"holding":    holding,
		"qty":        holding.Qty,
		"entryPrice": holding.EntryPrice,
		"entryFee":   holding.EntryFee,
	})); err != nil {
		return 0
	}

	pnl := price.PnL(pair, holding)

	if pnl == nil {
		return 0
	}

	entryGross := decimalZero.Add(holding.EntryPrice).Mul(holding.Qty)
	entryValue := entryGross.Add(holding.EntryFee)

	entryValueFloat := entryValue.Float64()

	if entryValueFloat <= 0 {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"entry value must be greater than zero",
			nil,
		))

		return 0
	}

	return decimalZero.Add(pnl).Div(entryValue).Mul(decimalHundred).Float64()
}

/*
Quantity returns the quantity that can be purchased for a given notional amount
at the current market price with taker fees applied.
*/
func (price *Price) Quantity(
	symbol string,
	notional *decimal.Decimal,
) *decimal.Decimal {
	tick := price.Tick(symbol)

	if tick == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"price: ticker required for quantity",
			nil,
		))

		return nil
	}

	if err := errnie.Error(errnie.Require(map[string]any{
		"symbol":   symbol,
		"notional": notional,
		"tick":     tick,
		"tick.ask": tick.Ask,
	})); err != nil {
		return nil
	}

	askWithFee := price.WithFee(
		symbol,
		tick.Ask,
		BUY,
	)

	if askWithFee == nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"could not apply fee to ask price",
			nil,
		))

		return nil
	}

	out, err := price.normalizer.FormatSize(
		symbol,
		decimalZero.Add(notional).Div(askWithFee),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid quantity",
			err,
		))

		return nil
	}

	return out
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
