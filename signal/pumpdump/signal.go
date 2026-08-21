package pumpdump

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
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
	absolute     nomagique.Primitive
	decompose    nomagique.Primitive
	polarize     nomagique.Primitive
	separate     nomagique.Primitive
	work         *transport.Consumer[*types.Symbol]
}

func NewSignal(
	ctx context.Context,
	thesis *types.Thesis,
	books websocket.BookSource,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		thesis:       thesis,
		books:        books,
		geometry:     nomagique.NewNumber[string](equation.Geometry()),
		depthChange:  nomagique.NewNumber[seriesKey](equation.RelativeChange(nomagique.SampleValue)),
		tickerChange: nomagique.NewNumber[string](equation.LogChange(nomagique.SampleValue)),
		acceleration: nomagique.NewNumber[string](equation.Acceleration()),
		normalize:    nomagique.NewNumber[seriesKey](equation.Normalize()),
		rateChange:   nomagique.NewNumber[seriesKey](equation.RelativeChange(nomagique.SampleValue)),
		absolute: nomagique.Pipe(
			nomagique.Relay(equation.SymbolChange, calculus.SymbolValue),
			calculus.Absolute,
			nomagique.Relay(calculus.SymbolResult, nomagique.SampleValue),
		),
		decompose: equation.Decompose(),
		polarize:  equation.Polarize(),
		separate:  statistic.Separation,
	}
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourcePumpDump).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string { return string(types.SourcePumpDump) }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Type() types.SourceType { return types.SourcePumpDump }

func (signal *Signal) consume() {
	go func() {
		defer func() { signal.thesis.Fail(signal.err) }()

		for symbol := range signal.thesis.Work(types.SourcePumpDump).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.err = signal.ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			if err := signal.consumeSymbol(symbol); err != nil {
				signal.err = errnie.Error(errnie.Err(
					errnie.Validation,
					"pumpdump: condition "+symbol.Symbol,
					err,
				))
				return
			}
		}
	}()
}

func (signal *Signal) consumeSymbol(symbol *types.Symbol) error {
	for level3 := range symbol.MarketLevel3(
		symbol.Level3Consumers[types.Level3ConsumerPumpDump],
	) {
		if err := signal.consumeLevel3(symbol, level3); err != nil {
			return err
		}
	}

	for ticker := range symbol.MarketTickers(
		symbol.TickerConsumers[types.TickerConsumerPumpDump],
	) {
		if err := signal.consumeTicker(symbol, ticker); err != nil {
			return err
		}
	}

	for trade := range symbol.MarketTrades(
		symbol.TradeConsumers[types.TradeConsumerPumpDump],
	) {
		if err := signal.consumeTrade(symbol, trade); err != nil {
			return err
		}
	}

	return nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
