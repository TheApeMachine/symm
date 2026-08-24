package pumpdump

import (
	"context"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"

	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

const (
	seriesAlphaDepth uint8 = iota + 1
	seriesBetaDepth
	seriesTickerDisplacement
	seriesRate
	seriesReturn
	seriesRateRatio
)

type seriesKey struct {
	symbol string
	series uint8
}

/*
Signal converts the three PumpDump market streams into measurements. Level 3
drives book geometry, ticker updates measure reference displacement, and trades
drive quantity-clocked acceleration. Persistent numeric state belongs only to
the composed Nomagique Numbers.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	thesis       *types.Thesis
	books        websocket.BookSource
	geometry     *nomagique.Number[string]
	depthChange  *nomagique.Number[seriesKey]
	tickerChange *nomagique.Number[string]
	acceleration *nomagique.Number[string]
	normalize    *nomagique.Number[seriesKey]
	rateChange   *nomagique.Number[seriesKey]
	absolute     nmtypes.Primitive
	decompose    nmtypes.Primitive
	polarize     nmtypes.Primitive
	separate     nmtypes.Primitive
	measurements *runtime.Channel[*nmtypes.Measurement]
	pool         *types.SymbolPool
}

func NewSignal(
	ctx context.Context,
	thesis *types.Thesis,
	books websocket.BookSource,
	bus *runtime.Workspace,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		thesis:       thesis,
		books:        books,
		geometry:     nomagique.NewNumber[string](equation.Geometry()),
		depthChange:  nomagique.NewNumber[seriesKey](equation.RelativeChange(nmtypes.SampleValue)),
		tickerChange: nomagique.NewNumber[string](equation.LogChange(nmtypes.SampleValue)),
		acceleration: nomagique.NewNumber[string](equation.Acceleration()),
		normalize:    nomagique.NewNumber[seriesKey](equation.Normalize()),
		rateChange:   nomagique.NewNumber[seriesKey](equation.RelativeChange(nmtypes.SampleValue)),
		absolute: nmtypes.Pipe(
			nmtypes.Relay(equation.SymbolChange, calculus.SymbolValue),
			calculus.Absolute,
			nmtypes.Relay(calculus.SymbolResult, nmtypes.SampleValue),
		),
		decompose: equation.Decompose(),
		polarize:  equation.Polarize(),
		separate:  statistic.Separation,
		pool:      types.NewSymbolPool(types.ShardWorkers()),
	}
	signal.measurements = runtime.ChannelOf[*nmtypes.Measurement](
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	)
	runtime.ChannelOf[kraken.Level3Data](
		bus, types.ChannelLevel3,
		func(frame kraken.Level3Data) string { return frame.Symbol },
	).Subscribe(signal.Name(), func(frame kraken.Level3Data) error {
		return signal.consumeLevel3(signal.thesis.Symbol(frame.Symbol), frame)
	})
	runtime.ChannelOf[kraken.TickerData](
		bus, types.ChannelTickers,
		func(ticker kraken.TickerData) string { return ticker.Symbol },
	).Subscribe(signal.Name(), func(ticker kraken.TickerData) error {
		return signal.consumeTicker(signal.thesis.Symbol(ticker.Symbol), ticker)
	})
	runtime.ChannelOf[kraken.TradeData](
		bus, types.ChannelTrades,
		func(trade kraken.TradeData) string { return trade.Symbol },
	).Subscribe(signal.Name(), func(trade kraken.TradeData) error {
		return signal.consumeTrade(signal.thesis.Symbol(trade.Symbol), trade)
	})

	return signal
}

func (signal *Signal) Name() string { return string(types.SourcePumpDump) }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Type() types.SourceType { return types.SourcePumpDump }

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if signal.pool != nil {
		signal.pool.Close()
	}

	return nil
}
