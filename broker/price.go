package broker

import (
	"context"
	"math"
	"math/big"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Price struct {
	ctx         context.Context
	cancel      context.CancelFunc
	public      websocket.PublicSocket
	private     websocket.Private
	tickers     atomic.Value
	fees        atomic.Value
	predictions atomic.Value
}

func NewPrice(
	ctx context.Context,
	public websocket.PublicSocket,
	private websocket.Private,
) (*Price, error) {
	if public == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: public stream required",
			nil,
		))
	}

	if private == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: private stream required",
			nil,
		))
	}

	ctx, cancel := context.WithCancel(ctx)
	price := &Price{
		ctx:     ctx,
		cancel:  cancel,
		public:  public,
		private: private,
	}

	price.tickers.Store(map[string]kraken.TickerData{})
	price.fees.Store(map[string]websocket.FeeRates{})
	price.predictions.Store(map[string][]types.Prediction{})

	go price.observeTickers(public.Observe(channelTicker))
	go price.observeInstruments(public.Observe("instrument"))

	return price, nil
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

/*
Predicted gives back the latest prediction window stored for a symbol.
*/
func (price *Price) Predicted(pair string) []types.Prediction {
	pair = strings.TrimSpace(pair)
	current, _ := price.predictions.Load().(map[string][]types.Prediction)
	rows := append([]types.Prediction(nil), current[pair]...)

	sort.Slice(rows, func(left int, right int) bool {
		return rows[left].Timestamp < rows[right].Timestamp
	})

	now := uint64(time.Now().UnixNano())
	future := sort.Search(len(rows), func(index int) bool {
		return rows[index].Timestamp > now
	})

	if future == 0 || future == len(rows) {
		return rows
	}

	count := min(future, len(rows)-future)
	return append(
		append([]types.Prediction(nil), rows[future-count:future]...),
		rows[future:future+count]...,
	)
}

func (price *Price) ObservePredictions(rows []types.Prediction) {
	if price == nil || len(rows) == 0 {
		return
	}

	current, _ := price.predictions.Load().(map[string][]types.Prediction)
	next := make(map[string][]types.Prediction, len(current))

	for symbol, stored := range current {
		next[symbol] = append([]types.Prediction(nil), stored...)
	}

	grouped := map[string][]types.Prediction{}

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)
		if symbol == "" {
			continue
		}

		row.Symbol = symbol
		grouped[symbol] = append(grouped[symbol], row)
	}

	for symbol, stored := range grouped {
		next[symbol] = append([]types.Prediction(nil), stored...)
	}

	price.predictions.Store(next)
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

			price.observeTickerRows(kraken.NewTickerDataSlice(msg))
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

			price.observeInstrumentData(kraken.NewInstrumentData(msg))
		}
	}
}

func (price *Price) observeTickerRows(rows kraken.TickerDataSlice) {
	if len(rows) == 0 {
		return
	}

	current, _ := price.tickers.Load().(map[string]kraken.TickerData)
	next := make(map[string]kraken.TickerData, len(current)+len(rows))

	for symbol, ticker := range current {
		next[symbol] = ticker
	}

	changed := false

	for _, ticker := range rows {
		symbol := strings.TrimSpace(ticker.Symbol)
		if symbol == "" {
			continue
		}

		ticker.Symbol = symbol
		next[symbol] = ticker
		changed = true
	}

	if changed {
		price.tickers.Store(next)
	}
}

func (price *Price) observeInstrumentData(data kraken.InstrumentData) {
	symbols := price.instrumentSymbols(data)
	if len(symbols) == 0 {
		return
	}

	schedule, err := price.private.TradeVolume(symbols)
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"broker price: TradeVolume failed",
			err,
		))
		return
	}

	price.observeFeeSchedule(schedule)
}

func (price *Price) observeFeeSchedule(schedule websocket.FeeSchedule) {
	if len(schedule.Pairs) == 0 {
		return
	}

	current, _ := price.fees.Load().(map[string]websocket.FeeRates)
	next := make(map[string]websocket.FeeRates, len(current)+len(schedule.Pairs))

	for symbol, rates := range current {
		next[symbol] = rates
	}

	for symbol, rates := range schedule.Pairs {
		symbol = strings.TrimSpace(symbol)
		if symbol == "" {
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

	scale := int(max(
		entry.GetScale(),
		exit.GetScale(),
		qty.GetScale(),
		decimal.NewFromFloat64(feeRate).GetScale(),
	))
	entryRat := entry.Rat()
	exitRat := exit.Rat()
	qtyRat := qty.Rat()
	feeRat := decimal.NewFromFloat64(feeRate).Rat()
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
	net, err := decimal.NewFromString(netRat.FloatString(scale))

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"broker price: invalid PnL calculation",
			err,
		))
		return decimal.Decimal{}
	}

	return *net
}
