package cvd

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolNetFlow    = nomagique.MustIntern("cvd/net_flow")
	SymbolAbsorption = nomagique.MustIntern("cvd/absorption")
)

/*
cvdPipeline is slot-aligned with the calculus atoms:

  - calculus.Difference reads left, right and writes result (the signed net
    flow).
  - The net flow is lifted into SampleValue for the adaptive baseline, window,
    z-score, and deviation stages.
  - Absorption is the non-negative squash of the same flow magnitude, so high
    aggressor flow with little price response reads as absorption.
*/
func cvdPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		calculus.Difference,
		nomagique.Relay(calculus.SymbolResult, SymbolNetFlow),
		nomagique.Relay(SymbolNetFlow, nomagique.SampleValue),
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
					nomagique.Relay(calculus.SymbolResult, calculus.SymbolValue),
					calculus.Squash,
					nomagique.Relay(calculus.SymbolResult, SymbolAbsorption),
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
		number: nomagique.NewNumber[string](cvdPipeline()),
	}

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceCVD) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceCVD }

func (signal *Signal) Run() error {
	for signal.err == nil {
		symbol, available := signal.thesis.Work(types.SourceCVD).WaitPop(
			signal.ctx,
			string(types.SourceCVD),
		)

		if !available {
			return signal.ctx.Err()
		}

		if symbol == nil {
			continue
		}

		for trade := range symbol.MarketTrades(types.SourceCVD) {
			notional := trade.Price.Float64() * trade.Qty

			input := nomagique.Frame{}
			input.Put(calculus.SymbolRight, notional)
			input.Put(calculus.SymbolLeft, 0)

			if trade.Side != "sell" {
				input.Put(calculus.SymbolLeft, notional)
				input.Put(calculus.SymbolRight, 0)
			}

			input.Put(calculus.SymbolValue, notional)
			input.Put(calculus.SymbolScale, 1.0)
			input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
			input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))
			input.Put(statistic.SymbolDispersionHalflife, 30.0)

			output, err := signal.number(symbol.Symbol, input)

			if err != nil {
				signal.err = errnie.Error(errnie.Err(
					errnie.Validation,
					"cvd: failed for "+symbol.Symbol,
					err,
				))
				break
			}

			symbol.AppendMeasurement(nmtypes.NewMeasurement(
				uuid.NewString(),
				signal.Name(),
				trade.Timestamp.UnixNano(),
				trade.Timestamp.UnixNano(),
			).AddMetrics(
				nmtypes.NewMetric("net", output.MustGet(SymbolNetFlow), nmtypes.Descriptor{
					Unit:      nmtypes.UnitQuoteCurrency,
					Timescale: nmtypes.TimescalePerTick,
				}),
				nmtypes.NewMetric("flow_baseline", output.MustGet(statistic.SymbolBaselineValue), nmtypes.Descriptor{
					Unit:      nmtypes.UnitQuoteCurrency,
					Timescale: nmtypes.TimescalePerSecond,
				}),
				nmtypes.NewMetric("flow_zscore", output.MustGet(statistic.SymbolZScore), nmtypes.Descriptor{
					Unit:      nmtypes.UnitDimensionless,
					Timescale: nmtypes.TimescaleInstantaneous,
				}),
				nmtypes.NewNormalizedMetric("absorption", output.MustGet(SymbolAbsorption), output.MustGet(SymbolAbsorption), nmtypes.Descriptor{
					Unit:      nmtypes.UnitDimensionless,
					Timescale: nmtypes.TimescaleInstantaneous,
				}),
			))
		}
	}

	return signal.err
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
