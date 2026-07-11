package broker

import (
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Price is the broker price surface for symm.
It owns live ticker snapshots, TradeVolume fee tiers, and every
quote/fee calculation callers need before placing an order.
*/
type Price struct {
	status  types.Status
	api     *websocket.API
	ui      chan []byte
	fees    *sync.Map
	tickers *sync.Map
}

/*
NewPrice wires the price stream to the shared Kraken API and UI channel.
*/
func NewPrice(api *websocket.API, ui chan []byte) *Price {
	price := &Price{
		status:  types.INITIALIZING,
		api:     api,
		ui:      ui,
		fees:    &sync.Map{},
		tickers: &sync.Map{},
	}

	price.api.On("ticker", price.TickerAck)
	return price
}

func (price *Price) Status() types.Status {
	return price.status
}

/*
Publish pushes the current ticker cache to the UI channel.
*/
func (price *Price) Publish() {
	tickers := make([]kraken.TickerData, 0)

	price.tickers.Range(func(_, value any) bool {
		tickers = append(tickers, value.(kraken.TickerData))
		return true
	})
}

/*
TickerAck decodes a ticker envelope and refreshes the per-symbol cache.
*/
func (price *Price) TickerAck(buf []byte) {
	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return
	}

	for _, item := range ticker.Data {
		price.tickers.Store(item.Symbol, item)
	}

	price.Publish()
}

/*
Taker returns the all-in taker quote for qty at the current last price.
*/
func (price *Price) Taker(symbol string, qty float64) (*decimal.Decimal, error) {
	ticker, err := price.Get(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get ticker: "+err.Error(),
			err,
		))
	}

	return price.WithFee(
		symbol, *ticker.Last.Mul(decimal.NewFromFloat64(qty)),
	)
}

/*
WithFriction returns the all-in round-trip taker quote for qty at the
current last price. Both buy and sell legs use the live TradeVolume fee.
*/
func (price *Price) WithFriction(symbol string, qty float64) (*decimal.Decimal, error) {
	taker, err := price.Taker(symbol, qty)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get friction: "+err.Error(),
			err,
		))
	}

	return price.WithFee(symbol, *taker)
}

/*
WithFee applies the symbol's current taker fee rate to amount.
*/
func (price *Price) WithFee(
	symbol string, amount decimal.Decimal,
) (*decimal.Decimal, error) {
	feeRate, err := price.FeeRate(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get fee rate: "+err.Error(),
			err,
		))
	}

	fee, err := decimal.NewFromString(feeRate.Fee)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to parse fee: "+err.Error(),
			err,
		))
	}

	return amount.OffsetPercent(fee), nil
}

/*
GetFees loads TradeVolume fee tiers for symbols and marks the price ready.
*/
func (price *Price) GetFees(symbols []string) error {
	tradeVolume, err := price.api.TradeVolume(symbols)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trade volume",
			err,
		))
	}

	if errnie.Error(kraken.Validate(tradeVolume)) != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"invalid trade volume",
			nil,
		))
	}

	for symbol, feeRate := range tradeVolume.Result.Fees {
		price.fees.Store(symbol, feeRate)
	}

	price.status = types.READY
	return nil
}

/*
Get returns the latest cached ticker row for symbol.
*/
func (price *Price) Get(symbol string) (*kraken.TickerData, error) {
	if price.status != types.READY {
		return nil, errnie.Error(errnie.Err(
			errnie.NotImplemented,
			"price not ready",
			nil,
		))
	}

	ticker, ok := price.tickers.Load(symbol)

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"ticker not found for symbol "+symbol,
			nil,
		))
	}

	return ticker.(*kraken.TickerData), nil
}

/*
FeeRate returns the cached TradeVolume taker fee tier for symbol.
*/
func (price *Price) FeeRate(symbol string) (kraken.TradeVolumeFees, error) {
	if price.status != types.READY {
		return kraken.TradeVolumeFees{}, errnie.Error(errnie.Err(
			errnie.NotImplemented,
			"price not ready",
			nil,
		))
	}

	rate, ok := price.fees.Load(symbol)

	if !ok {
		return kraken.TradeVolumeFees{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			"fee rate not found for symbol "+symbol,
			nil,
		))
	}

	return rate.(kraken.TradeVolumeFees), nil
}
