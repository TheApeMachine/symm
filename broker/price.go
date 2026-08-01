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
		out, err = price.normalizer.FormatPrice(
			symbol, tick.Bid.OffsetPercent(fee.Fee),
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
			symbol, volume.Mul(tick.Ask).OffsetPercent(fee.Fee),
		)
	case SELL:
		out, err = price.normalizer.FormatPrice(
			symbol, volume.Mul(tick.Bid).OffsetPercent(fee.Fee),
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
	_, fee, err := price.getTickAndFee(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"could not get tick and fee for symbol",
			err,
		))
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

func (price *Price) getTickAndFee(symbol string) (
	*kraken.TickerData, *kraken.TradeVolumeFee, error,
) {
	found, ok := price.fees.Load(price.api.Normalizer().Name(symbol))

	if !ok || found == nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"fee not found",
			nil,
		))
	}

	fee, ok := found.(kraken.TradeVolumeFee)

	if !ok {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid fee type",
			nil,
		))
	}

	tick := price.Tick(symbol)

	if tick == nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"ticker not found",
			nil,
		))
	}

	return tick, &fee, nil
}
