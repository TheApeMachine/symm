package sentiment

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var SymbolBreadth = nomagique.MustIntern("sentiment/breadth")

/*
sentimentPipeline composes market-wide breadth from the shared atomic math
units: every symbol's signed return is accumulated into the cohort's running
net (advances minus declines), and the rate of that net over the cohort's own
observed duration is the breadth reading.
*/
func sentimentPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		calculus.Accumulate,
		nomagique.Relay(calculus.SymbolTotal, nomagique.SampleValue),
		nomagique.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		nomagique.Fork(
			nomagique.Pipe(
				calculus.Rate,
				nomagique.Relay(calculus.SymbolRate, SymbolBreadth),
			),
			statistic.Deviation,
		),
	)
}

type Signal struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	cohortAt time.Time
	thesis   *types.Thesis
	number   nomagique.Number[string]
	work     *transport.Consumer[*types.Symbol]
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](sentimentPipeline()),
	}
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceSentiment).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string { return string(types.SourceSentiment) }
func (signal *Signal) Error() error { return signal.err }
func (signal *Signal) Type() types.SourceType {
	return types.SourceSentiment
}

func (signal *Signal) consume() {
	go func() {
		ready := make([]*types.Symbol, 0, 1)

		for symbol := range signal.thesis.Work(types.SourceSentiment).Drain(
			signal.work, nil,
		) {
			select {
			case <-signal.ctx.Done():
				signal.err = signal.ctx.Err()
				return
			default:
			}

			if symbol != nil {
				ready = append(ready, symbol)
			}
		}

		if signal.err = signal.step(ready); signal.err != nil {
			signal.thesis.Fail(signal.err)
		}
	}()
}

func (signal *Signal) step(symbols []*types.Symbol) error {
	observations := make([]struct {
		symbol *types.Symbol
		ticker kraken.TickerData
	}, 0)

	for _, symbol := range symbols {
		if symbol == nil {
			continue
		}

		for ticker := range symbol.MarketTickers(
			symbol.TickerConsumers[types.TickerConsumerSentiment],
		) {
			observations = append(observations, struct {
				symbol *types.Symbol
				ticker kraken.TickerData
			}{symbol: symbol, ticker: ticker})
		}

	}

	sort.SliceStable(observations, func(leftIndex int, rightIndex int) bool {
		return observations[leftIndex].ticker.Timestamp.Before(
			observations[rightIndex].ticker.Timestamp,
		)
	})

	for _, observation := range observations {
		if err := signal.measure(observation.symbol, observation.ticker); err != nil {
			return err
		}
	}

	return nil
}

func (signal *Signal) measure(symbol *types.Symbol, ticker kraken.TickerData) error {
	cohortAt := ticker.Timestamp

	if cohortAt.Before(signal.cohortAt) {
		cohortAt = signal.cohortAt
	}

	if cohortAt.After(signal.cohortAt) {
		signal.cohortAt = cohortAt
	}

	input := nomagique.Frame{}
	input.Put(calculus.SymbolDelta, signedReturn(ticker))
	input.Put(calculus.SymbolCount, 1)
	input.Put(calculus.SymbolDuration, 1)
	input.Put(nmtypes.EventTimeSec, float64(cohortAt.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(cohortAt.Nanosecond()))

	output, err := signal.number("cohort", input)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"sentiment: failed for "+symbol.Symbol,
			err,
		))
	}

	symbol.AppendMeasurement(nmtypes.NewMeasurement(
		uuid.NewString(),
		signal.Name(),
		ticker.Timestamp.UnixNano(),
		ticker.Timestamp.UnixNano(),
	).AddMetrics(
		nmtypes.NewMetric("breadth", output.MustGet(SymbolBreadth), nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
	))

	return nil
}

func signedReturn(ticker kraken.TickerData) float64 {
	if ticker.Change == nil {
		return 0
	}

	return ticker.Change.Float64()
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
