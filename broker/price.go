package broker

import (
	"strings"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

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
	status  types.Status
	api     *websocket.API
	fees    *sync.Map
	scales  *sync.Map
	tickers *sync.Map
}

/*
NewPrice wires the price stream to the shared Kraken API.
*/
func NewPrice(
	api *websocket.API,
) *Price {
	return &Price{
		status:  types.INITIALIZING,
		api:     api,
		fees:    &sync.Map{},
		tickers: &sync.Map{},
	}
}

func (price *Price) Initialize() error {
	price.api.On("ticker", price.TickerAck)
	price.status = types.READY

	return nil
}

func (price *Price) Status() types.Status {
	return price.status
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
		price.tickers.Store(
			item.Symbol,
			&item,
		)
	}
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
		value, ok := price.tickers.Load(symbol)

		if !ok {
			missing = append(
				missing,
				symbol,
			)

			continue
		}

		rows = append(
			rows,
			*value.(*kraken.TickerData),
		)
	}

	return rows, missing
}

/*
Get returns the latest cached ticker row for symbol.
*/
func (price *Price) Get(
	symbol string,
) (*kraken.TickerData, error) {
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
		price.fees.Store(symbol, fee)
	}

	price.status = types.READY
	return nil
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

	rate, ok := price.fees.Load(symbol)

	if !ok {
		return kraken.TradeVolumeFee{}, errnie.Error(
			errnie.Err(
				errnie.NotFound,
				"fee rate not found for symbol "+symbol,
				nil,
			),
		)
	}

	return rate.(kraken.TradeVolumeFee), nil
}

/*
Notional converts price and quantity into quote-currency value.

For example:

	Price:    50,000 USD per BTC
	Quantity: 0.1 BTC
	Notional: 5,000 USD

The instrument supplies the only valid price and cost precision. The quantity
is scaled to the instrument's cost precision before multiplication, and the
result remains at that authoritative cost precision.
*/
func (price *Price) Notional(
	instrument *kraken.InstrumentPair,
	rate *decimal.Decimal,
	quantity *decimal.Decimal,
) *decimal.Decimal {
	return quantity.SetScale(int64(
		instrument.CostPrecision,
	)).Mul(rate.SetScale(int64(
		instrument.PricePrecision,
	))).SetScale(int64(instrument.CostPrecision))
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
	fraction := tradeVolume.Fee.SetScale(feePrecision).Div(
		decimal.NewFromInt64(100).SetScale(feePrecision),
	)

	return fraction.Mul(amount).SetScale(int64(instrument.CostPrecision))
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

	return notional.Add(price.Fee(
		instrument,
		notional,
	)), nil
}

/*
WithFriction returns the current notional plus two taker fees.

This helper assumes both fees are based on the same current notional.
It is useful as a rough current-price estimate, but it is not PnL.
*/
func (price *Price) WithFriction(
	instrument *kraken.InstrumentPair,
	quantity *decimal.Decimal,
) (*decimal.Decimal, error) {
	ticker, err := price.Get(instrument.Symbol)

	if err != nil {
		return nil, err
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

	taker, err := price.Taker(
		instrument,
		quantity,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate friction for "+instrument.Symbol,
			err,
		))
	}

	return taker.Add(price.Fee(
		instrument, notional,
	)), nil
}
