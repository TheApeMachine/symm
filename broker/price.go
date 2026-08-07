package broker

import (
	"fmt"
	"sync"

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

/*
PnL returns the profit or loss for a holding, including fees.
It is based on the current market best bid price.
*/
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
		holding.EntryPrice.Mul(holding.Qty),
	).Sub(holding.EntryFee)
}

/*
ExitValue returns the exit value for a holding, which is the current market best bid price minus the taker fee.
*/
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
		tick.Bid.Mul(holding.Qty),
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
	book := price.api.Book(price.api.Normalizer().Name(pair.Symbol))

	if err := errnie.Error(errnie.Require(map[string]any{
		"holding": holding,
		"qty":     holding.Qty,
		"value":   value,
		"book":    book,
	})); err != nil {
		return nil
	}

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

		bookGross = bookGross.Add(
			bid.Price.Mul(fillQty),
		)

		remaining = remaining.Sub(fillQty)
	}

	if remaining.Cmp(zero) > 0 {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"insufficient bid liquidity to exit holding",
			nil,
		))

		return nil
	}

	bestBidNet := price.WithFee(
		pair.Symbol,
		tick.Bid.Mul(holding.Qty),
		SELL,
	)

	bookNet := price.WithFee(
		pair.Symbol,
		bookGross,
		SELL,
	)

	return value.Sub(bestBidNet.Sub(bookNet))
}

/*
ReturnPct returns the return percentage for a holding.
*/
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

	entryValue := holding.EntryPrice.Mul(
		holding.Qty,
	).Add(holding.EntryFee)

	zero := decimal.NewFromInt64(0)

	if entryValue.Cmp(zero) <= 0 {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"entry value must be greater than zero",
			nil,
		))

		return 0
	}

	return pnl.Div(entryValue).Mul(
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
Fee returns the taker fee for a symbol.
*/
func (price *Price) Fee(symbol string) *kraken.TradeVolumeFee {
	found, ok := price.fees.Load(price.api.Normalizer().Name(symbol))

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"fee not found",
			nil,
		))

		return nil
	}

	fee, ok := found.(kraken.TradeVolumeFee)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid fee type",
			nil,
		))

		return nil
	}

	return &fee
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
		price.fees.Store(price.api.Normalizer().Name(symbol), fee)
	}

	price.status = types.READY
	return nil
}

func (price *Price) WithFee(
	symbol string,
	amount *decimal.Decimal,
	direction Direction,
) *decimal.Decimal {
	fee := price.Fee(symbol)

	if err := errnie.Error(errnie.Require(map[string]any{
		"symbol":    symbol,
		"amount":    amount,
		"direction": direction,
		"fee":       fee,
	})); err != nil {
		return nil
	}

	feeAmount := decimal.ExactMul(amount, decimal.ExactDiv(
		fee.Fee, decimal.NewFromInt64(100),
	))

	switch direction {
	case BUY:
		amount = amount.Add(feeAmount)
	case SELL:
		amount = amount.Sub(feeAmount)
	}

	return amount
}
