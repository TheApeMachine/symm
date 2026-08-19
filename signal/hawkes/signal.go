package hawkes

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures the buy/sell trade-arrival process as a bivariate Hawkes
process. It is ONLY a nomagique Number: one self-adapting numeric unit per
symbol that maps each signed trade mark into fitted self- and cross-exciting
intensities.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	thesis *types.Thesis
	number nomagique.Number[string]
}

/*
NewSignal constructs the Hawkes pipeline and starts it in its own goroutine.
*/
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
			nomagique.Pipe(algo.Hawkes()),
		),
	}

	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceHawkes)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceHawkes
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

				for trade := range symbol.MarketTrades(types.SourceHawkes) {
					input := nomagique.Frame{}
					input.Put(algo.SymbolMark, markForSide(trade.Side))
					input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"hawkes: number step failed for "+symbol.Symbol,
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
						nmtypes.NewMetric("buy_intensity", output.MustGet(algo.SymbolLambdaAlpha), nmtypes.Descriptor{
							Unit:      nmtypes.UnitRate,
							Timescale: nmtypes.TimescalePerSecond,
						}),
						nmtypes.NewMetric("sell_intensity", output.MustGet(algo.SymbolLambdaBeta), nmtypes.Descriptor{
							Unit:      nmtypes.UnitRate,
							Timescale: nmtypes.TimescalePerSecond,
						}),
						nmtypes.NewMetric("buy_arrival_rate", output.MustGet(algo.SymbolAlphaArrivalRate), nmtypes.Descriptor{
							Unit:      nmtypes.UnitRate,
							Timescale: nmtypes.TimescalePerSecond,
						}),
						nmtypes.NewMetric("sell_arrival_rate", output.MustGet(algo.SymbolBetaArrivalRate), nmtypes.Descriptor{
							Unit:      nmtypes.UnitRate,
							Timescale: nmtypes.TimescalePerSecond,
						}),
						nmtypes.NewMetric("event_count", output.MustGet(algo.SymbolEventCount), nmtypes.Descriptor{
							Unit:      nmtypes.UnitCount,
							Timescale: nmtypes.TimescalePerTick,
						}),
					))
				}

				return true
			})
		}
	}
}

/*
markForSide encodes one trade's aggressor side into the process mark: buy is
the positive (alpha) channel, sell the negative (beta) channel.
*/
func markForSide(side string) float64 {
	if side == "buy" {
		return 1
	}

	return -1
}

/*
Close releases the receiver's owned resources so shutdown does not leave active
market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
