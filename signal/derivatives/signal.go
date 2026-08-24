package derivatives

import (
	"context"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal converts real-time Kraken Futures streams (tickers, trades, and
order books) into multi-dimensional derivatives measurements via nomagique.
*/
type Signal struct {
	ctx            context.Context
	cancel         context.CancelFunc
	err            error
	thesis         *types.Thesis
	oi             *nomagique.Number[string]
	oiAcceleration *nomagique.Number[string]
	basis          *nomagique.Number[string]
	basisVelocity  *nomagique.Number[string]
	indexBasis     *nomagique.Number[string]
	flow           *nomagique.Number[string]
	liquidations   *nomagique.Number[string]
	tickerPrice    *nomagique.Number[string]
	tradePrice     *nomagique.Number[string]
	measurements   *runtime.Channel[*nmtypes.Measurement]
	data           sync.Map
	pool           *types.SymbolPool
}

/*
NewSignal constructs a new Derivatives Signal.
*/
func NewSignal(ctx context.Context, thesis *types.Thesis, bus *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:            ctx,
		cancel:         cancel,
		thesis:         thesis,
		oi:             nomagique.NewNumber[string](equation.RelativeChange(nmtypes.SampleValue)),
		oiAcceleration: nomagique.NewNumber[string](oiAccelerationPipeline()),
		basis:          nomagique.NewNumber[string](basisPipeline()),
		basisVelocity:  nomagique.NewNumber[string](basisVelocityPipeline()),
		indexBasis:     nomagique.NewNumber[string](indexBasisPipeline()),
		flow:           nomagique.NewNumber[string](flowPipeline()),
		liquidations:   nomagique.NewNumber[string](liqPipeline()),
		tickerPrice:    nomagique.NewNumber[string](equation.RelativeChange(nmtypes.SampleValue)),
		tradePrice:     nomagique.NewNumber[string](equation.RelativeChange(nmtypes.SampleValue)),
		pool:           types.NewSymbolPool(types.ShardWorkers()),
	}

	signal.measurements = runtime.ChannelOf[*nmtypes.Measurement](
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	)
	runtime.ChannelOf[kraken.FuturesTickerData](
		bus, types.ChannelFuturesTickers,
		func(ticker kraken.FuturesTickerData) string { return ticker.Symbol },
	).Subscribe(signal.Name(), signal.onTicker)
	runtime.ChannelOf[kraken.FuturesTradeData](
		bus, types.ChannelFuturesTrades,
		func(trade kraken.FuturesTradeData) string { return trade.Symbol },
	).Subscribe(signal.Name(), signal.onTrade)
	runtime.ChannelOf[kraken.FuturesBookData](
		bus, types.ChannelFuturesBooks,
		func(book kraken.FuturesBookData) string { return book.Symbol },
	).Subscribe(signal.Name(), signal.onBook)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceDerivatives) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceDerivatives }

// onTicker advances one symbol's derivatives accumulator from a futures
// ticker and publishes the latest measurement downstream.
func (signal *Signal) onTicker(ticker kraken.FuturesTickerData) error {
	symbol := signal.thesis.Symbol(ticker.Symbol)
	data := signal.symbolData(symbol.Symbol)

	if err := signal.consumeTicker(symbol, ticker, data); err != nil {
		return err
	}

	return signal.emit(symbol, ticker.Timestamp, data)
}

func (signal *Signal) onTrade(trade kraken.FuturesTradeData) error {
	symbol := signal.thesis.Symbol(trade.Symbol)
	data := signal.symbolData(symbol.Symbol)

	if err := signal.consumeTrade(symbol, trade, data); err != nil {
		return err
	}

	return signal.emit(symbol, trade.Timestamp, data)
}

func (signal *Signal) onBook(book kraken.FuturesBookData) error {
	symbol := signal.thesis.Symbol(book.Symbol)
	return signal.emit(symbol, book.Timestamp, signal.symbolData(symbol.Symbol))
}

func (signal *Signal) symbolData(symbol string) *DerivativesData {
	loaded, _ := signal.data.LoadOrStore(symbol, &DerivativesData{})
	return loaded.(*DerivativesData)
}

func (signal *Signal) emit(symbol *types.Symbol, at time.Time, data *DerivativesData) error {
	measurement, err := BuildMeasurement(signal.Name(), symbol.Symbol, at, *data)

	if err != nil {
		return err
	}

	types.PublishMeasurement(signal.thesis, signal.measurements, symbol.Symbol, measurement)

	return nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if signal.pool != nil {
		signal.pool.Close()
	}

	return nil
}
