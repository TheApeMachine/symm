package fluid

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal is a fluid signal that observes market data and calculates measurements.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	instrument  *broker.Instrument
	registry    *Registry
	ticker      *Ticker
	trade       *Trade
	book        *Book
	tickerCache *types.MarketFeed[kraken.TickerData]
	tradeCache  *types.MarketFeed[kraken.TradeData]
	bookCache   *types.MarketFeed[kraken.BookData]
	ui          chan []byte
}

/*
observeTicker retains replay data for direct signal tests. Live ingestion is owned
exclusively by trader.Market and does not register this method.
*/
func (signal *Signal) observeTicker(data []byte) {
	frame := utils.Unmarshal[kraken.Ticker](data)

	for _, row := range frame.Data {
		if err := signal.tickerCache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			errnie.Error(err)
			return
		}
	}
}

/*
observeTrade retains replay data for direct signal tests. Live ingestion is owned
exclusively by trader.Market and does not register this method.
*/
func (signal *Signal) observeTrade(data []byte) {
	frame := kraken.NewTrade(data)

	for _, row := range frame.Data {
		if err := signal.tradeCache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			errnie.Error(err)
			return
		}
	}
}

/*
observeBook retains enriched replay data for direct signal tests. Live ingestion is
owned exclusively by trader.Market and does not register this method.
*/
func (signal *Signal) observeBook(data []byte) {
	frame := kraken.NewBook(data)

	for _, row := range frame.Data {
		increment, err := signal.increment(row.Symbol)

		if err != nil {
			errnie.Error(err)
			return
		}

		row.PriceIncrement = increment

		if err := signal.bookCache.Observe(row.Symbol, row.Timestamp, row); err != nil {
			errnie.Error(err)
			return
		}
	}
}

/*
increment resolves exchange tick size for direct replay. Central live book
ingestion performs this enrichment before producing the shared market cut.
*/
func (signal *Signal) increment(symbol string) (*decimal.Decimal, error) {
	if signal.instrument == nil {
		return decimal.NewFromInt64(0), nil
	}

	pair, err := signal.instrument.Pair(symbol)

	if err != nil {
		return nil, err
	}

	return pair.PriceIncrement.Copy(), nil
}

/*
Interest requires ticker, trade, and book streams; the mechanical metrics merge
all three inputs into one causal event timeline per symbol.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamAll
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements, err := signal.Calculate(thesis.Market())

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return measurements
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.WireMeasurements(measurements),
	}.Marshal():
	default:
	}
}

func NewSignal(
	ctx context.Context, api *websocket.API, instrument *broker.Instrument, ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	registry := NewSyncRegistry()

	signal := &Signal{
		ctx:        ctx,
		cancel:     cancel,
		instrument: instrument,
		registry:   registry,
		ui:         ui,
		ticker:     NewTicker(registry),
		trade:      NewTrade(registry),
		book:       NewBook(registry),
	}

	return signal
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
