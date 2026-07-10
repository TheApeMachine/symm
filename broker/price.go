package broker

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Price struct {
	private websocket.Conn
	symbols atomic.Value
	tickers atomic.Value
	fees    atomic.Value
}

func NewPrice(private, public websocket.Conn) *Price {
	if private == nil || public == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: private and public streams required",
			nil,
		))

		return nil
	}

	price := &Price{private: private}
	price.symbols.Store(map[string]struct{}{})
	price.tickers.Store(map[string]kraken.TickerData{})
	price.fees.Store(map[string]kraken.FeeRates{})

	public.On("instrument", NewFees(price).On)
	public.On("ticker", NewQuote(price).On)

	return price
}

func (price *Price) Status() types.Status {
	if price == nil {
		return types.INITIALIZING
	}

	symbols, _ := price.symbols.Load().(map[string]struct{})

	if len(symbols) == 0 {
		return types.INITIALIZING
	}

	fees, _ := price.fees.Load().(map[string]kraken.FeeRates)

	if len(fees) == 0 {
		return types.PENDING
	}

	tickers, _ := price.tickers.Load().(map[string]kraken.TickerData)

	if len(tickers) == 0 {
		return types.PENDING
	}

	return types.READY
}

/*
Symbol gives back the most recent, raw ticker price for a given symbol pair.
*/
func (price *Price) Symbol(pair string) decimal.Decimal {
	ticker, ok := price.ticker(pair)
	if !ok {
		return decimal.Decimal{}
	}

	return ticker.Last
}

/*
Entry returns the executable ask price for opening a long position.
*/
func (price *Price) Entry(pair string) (decimal.Decimal, bool) {
	ticker, ok := price.ticker(pair)
	if !ok {
		return decimal.Decimal{}, false
	}

	if ticker.Ask.Rat().Sign() <= 0 {
		return decimal.Decimal{}, false
	}

	return ticker.Ask, true
}

/*
RoundTripFriction returns the current executable round-trip friction for symbol:
crossing the spread once plus entry and exit taker fees.
*/
func (price *Price) RoundTripFriction(pair string) (*big.Rat, bool) {
	ticker, ok := price.ticker(pair)
	if !ok {
		return nil, false
	}

	feeRate, ok := price.fee(pair)
	if !ok {
		return nil, false
	}

	if math.IsNaN(feeRate) || math.IsInf(feeRate, 0) || feeRate < 0 {
		return nil, false
	}

	bidRat := ticker.Bid.Rat()
	askRat := ticker.Ask.Rat()

	if bidRat.Sign() <= 0 || askRat.Sign() <= 0 || askRat.Cmp(bidRat) < 0 {
		return nil, false
	}

	midRat := new(big.Rat).Quo(
		new(big.Rat).Add(askRat, bidRat),
		big.NewRat(2, 1),
	)
	spreadRat := new(big.Rat).Quo(
		new(big.Rat).Sub(askRat, bidRat),
		midRat,
	)
	feeRat, ok := new(big.Rat).SetString(strconv.FormatFloat(feeRate, 'f', -1, 64))
	if !ok {
		return nil, false
	}

	frictionRat := new(big.Rat).Add(
		spreadRat,
		new(big.Rat).Mul(big.NewRat(2, 1), feeRat),
	)

	if frictionRat.Sign() < 0 {
		return nil, false
	}

	return frictionRat, true
}

/*
PnL gives back the profit or loss for a Position, with real fees from
TradeVolume for both entry and exit. Exit slippage is represented by liquidation
at the executable bid, not by a synthetic spread guess.
*/
func (price *Price) PnL(position *Position) decimal.Decimal {
	if price == nil || position == nil || position.data == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: position required",
			nil,
		))
		return decimal.Decimal{}
	}

	symbol := strings.TrimSpace(position.data.Symbol)
	ticker, ok := price.ticker(symbol)
	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"broker price: ticker missing for "+symbol,
			nil,
		))
		return decimal.Decimal{}
	}

	feeRate, ok := price.fee(symbol)
	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"broker price: TradeVolume fee missing for "+symbol,
			nil,
		))
		return decimal.Decimal{}
	}

	return price.pnl(position, ticker, feeRate)
}

func (price *Price) ticker(pair string) (kraken.TickerData, bool) {
	pair = strings.TrimSpace(pair)
	current, _ := price.tickers.Load().(map[string]kraken.TickerData)
	ticker, ok := current[pair]

	return ticker, ok
}

func (price *Price) fee(pair string) (float64, bool) {
	pair = strings.TrimSpace(pair)
	current, _ := price.fees.Load().(map[string]kraken.FeeRates)
	rates, ok := current[pair]

	return rates.Taker, ok
}

func (price *Price) pnl(
	position *Position,
	ticker kraken.TickerData,
	feeRate float64,
) decimal.Decimal {
	if math.IsNaN(feeRate) || math.IsInf(feeRate, 0) || feeRate < 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: fee rate must be finite and non-negative",
			nil,
		))
		return decimal.Decimal{}
	}

	entryRat := position.data.EntryPrice.Rat()
	exitRat := ticker.Bid.Rat()
	qtyRat := new(big.Rat).SetFloat64(position.data.Qty)
	feeRat := new(big.Rat).SetFloat64(feeRate)

	if entryRat.Sign() <= 0 || exitRat.Sign() <= 0 || qtyRat.Sign() <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: position entry, exit bid, and quantity must be positive",
			nil,
		))
		return decimal.Decimal{}
	}

	grossRat := new(big.Rat).Mul(
		new(big.Rat).Sub(exitRat, entryRat),
		qtyRat,
	)
	entryFeeRat := new(big.Rat).Mul(
		new(big.Rat).Mul(entryRat, qtyRat),
		feeRat,
	)
	exitFeeRat := new(big.Rat).Mul(
		new(big.Rat).Mul(exitRat, qtyRat),
		feeRat,
	)
	netRat := new(big.Rat).Sub(
		new(big.Rat).Sub(grossRat, entryFeeRat),
		exitFeeRat,
	)

	calculationScale := int(max(
		position.data.EntryPrice.GetScale(),
		ticker.Bid.GetScale(),
		decimal.NewFromFloat64(feeRate).GetScale(),
	))
	net, err := decimal.NewFromString(netRat.FloatString(calculationScale))

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: invalid pnl calculation",
			err,
		))
		return decimal.Decimal{}
	}

	return *net
}
