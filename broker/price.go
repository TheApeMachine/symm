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
PositionQuote is the complete current valuation of a position.

All monetary fields are in the quote currency.

For BTC/USD:

	EntryNotional, ExitNotional, EntryFee, ExitFee and PnL are USD.
*/
type PositionQuote struct {
	Mark          decimal.Decimal
	EntryNotional decimal.Decimal
	ExitNotional  decimal.Decimal
	EntryFee      decimal.Decimal
	ExitFee       decimal.Decimal
	PnL           decimal.Decimal
	ReturnPct     float64
}

/*
Price is the broker price surface for symm.

It owns:

  - live ticker snapshots
  - TradeVolume fee tiers
  - notional calculations
  - fee calculations
  - current position valuations
*/
type Price struct {
	status  types.Status
	api     *websocket.API
	fees    *sync.Map
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
		/*
			Make a new copy for each iteration before storing
			its address.
		*/
		value := item

		price.tickers.Store(
			item.Symbol,
			&value,
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
	tradeVolume, err := price.api.TradeVolume(symbols)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trade volume",
			err,
		))
	}

	for _, symbol := range symbols {
		price.fees.Store(symbol, tradeVolume.Result.Fees[symbol])
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
) (kraken.TradeVolumeFees, error) {
	if price.Status() != types.READY {
		return kraken.TradeVolumeFees{}, errnie.Error(
			errnie.Err(
				errnie.NotImplemented,
				"price not ready",
				nil,
			),
		)
	}

	rate, ok := price.fees.Load(symbol)

	if !ok {
		return kraken.TradeVolumeFees{}, errnie.Error(
			errnie.Err(
				errnie.NotFound,
				"fee rate not found for symbol "+symbol,
				nil,
			),
		)
	}

	return rate.(kraken.TradeVolumeFees), nil
}

/*
FeeFraction returns Kraken's fee percentage as a decimal fraction.

Examples:

	Kraken "0.1000" percent becomes 0.001.
	Kraken "0.2600" percent becomes 0.0026.

This is the value that can safely be multiplied by a notional.
*/
func (price *Price) FeeFraction(
	symbol string,
) (*decimal.Decimal, error) {
	feeTier, err := price.FeeRate(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get fee tier for "+symbol,
			err,
		))
	}

	feePercent, err := decimal.NewFromString(
		feeTier.Fee,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to parse fee rate "+feeTier.Fee,
			err,
		))
	}

	/*
		Kraken gives us a percentage.

		0.1000 / 100 = 0.001
	*/
	return feePercent.Div(
		decimal.NewFromInt64(100),
	), nil
}

/*
Notional converts price and quantity into quote-currency value.

For example:

	Price:    50,000 USD per BTC
	Quantity: 0.1 BTC
	Notional: 5,000 USD

Decimal.Mul rescales its argument down to the receiver's own scale
before multiplying, which silently rounds a fine-grained quantity (e.g.
0.0001 BTC, scale 4) to zero whenever the coarser-scaled price (e.g.
64129.9, scale 1) is the receiver. Widening the receiver's scale to
whichever operand needs more decimal places first keeps that rescale
lossless in both directions.
*/
func (price *Price) Notional(
	unitPrice decimal.Decimal,
	quantity decimal.Decimal,
) *decimal.Decimal {
	return unitPrice.SetScale(decimal.DefaultScale).Mul(&quantity)
}

/*
Fee calculates the taker fee for a quote-currency amount.

For example:

	Notional: 5,000 USD
	Fee rate: 0.1 percent
	Fee:      5 USD
*/
func (price *Price) Fee(
	symbol string,
	amount decimal.Decimal,
) (*decimal.Decimal, error) {
	rate, err := price.FeeFraction(symbol)

	if err != nil {
		return nil, err
	}

	return amount.Mul(rate), nil
}

/*
WithFee adds one taker fee to an amount.

For example:

	Amount: 10,000
	Fee:        10
	Result: 10,010
*/
func (price *Price) WithFee(
	symbol string,
	amount decimal.Decimal,
) (*decimal.Decimal, error) {
	fee, err := price.Fee(symbol, amount)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate fee",
			err,
		))
	}

	return amount.Add(fee), nil
}

/*
Taker returns the estimated all-in taker purchase cost for a quantity
at the current last price.

The result is:

	current notional + one taker fee
*/
func (price *Price) Taker(
	symbol string,
	quantity decimal.Decimal,
) (*decimal.Decimal, error) {
	ticker, err := price.Get(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get ticker for "+symbol,
			err,
		))
	}

	if ticker.Last == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"ticker has no last price for "+symbol,
			nil,
		))
	}

	notional := price.Notional(
		*ticker.Last,
		quantity,
	)

	return price.WithFee(
		symbol,
		*notional,
	)
}

/*
WithFriction returns the current notional plus two taker fees.

This helper assumes both fees are based on the same current notional.
It is useful as a rough current-price estimate, but it is not PnL.

PositionQuote should be used for actual position valuation because it
calculates entry and exit fees from their respective notionals.
*/
func (price *Price) WithFriction(
	symbol string,
	quantity decimal.Decimal,
) (*decimal.Decimal, error) {
	ticker, err := price.Get(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get ticker for "+symbol,
			err,
		))
	}

	if ticker.Last == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"ticker has no last price for "+symbol,
			nil,
		))
	}

	notional := price.Notional(
		*ticker.Last,
		quantity,
	)

	fee, err := price.Fee(
		symbol,
		*notional,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate friction for "+symbol,
			err,
		))
	}

	totalFees := fee.Mul(
		decimal.NewFromInt64(2),
	)

	return notional.Add(totalFees), nil
}

/*
PositionQuote calculates a position using the latest cached price.

This is the main method Desk and Position should call.
*/
func (price *Price) PositionQuote(
	symbol string,
	entryPrice decimal.Decimal,
	quantity decimal.Decimal,
) (*PositionQuote, error) {
	ticker, err := price.Get(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get ticker for "+symbol,
			err,
		))
	}

	if ticker.Last == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"ticker has no last price for "+symbol,
			nil,
		))
	}

	return price.PositionQuoteAt(
		symbol,
		entryPrice,
		*ticker.Last,
		quantity,
	)
}

/*
PositionQuoteAt calculates the current estimated net PnL of a long
spot position at a supplied mark price.

The formula is:

	entry notional = entry price × quantity
	exit notional  = mark price × quantity

	entry fee = entry notional × fee fraction
	exit fee  = exit notional × fee fraction

	PnL = exit notional
	    - entry notional
	    - entry fee
	    - exit fee

	ReturnPct = PnL / entry notional × 100

This currently assumes:

  - a positive long position
  - the same taker fee rate for entry and exit
  - the entire quantity exits at the supplied mark
*/
func (price *Price) PositionQuoteAt(
	symbol string,
	entryPrice decimal.Decimal,
	mark decimal.Decimal,
	quantity decimal.Decimal,
) (*PositionQuote, error) {
	if entryPrice.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"entry price must be positive for "+symbol,
			nil,
		))
	}

	if mark.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"mark price must be positive for "+symbol,
			nil,
		))
	}

	if quantity.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"quantity must be positive for "+symbol,
			nil,
		))
	}

	entryNotional := price.Notional(
		entryPrice,
		quantity,
	)

	exitNotional := price.Notional(
		mark,
		quantity,
	)

	entryFee, err := price.Fee(
		symbol,
		*entryNotional,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate entry fee for "+symbol,
			err,
		))
	}

	exitFee, err := price.Fee(
		symbol,
		*exitNotional,
	)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate exit fee for "+symbol,
			err,
		))
	}

	grossPnL := exitNotional.Sub(
		entryNotional,
	)

	netPnL := grossPnL.
		Sub(entryFee).
		Sub(exitFee)

	returnPct := netPnL.
		Div(entryNotional).
		Mul(decimal.NewFromInt64(100))

	return &PositionQuote{
		Mark:          mark,
		EntryNotional: *entryNotional,
		ExitNotional:  *exitNotional,
		EntryFee:      *entryFee,
		ExitFee:       *exitFee,
		PnL:           *netPnL,
		ReturnPct:     returnPct.Float64(),
	}, nil
}
