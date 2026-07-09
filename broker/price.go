package broker

import (
	"context"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

type Price struct {
	ctx     context.Context
	cancel  context.CancelFunc
	public  websocket.PublicSocket
	private websocket.Private
	symbols atomic.Value
	tickers atomic.Value
	fees    atomic.Value
}

func NewPrice(
	ctx context.Context,
	public websocket.PublicSocket,
	private websocket.Private,
) *Price {
	if public == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: public stream required",
			nil,
		))

		return nil
	}

	if private == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: private stream required",
			nil,
		))

		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	price := &Price{
		ctx:     ctx,
		cancel:  cancel,
		public:  public,
		private: private,
	}

	price.symbols.Store(map[string]struct{}{})
	price.tickers.Store(map[string]kraken.TickerData{})
	price.fees.Store(map[string]websocket.FeeRates{})

	go price.observeTickers(public.Observe(channelTicker))
	go price.observeInstruments(public.Observe("instrument"))

	return price
}

func (price *Price) Close() {
	if price == nil || price.cancel == nil {
		return
	}

	price.cancel()
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

func (price *Price) observeTickers(channel chan []byte) {
	for {
		select {
		case <-price.ctx.Done():
			return
		case msg, ok := <-channel:
			if !ok {
				return
			}

			rows := kraken.NewTickerDataSlice(msg)
			symbols, _ := price.symbols.Load().(map[string]struct{})
			if len(symbols) == 0 {
				continue
			}

			current, _ := price.tickers.Load().(map[string]kraken.TickerData)
			next := make(map[string]kraken.TickerData, len(symbols))

			for symbol, ticker := range current {
				if _, ok := symbols[symbol]; ok {
					next[symbol] = ticker
				}
			}

			if len(rows) == 0 {
				if len(next) != len(current) {
					price.tickers.Store(next)
				}

				continue
			}

			changed := false

			for _, ticker := range rows {
				symbol := strings.TrimSpace(ticker.Symbol)
				if symbol == "" {
					continue
				}

				if _, ok := symbols[symbol]; !ok {
					continue
				}

				ticker.Symbol = symbol
				next[symbol] = ticker
				changed = true
			}

			if changed || len(next) != len(current) {
				price.tickers.Store(next)
			}
		}
	}
}

func (price *Price) observeInstruments(channel chan []byte) {
	for {
		select {
		case <-price.ctx.Done():
			return
		case msg, ok := <-channel:
			if !ok {
				return
			}

			data := kraken.NewInstrumentData(msg)
			symbols := price.instrumentSymbols(data)
			nextSymbols := make(map[string]struct{}, len(symbols))

			for _, symbol := range symbols {
				nextSymbols[symbol] = struct{}{}
			}

			price.symbols.Store(nextSymbols)
			current, _ := price.tickers.Load().(map[string]kraken.TickerData)
			next := make(map[string]kraken.TickerData, len(nextSymbols))

			for symbol, ticker := range current {
				if _, ok := nextSymbols[symbol]; ok {
					next[symbol] = ticker
				}
			}

			if len(next) != len(current) {
				price.tickers.Store(next)
			}

			if len(symbols) == 0 {
				price.fees.Store(map[string]websocket.FeeRates{})
				price.tickers.Store(map[string]kraken.TickerData{})
				continue
			}

			schedule, err := price.private.TradeVolume(symbols)
			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"broker price: TradeVolume failed",
					err,
				))
				continue
			}

			price.observeFeeSchedule(schedule)
		}
	}
}

func (price *Price) observeFeeSchedule(schedule websocket.FeeSchedule) {
	symbols, _ := price.symbols.Load().(map[string]struct{})
	if len(symbols) == 0 {
		price.fees.Store(map[string]websocket.FeeRates{})
		return
	}

	next := make(map[string]websocket.FeeRates, len(symbols))

	for symbol, rates := range schedule.Pairs {
		symbol = strings.TrimSpace(symbol)
		if symbol == "" {
			continue
		}

		if _, ok := symbols[symbol]; !ok {
			continue
		}

		next[symbol] = rates
	}

	price.fees.Store(next)
}

func (price *Price) instrumentSymbols(data kraken.InstrumentData) []string {
	quote := strings.ToUpper(strings.TrimSpace(
		viper.GetViper().GetString("market.quote_currency"),
	))
	symbols := make([]string, 0, len(data.Pairs))

	for _, pair := range data.Pairs {
		symbol := strings.TrimSpace(pair.Symbol)
		if symbol == "" || pair.Status != "online" {
			continue
		}

		if quote != "" && strings.ToUpper(strings.TrimSpace(pair.Quote)) != quote {
			continue
		}

		symbols = append(symbols, symbol)
	}

	return symbols
}

func (price *Price) ticker(pair string) (kraken.TickerData, bool) {
	pair = strings.TrimSpace(pair)
	current, _ := price.tickers.Load().(map[string]kraken.TickerData)
	ticker, ok := current[pair]

	return ticker, ok
}

func (price *Price) fee(pair string) (float64, bool) {
	pair = strings.TrimSpace(pair)
	current, _ := price.fees.Load().(map[string]websocket.FeeRates)
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

	entry := position.data.EntryPrice
	exit := ticker.Bid
	qty := decimal.NewFromFloat64(position.data.Qty)

	if entry.Rat().Sign() <= 0 || exit.Rat().Sign() <= 0 ||
		qty == nil || qty.Rat().Sign() <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: position entry, exit bid, and quantity must be positive",
			nil,
		))
		return decimal.Decimal{}
	}

	fee := decimal.NewFromFloat64(feeRate)
	return *exit.Sub(&entry).Mul(qty).Sub(entry.Mul(qty).Mul(fee)).Sub(exit.Mul(qty).Mul(fee))
}
