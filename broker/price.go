package broker

import (
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
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
Price is the broker price surface for symm. It owns fee tiers, ticker cache,
the canonical per-symbol L3 book, and all money math so the rest of the broker
never drifts from Kraken's precision and executable boundaries.
*/
type Price struct {
	status     types.Status
	api        *websocket.API
	fees       *sync.Map
	tickers    *sync.Map
	books      *kraken.BookOwner
	normalizer *spot.Normalizer
	capture    *audit.Recorder
}

/*
NewPrice wires the price surface to the Kraken API. recorder is optional.
*/
func NewPrice(api *websocket.API, recorder *audit.Recorder) *Price {
	if api == nil {
		panic("broker: api required")
	}

	return &Price{
		status:     types.READY,
		api:        api,
		fees:       &sync.Map{},
		tickers:    &sync.Map{},
		books:      kraken.NewBookOwner(),
		normalizer: api.Normalizer(),
		capture:    recorder,
	}
}

/*
ApplyLevel3 folds one L3 frame into the canonical per-symbol book. The broker
owns the single authoritative L3 state; every other consumer of executable
liquidity reads it through ExecutableSurface or the protected Fold path rather
than reconstructing their own book from raw frames.
*/
func (price *Price) ApplyLevel3(level3 kraken.Level3Data) {
	if price == nil || price.books == nil {
		return
	}

	price.books.Apply(level3)
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
		decimal.NewFromInt64(0).Add(holding.EntryPrice).Mul(holding.Qty),
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
		decimal.NewFromInt64(0).Add(holding.Mark).Mul(holding.Qty),
		SELL,
	)
}

/*
ExecutableSurface derives the full-lot executable-liquidation state for one
position's actual current SellableQty from the authoritative L3 book, under the
protected read callback. It is the single authoritative executable-liquidation
calculation: it walks the book's high→low bid chain (already price ordered, so
no per-evaluation sort or full copy is made), filling the position's complete
SellableQty to produce the gross executable VWAP alongside the fee-net
executable value. It never falls back to ticker and never lets the managed book
escape the callback.

Derived independently for the exact SellableQty:

	ExecutableQty    — total quantity fillable from the whole visible valid bid
	                   depth (how much the book can absorb before running out).
	FloorCoverageQty — quantity visible at or above the protected floor (how much
	                   of the sellable lot the floor can still realize).
	ExecutableVWAP   — the full-lot liquidation-equivalent GROSS price (raw filled
	                   VWAP consumed best-bid-first until the entire SellableQty
	                   is filled). Defined ONLY when the entire lot is executable.
	ExecutableValue  — the actual gross proceeds of that full-lot fill minus the
	                   current taker exit fee, per the existing fee semantics. It
	                   is never divided back into a "fee-net price".

BookComplete=false (and ExecutableVWAP/ExecutableValue undefined) when the book
cannot truthfully price the position: missing bid side, crossed, or otherwise
invalid. FullyExecutable=false when visible valid depth cannot fill the complete
SellableQty, with no fabricated VWAP and no ticker fallback.
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

	if price == nil || price.books == nil || sellableQty == nil ||
		sellableQty.Sign() <= 0 {
		return surface
	}

	fee := price.FeeIfAvailable(symbol)

	if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 ||
		fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return surface
	}

	found := price.books.Fold(symbol, func(view kraken.BookView) {
		if !view.Valid {
			return
		}

		surface.BookComplete = true

		if len(view.Bids) > 0 && view.Bids[0].LimitPrice != nil {
			surface.BestBid = decimal.NewFromInt64(0).Add(view.Bids[0].LimitPrice)
		}

		// Walk visible bids best-first (already descending by price), summing
		// total executable quantity, floor coverage, and the full-lot fill.
		var executableQty, floorCoverageQty *decimal.Decimal
		var grossProceeds *decimal.Decimal
		remaining := decimal.NewFromInt64(0).Add(sellableQty)

		executableQty = decimal.NewFromInt64(0)
		floorCoverageQty = decimal.NewFromInt64(0)
		grossProceeds = decimal.NewFromInt64(0)

		for _, order := range view.Bids {
			if order.LimitPrice == nil || order.OrderQty == nil ||
				order.LimitPrice.Sign() <= 0 || order.OrderQty.Sign() <= 0 {
				continue
			}

			executableQty = executableQty.Add(order.OrderQty)

			if floor != nil && order.LimitPrice.Cmp(floor) >= 0 {
				floorCoverageQty = floorCoverageQty.Add(order.OrderQty)
			}

			if remaining.Sign() <= 0 {
				continue
			}

			fill := order.OrderQty

			if remaining.Cmp(fill) < 0 {
				fill = remaining
			}

			grossProceeds = grossProceeds.Add(order.LimitPrice.Mul(fill))
			remaining = remaining.Sub(fill)
		}

		surface.ExecutableQty = executableQty
		surface.FloorCoverageQty = floorCoverageQty

		if remaining.Sign() > 0 || executableQty.Cmp(sellableQty) < 0 {
			return
		}

		feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(
			decimal.NewFromInt64(100),
		)
		surface.FullyExecutable = true
		surface.ExecutableVWAP = grossProceeds.Div(sellableQty)
		surface.ExecutableValue = grossProceeds.Sub(grossProceeds.Mul(feeRate))
	})

	if !found {
		return surface
	}

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

	// This signal has no access to a full-depth book to walk for exit
	// friction, so it always reports unavailable rather than fabricating an
	// adjustment from ticker-level data alone.
	return nil, errnie.Error(errnie.Err(
		errnie.NotFound,
		"price: visible bid book required for exit friction",
		nil,
	))
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
