package broker

import (
	"context"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

type Direction string

const (
	BUY  Direction = "buy"
	SELL Direction = "sell"
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
	capture    *audit.Recorder
}

/*
NewPrice wires the price surface to the shared Kraken API.
*/
func NewPrice(ctx context.Context, bus *runtime.Workspace) *Price {
	if bus == nil {
		panic("broker: workspace bus required")
	}

	var api *websocket.API
	if shared, _ := bus.Shared("api", ""); shared != nil {
		api, _ = shared.(*websocket.API)
	}
	if api == nil {
		panic("broker: api not found in workspace")
	}

	price := &Price{
		status:     types.READY,
		api:        api,
		fees:       &sync.Map{},
		tickers:    &sync.Map{},
		normalizer: api.Normalizer(),
	}

	if shared, _ := bus.Shared("recorder", ""); shared != nil {
		if capture, ok := shared.(*audit.Recorder); ok {
			price.capture = capture
		}
	}

	return price
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

/* PnL returns holding profit or loss at the current bid, including fees. */
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

	tick, fee := price.Tick(pair.Symbol), price.Fee(pair.Symbol)

	if tick == nil || fee == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"price: ticker and fee required for PnL",
			nil,
		))

		return nil
	}

	if err := errnie.Error(errnie.Require(map[string]any{
		"holding":    holding,
		"qty":        holding.Qty,
		"entryPrice": holding.EntryPrice,
		"entryFee":   holding.EntryFee,
		"bid":        tick.Bid,
		"fee":        fee.Fee,
	})); err != nil {
		return nil
	}

	exitValue := price.ExitValue(pair, holding)

	if exitValue == nil {
		return nil
	}

	return exitValue.Sub(
		decimal.NewFromInt64(0).Add(holding.EntryPrice).Mul(holding.Qty),
	).Sub(holding.EntryFee)
}

/* ExitValue returns holding value at the current bid after the taker fee. */
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

	tick := price.Tick(pair.Symbol)

	if tick == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"price: ticker required for exit value",
			nil,
		))

		return nil
	}

	if err := errnie.Error(errnie.Require(map[string]any{
		"holding":    holding,
		"qty":        holding.Qty,
		"entryPrice": holding.EntryPrice,
		"entryFee":   holding.EntryFee,
		"bid":        tick.Bid,
	})); err != nil {
		return nil
	}

	return price.WithFee(
		pair.Symbol,
		decimal.NewFromInt64(0).Add(tick.Bid).Mul(holding.Qty),
		SELL,
	)
}

/*
ExecutableSurface derives the full-lot executable-liquidation state for one
position's actual current SellableQty from the authoritative L3 book, under the
protected read callback. It is the single authoritative executable-liquidation
calculation: it walks the same high→low bid chain WithFriction walks, filling
the position's complete SellableQty and producing the fee-net executable VWAP
and value. It never falls back to ticker, never copies the book, and never lets
the managed book escape the callback.

Three independent facts are derived:

  ExecutableQty    — total quantity fillable from the whole visible valid bid
                     depth (how much the book can absorb before running out).
  FloorCoverageQty — quantity visible at or above the protected floor (how much
                     of the sellable lot the floor can still realize).
  ExecutableVWAP   — the full-lot liquidation-equivalent price, defined only
                     when ExecutableQty >= SellableQty.

It reports BookComplete=false (and FullyExecutable=false) when the book cannot
truthfully price the position — missing bid side, or crossed with the ask side.
It reports FullyExecutable=false when the visible valid depth cannot fill the
complete SellableQty, with ExecutableVWAP/ExecutableValue left undefined rather
than fabricated.
*/
func (price *Price) ExecutableSurface(
	symbol string,
	sellableQty *decimal.Decimal,
	floor *decimal.Decimal,
	at time.Time,
) *types.ExecutionSurface {
	surface := &types.ExecutionSurface{
		Symbol:      symbol,
		At:          at,
		SellableQty: decimal.NewFromInt64(0).Add(sellableQty),
	}

	if price == nil || price.api == nil || sellableQty == nil || sellableQty.Sign() <= 0 {
		return surface
	}

	price.api.Book(price.api.Normalizer().Name(symbol), func(managed *book.Book) {
		if managed == nil || managed.Bids == nil || managed.Bids.High == nil ||
			managed.Asks == nil || managed.Asks.Low == nil {
			return
		}

		bestBid := decimal.NewFromInt64(0).Add(managed.Bids.High.Price)

		if managed.Asks.Low.Price.Cmp(bestBid) < 0 {
			return
		}

		surface.BookComplete = true
		surface.BestBid = bestBid

		zero := decimal.NewFromInt64(0)
		executable := zero
		covered := zero

		for bid := managed.Bids.High; bid != nil; bid = bid.Lower {
			if bid.Quantity.Sign() <= 0 {
				continue
			}

			executable = executable.Add(bid.Quantity)

			if floor == nil || bid.Price.Cmp(floor) >= 0 {
				covered = covered.Add(bid.Quantity)
			}
		}

		surface.ExecutableQty = decimal.NewFromInt64(0).Add(executable)
		surface.FloorCoverageQty = decimal.NewFromInt64(0).Add(covered)

		if executable.Cmp(sellableQty) < 0 {
			return
		}

		surface.FullyExecutable = true

		remaining := decimal.NewFromInt64(0).Add(sellableQty)
		gross := zero

		for bid := managed.Bids.High; bid != nil && remaining.Sign() > 0; bid = bid.Lower {
			if bid.Quantity.Sign() <= 0 {
				continue
			}

			fillQty := bid.Quantity

			if fillQty.Cmp(remaining) > 0 {
				fillQty = remaining
			}

			gross = gross.Add(decimal.NewFromInt64(0).Add(bid.Price).Mul(fillQty))
			remaining = remaining.Sub(fillQty)
		}

		surface.ExecutableValue = price.WithFee(symbol, gross, SELL)
		surface.ExecutableVWAP = decimal.NewFromInt64(0).Add(
			surface.ExecutableValue,
		).Div(sellableQty)
	})

	return surface
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
	price.api.Book(price.api.Normalizer().Name(pair.Symbol), func(managed *book.Book) {
		if managed == nil || managed.Bids == nil || managed.Bids.High == nil {
			err = errnie.Err(
				errnie.NotFound,
				"price: visible bid book required for exit friction",
				nil,
			)
			return
		}

		zero := decimal.NewFromInt64(0)
		remaining := decimal.NewFromInt64(0).Add(holding.Qty)
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
				decimal.NewFromInt64(0).Add(bid.Price).Mul(fillQty),
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
			decimal.NewFromInt64(0).Add(tick.Bid).Mul(holding.Qty),
			SELL,
		)

		bookNet := price.WithFee(
			pair.Symbol,
			bookGross,
			SELL,
		)

		adjusted = decimal.NewFromInt64(0).Add(value).Sub(
			bestBidNet.Sub(bookNet),
		)
	})

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

/* ReturnPct returns the holding's fee-inclusive percentage return. */
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

	entryGross := decimal.NewFromInt64(0).Add(holding.EntryPrice).Mul(
		holding.Qty,
	)
	entryValue := entryGross.Add(holding.EntryFee)

	zero := decimal.NewFromInt64(0)

	if entryValue.Cmp(zero) <= 0 {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"entry value must be greater than zero",
			nil,
		))

		return 0
	}

	return decimal.NewFromInt64(0).Add(pnl).Div(entryValue).Mul(
		decimal.NewFromInt64(100),
	).Float64()
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
		decimal.NewFromInt64(0).Add(notional).Div(askWithFee),
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
