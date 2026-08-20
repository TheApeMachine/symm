package toxicity

import (
	"context"
	"runtime"

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

/*
toxicityPipeline is a pure composition: net pulled liquidity is the difference
of the retreated and filled channels, conditioned through the adaptive
baseline, then scored both as dispersion and as a squashed non-negative
toxicity intensity.
*/
func toxicityPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		calculus.Difference,
		nomagique.Relay(calculus.SymbolResult, nomagique.SampleValue),
		nomagique.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		nomagique.Fork(
			statistic.ZScore,
			nomagique.Fork(
				statistic.Deviation,
				nomagique.Pipe(
					calculus.Positive,
					calculus.Squash,
				),
			),
		),
	)
}

type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	thesis *types.Thesis
	number nomagique.Number[string]
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](toxicityPipeline()),
	}

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceToxicity) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceToxicity }

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

		signal.thesis.Symbols.Range(func(_ any, value any) bool {
			symbol, valid := value.(*types.Symbol)

			if !valid || symbol == nil {
				return true
			}

			for frame := range symbol.MarketLevel3(types.SourceToxicity) {
				filled, retreated := level3Flow(frame)

				input := nomagique.Frame{}
				input.Put(nmtypes.AlphaQuantity, filled)
				input.Put(nmtypes.BetaQuantity, retreated)
				input.Put(calculus.SymbolLeft, filled)
				input.Put(calculus.SymbolRight, retreated)
				input.Put(nomagique.SampleValue, retreated-filled)
				input.Put(calculus.SymbolValue, retreated-filled)
				input.Put(calculus.SymbolScale, 1.0)
				input.Put(nmtypes.EventTimeSec, float64(frame.Timestamp.Unix()))
				input.Put(nmtypes.EventTimeNsec, float64(frame.Timestamp.Nanosecond()))
				input.Put(statistic.SymbolDispersionHalflife, 30.0)

				output, err := signal.number(symbol.Symbol, input)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"toxicity: failed for "+symbol.Symbol,
						err,
					))
					return false
				}

				symbol.AppendMeasurement(nmtypes.NewMeasurement(
					uuid.NewString(),
					signal.Name(),
					frame.Timestamp.UnixNano(),
					frame.Timestamp.UnixNano(),
				).AddMetrics(
					nmtypes.NewMetric("honesty_zscore", output.MustGet(statistic.SymbolZScore), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
					nmtypes.NewMetric("honesty_deviation", output.MustGet(statistic.SymbolDeviation), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
					nmtypes.NewMetric("toxicity_intensity", output.MustGet(calculus.SymbolResult), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
				))
			}

			return true
		})
	}

	return signal.err
}

/*
pending reports whether any symbol queues a Level3 frame, so the
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

		if symbol.HasLevel3() {
			hasWork = true

			return false
		}

		return true
	})

	return hasWork
}

/*
level3Flow lifts one accepted order frame into the two generic quantity
channels: filled quantity (Alpha) and retreated quantity (Beta). The
iteration here is input conversion, not calculation.
*/
func level3Flow(frame kraken.Level3Data) (filled float64, retreated float64) {
	for _, orders := range [][]kraken.Level3Order{frame.Bids, frame.Asks} {
		for _, order := range orders {
			if order.OrderQty == nil {
				continue
			}

			switch order.Event {
			case "fill":
				filled += order.OrderQty.Float64()
			case "delete":
				retreated += order.OrderQty.Float64()
			}
		}
	}

	return filled, retreated
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
