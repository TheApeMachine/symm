package broker

import (
	"sync"

	"github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/api-go/v2/pkg/spot"
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
	tick, fee := price.Tick(pair.Symbol), price.Fee(pair.Symbol)

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

	return price.ExitValue(pair, holding).Sub(
		decimal.ExactMul(holding.EntryPrice, holding.Qty),
	).Sub(holding.EntryFee)
}

/* ExitValue returns holding value at the current bid after the taker fee. */
func (price *Price) ExitValue(
	pair kraken.InstrumentPair,
	holding *types.Holding,
) *decimal.Decimal {
	tick := price.Tick(pair.Symbol)

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
		decimal.ExactMul(tick.Bid, holding.Qty),
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
) *decimal.Decimal {
	tick := price.Tick(pair.Symbol)

	if err := errnie.Error(errnie.Require(map[string]any{
		"holding": holding,
		"qty":     holding.Qty,
		"value":   value,
	})); err != nil {
		return nil
	}

	var adjusted *decimal.Decimal
	price.api.Book(price.api.Normalizer().Name(pair.Symbol), func(book *book.Book) {
		zero := decimal.NewFromInt64(0)
		remaining := holding.Qty
		bookGross := zero

		for _, bid := range book.Bids.Levels {
			if remaining.Cmp(zero) <= 0 {
				break
			}

			fillQty := bid.Quantity

			if fillQty.Cmp(remaining) > 0 {
				fillQty = remaining
			}

			bookGross = bookGross.Add(decimal.ExactMul(bid.Price, fillQty))

			remaining = remaining.Sub(fillQty)
		}

		if remaining.Cmp(zero) > 0 {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"insufficient bid liquidity to exit holding",
				nil,
			))

			return
		}

		bestBidNet := price.WithFee(
			pair.Symbol,
			decimal.ExactMul(tick.Bid, holding.Qty),
			SELL,
		)

		bookNet := price.WithFee(
			pair.Symbol,
			bookGross,
			SELL,
		)

		adjusted = value.Sub(bestBidNet.Sub(bookNet))
	})

	return adjusted
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

	entryGross := decimal.ExactMul(
		holding.EntryPrice,
		holding.Qty,
	)
	entryScale := max(entryGross.GetScale(), holding.EntryFee.GetScale())
	entryValue := entryGross.SetScale(entryScale).Add(holding.EntryFee)

	zero := decimal.NewFromInt64(0)

	if entryValue.Cmp(zero) <= 0 {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"entry value must be greater than zero",
			nil,
		))

		return 0
	}

	return decimal.ExactMul(
		decimal.ExactDiv(pnl, entryValue),
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
		decimal.ExactDiv(notional, askWithFee),
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
