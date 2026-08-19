package exhaust

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Exhaustion perspective: microstructural decay conditioned per
symbol. It is ONLY a nomagique pipeline — signed executed flow feeds an
adaptive baseline whose deviation and z-score report how far the current print
stands from the symbol's own established level, which is exhaustion rather
than a continuation.
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
				temporal.Window,
				statistic.Baseline,
				statistic.ZScore,
			),
		),
	}

	signal.run()
	return signal
}

func (signal *Signal) Name() string {
	return string(types.SourceExhaustion)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceExhaustion
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

				for trade := range symbol.MarketTrades(types.SourceExhaustion) {
					input := nomagique.Frame{}
					input.Put(nmtypes.Quantity, signedFlow(trade))
					input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))
					input.Put(statistic.SymbolDispersionHalflife, 30.0)

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"exhaust: number step failed for "+symbol.Symbol,
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
						nmtypes.NewMetric("exhaustion_zscore", output.MustGet(statistic.SymbolZScore), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("flow_baseline", output.MustGet(statistic.SymbolBaselineValue), nmtypes.Descriptor{
							Unit:      nmtypes.UnitBaseCurrency,
							Timescale: nmtypes.TimescalePerSecond,
						}),
					))
				}

				return true
			})
		}
	}
}

func signedFlow(trade kraken.TradeData) float64 {
	if trade.Side == "sell" {
		return -trade.Qty
	}

	return trade.Qty
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
