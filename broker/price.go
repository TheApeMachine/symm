package broker

import (
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

const percentageFractionPlaces int64 = 2

/*
PriceAPI is the Kraken surface Price needs for ticker callbacks and fee tiers.
*/
type PriceAPI interface {
	On(channel string, action func([]byte))
	TradeVolume(symbols []string) (*kraken.TradeVolume, error)
}

/*
Price is the broker price surface for symm.
It owns live ticker snapshots, TradeVolume fee tiers, and every
quote/fee calculation callers need before placing an order.
*/
type Price struct {
	ready   atomic.Bool
	api     PriceAPI
	ui      chan []byte
	fees    *sync.Map
	tickers *sync.Map
}

/*
NewPrice wires the price stream to the shared Kraken API.
*/
func NewPrice(api PriceAPI, ui chan []byte) *Price {
	price := &Price{
		api:     api,
		ui:      ui,
		fees:    &sync.Map{},
		tickers: &sync.Map{},
	}

	price.api.On("ticker", price.TickerAck)
	return price
}

func (price *Price) Status() types.Status {
	if price.ready.Load() {
		return types.READY
	}

	return types.INITIALIZING
}

func (price *Price) Publish() {
	price.tickers.Range(func(key, value any) bool {
		price.ui <- datura.Map[any]{
			"tickers": []kraken.TickerData{*value.(*kraken.TickerData)},
		}.Marshal()
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
		// Create a copy to take the address of
		val := item
		price.tickers.Store(item.Symbol, &val)
	}
}

/*
Snapshot returns ticker rows for the requested identity set and names every
symbol that has not produced a ticker row yet.
*/
func (price *Price) Snapshot(symbols []string) ([]kraken.TickerData, []string) {
	rows := make([]kraken.TickerData, 0, len(symbols))
	missing := make([]string, 0)

	for _, symbol := range symbols {
		value, ok := price.tickers.Load(symbol)

		if !ok {
			missing = append(missing, symbol)
			continue
		}

		rows = append(rows, *value.(*kraken.TickerData))
	}

	return rows, missing
}

/*
Taker returns the all-in taker quote for qty at the current last price.
*/
func (price *Price) Taker(
	symbol string, quantity decimal.Decimal,
) (*decimal.Decimal, error) {
	ticker, err := price.Get(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get ticker: "+err.Error(),
			err,
		))
	}

	productScale := ticker.Last.GetScale() + quantity.GetScale()
	// Rat preserves the declared scales. api-go Decimal.Mul misclassifies an
	// integer-scale remainder as a half tie and rounds odd products upward.
	product := new(big.Rat).Mul(ticker.Last.Rat(), quantity.Rat())
	amount, err := decimal.NewFromString(product.FloatString(int(productScale)))

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate taker amount",
			err,
		))
	}

	return price.WithFee(symbol, *amount)
}

/*
WithFriction returns the all-in round-trip taker quote for qty at the
current last price. Both buy and sell legs use the live TradeVolume fee.
*/
func (price *Price) WithFriction(
	symbol string, quantity decimal.Decimal,
) (*decimal.Decimal, error) {
	taker, err := price.Taker(symbol, quantity)

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

	// TradeVolume reports percentage points, so two decimal places are added
	// before division by 100 to retain the full fractional fee rate.
	percentageScale := fee.GetScale() + percentageFractionPlaces
	rate := fee.SetScale(percentageScale).Div(decimal.NewFromInt64(100))
	productScale := amount.GetScale() + rate.GetScale()

	return amount.SetScale(productScale).OffsetPercent(rate), nil
}

/*
GetFees loads TradeVolume fee tiers for symbols and marks the price ready.
*/
func (price *Price) GetFees(symbols []string) error {
	requested := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"fee symbol required",
				nil,
			))
		}

		if _, duplicate := requested[symbol]; duplicate {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"duplicate fee symbol "+symbol,
				nil,
			))
		}

		requested[symbol] = struct{}{}
	}

	if len(requested) == 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"at least one fee symbol required",
			nil,
		))
	}

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

	fees := make(map[string]kraken.TradeVolumeFees, len(symbols))

	for _, symbol := range symbols {
		feeRate, ok := tradeVolume.Result.Fees[symbol]

		if !ok {
			return errnie.Error(errnie.Err(
				errnie.NotFound,
				"trade volume fee missing for symbol "+symbol,
				nil,
			))
		}

		if feeRate.Fee == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"trade volume fee required for symbol "+symbol,
				nil,
			))
		}

		fee, err := decimal.NewFromString(feeRate.Fee)

		if err != nil || fee.Sign() < 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"invalid trade volume fee for symbol "+symbol,
				err,
			))
		}

		fees[symbol] = feeRate
	}

	price.ready.Store(false)
	price.fees.Clear()

	for symbol, feeRate := range fees {
		price.fees.Store(symbol, feeRate)
	}

	price.ready.Store(true)
	return nil
}

/*
Get returns the latest cached ticker row for symbol.
*/
func (price *Price) Get(symbol string) (*kraken.TickerData, error) {
	if price.Status() != types.READY {
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
	if !price.ready.Load() {
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
