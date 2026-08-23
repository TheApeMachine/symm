package correlation

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
	"golang.org/x/sync/errgroup"
)

type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	thesis *types.Thesis
	number *nomagique.Number[string]
	work   *transport.Consumer[*types.Symbol]
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](temporal.Path),
	}

	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)

	thesis.Work(types.SourceCorrelation).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceCorrelation) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceCorrelation }

func (signal *Signal) consume() {
	go func() {
		defer func() {
			signal.thesis.Fail(signal.err)
		}()

		group, ctx := errgroup.WithContext(signal.ctx)
		group.SetLimit(types.ShardWorkers())

		for symbol := range signal.thesis.Work(types.SourceCorrelation).Drain(
			signal.work, nil,
		) {
			select {
			case <-ctx.Done():
				signal.err = ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			symbol := symbol
			group.Go(func() error {
				return signal.consumeSymbol(symbol)
			})
		}

		if err := group.Wait(); err != nil {
			signal.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"correlation: processing failed",
				err,
			))
		}
	}()
}

func (signal *Signal) consumeSymbol(symbol *types.Symbol) error {
	for ticker := range symbol.MarketTickers(
		symbol.TickerConsumers[types.TickerConsumerCorrelation],
	) {
		input := nmtypes.Frame{}
		input.Put(nmtypes.SampleValue, ticker.Last.Float64())
		input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
		input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))
		_, err := signal.number.Step(symbol.Symbol, input)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"correlation: path failed for "+symbol.Symbol,
				err,
			))
		}

		output, reduced, err := signal.number.CrossSection(
			symbol.Symbol,
			nmcorrelation.Hayashi,
			nmcorrelation.Cohort,
			algo.Correlation(),
		)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"correlation: cohort failed for "+symbol.Symbol,
				err,
			))
		}

		ready, _ := output.Get(nmcorrelation.SymbolReady)
		separation := 0.0
		measured := reduced && ready != 0

		if measured {
			separation, _ = output.Get(algo.SymbolHypothesisSeparation)
		}

		symbol.AppendMeasurement(signal.measurement(
			symbol.Symbol,
			ticker.Timestamp,
			output,
			measured,
			separation,
			signal.support(symbol.Symbol),
		))
	}

	return nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
