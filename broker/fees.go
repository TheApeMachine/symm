package broker

import (
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

type Fees struct {
	price *Price
}

func NewFees(price *Price) *Fees {
	return &Fees{price: price}
}

func (fees *Fees) On(data []byte) {
	instrumentData := kraken.NewInstrumentData(data)
	symbols := fees.instrumentSymbols(instrumentData)
	nextSymbols := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		nextSymbols[symbol] = struct{}{}
	}

	fees.price.symbols.Store(nextSymbols)
	current, _ := fees.price.tickers.Load().(map[string]kraken.TickerData)
	next := make(map[string]kraken.TickerData, len(nextSymbols))

	for symbol, ticker := range current {
		if _, ok := nextSymbols[symbol]; ok {
			next[symbol] = ticker
		}
	}

	if len(next) != len(current) {
		fees.price.tickers.Store(next)
	}

	if len(symbols) == 0 {
		fees.price.fees.Store(map[string]kraken.FeeRates{})
		fees.price.tickers.Store(map[string]kraken.TickerData{})
		return
	}

	scheduleBody, err := fees.price.private.Post(
		websocket.TradeVolumeEndpoint,
		kraken.NewTradeVolumeRequest(symbols),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"broker price: TradeVolume failed",
			err,
		))
		return
	}

	schedule := kraken.FeeSchedule{}

	if err := sonic.Unmarshal(scheduleBody, &schedule); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"broker price: TradeVolume decode failed",
			err,
		))
		return
	}

	fees.schedule(schedule)
}

func (fees *Fees) schedule(feeSchedule kraken.FeeSchedule) {
	symbols, _ := fees.price.symbols.Load().(map[string]struct{})

	if len(symbols) == 0 {
		fees.price.fees.Store(map[string]kraken.FeeRates{})
		return
	}

	next := make(map[string]kraken.FeeRates, len(symbols))

	for symbol, rates := range feeSchedule.Pairs {
		symbol = strings.TrimSpace(symbol)

		if symbol == "" {
			continue
		}

		if _, ok := symbols[symbol]; !ok {
			continue
		}

		next[symbol] = rates
	}

	fees.price.fees.Store(next)
}

func (fees *Fees) instrumentSymbols(data kraken.InstrumentData) []string {
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
