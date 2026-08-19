package depthflow

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the DepthFlow perspective: touch-level scarcity conditioned per
symbol. It is ONLY a nomagique pipeline — each trade's signed execution feeds
an adaptive baseline whose deviation reports how far the current flow stands
from the symbol's own established pressure.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	thesis *types.Thesis
	number nomagique.Number[string]
}

func NewSignal(
	ctx context.Context,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](
			nomagique.Pipe(
				nomagique.Configure(
					temporal.Window,
					nmtypes.Span,
					statistic.Baseline,
				),
				statistic.Deviation,
			),
		),
	}

	signal.run()
	return signal
}

func (signal *Signal) Name() string {
	return string(types.SourceDepthFlow)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceDepthFlow
}

func (signal *Signal) run() {
	for {
		select {
		case <-signal.ctx.Done():
			return
		default:
			signal.thesis.Symbols.Range(func(_ any, value any) bool {
				symbol, valid := value.(*types.Symbol)

				if !valid || symbol == nil {
					return true
				}

				for trade := range symbol.MarketTrades(types.SourceDepthFlow) {
					input := nomagique.Frame{}
					input.Put(nmtypes.Quantity, trade.Qty)
					input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"depthflow: number step failed for "+symbol.Symbol,
							err,
						))
						continue
					}

					symbol.Measurements.Push(nmtypes.NewMeasurement(
						uuid.NewString(),
						signal.Name(),
						trade.Timestamp.UnixNano(),
						trade.Timestamp.UnixNano(),
					).AddMetrics(
						nmtypes.NewMetric("depth_baseline", output.MustGet(statistic.SymbolBaselineValue), nmtypes.Descriptor{
							Unit:      nmtypes.UnitBaseCurrency,
							Timescale: nmtypes.TimescalePerSecond,
						}),
						nmtypes.NewMetric("depth_deviation", output.MustGet(statistic.SymbolDeviation), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
					))
				}

				return true
			})
		}
	}
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
