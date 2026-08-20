package sentiment

import (
	"context"
	"runtime"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
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
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](sentimentPipeline()),
	}

	return signal
}

func (signal *Signal) Name() string { return string(types.SourceSentiment) }
func (signal *Signal) Error() error { return signal.err }
func (signal *Signal) Type() types.SourceType {
	return types.SourceSentiment
}

func (signal *Signal) Run() error {
	for signal.err == nil {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		default:
		}

		if !signal.pending() {
			// Nothing queued for this signal; yield before polling again.
			runtime.Gosched()
			continue
		}

		if signal.err = signal.step(); signal.err != nil {
			return signal.err
		}
	}

	return signal.err
}

func (signal *Signal) step() error {
	observations := make([]struct {
		symbol *types.Symbol
		ticker kraken.TickerData
	}, 0)

	signal.thesis.Symbols.Range(func(_ any, value any) bool {
		symbol, valid := value.(*types.Symbol)

		if !valid || symbol == nil {
			return true
		}

		for ticker := range symbol.MarketTickers(types.SourceSentiment) {
			observations = append(observations, struct {
				symbol *types.Symbol
				ticker kraken.TickerData
			}{symbol: symbol, ticker: ticker})
		}

		return true
	})

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

/*
pending reports whether any symbol queues a Tickers frame, so the
run loop can yield without draining empty input.
*/
func (signal *Signal) pending() bool {
	if signal.thesis == nil {
		return false
	}

	hasWork := false

	signal.thesis.Symbols.Range(func(_ any, value any) bool {
		symbol, valid := value.(*types.Symbol)

		if !valid || symbol == nil {
			return true
		}

		if symbol.HasTickers() {
			hasWork = true

			return false
		}

		return true
	})

	return hasWork
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
