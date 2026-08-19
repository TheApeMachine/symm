package liquidity

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
Signal is the Liquidity perspective. It is ONLY a nomagique Number pipeline:
depth extraction, adaptive window, adaptive baseline, and deviation compose a
single numeric unit that maps an encoded ticker row to a measurement. The
signal owns nothing except that composed number and its own goroutine.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	thesis *types.Thesis
	number nomagique.Number
}

/*
NewSignal constructs the liquidity pipeline as one composed nomagique.Number
and starts it in its own goroutine. Nothing else is owned by the signal.
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
		number: nomagique.NewNumber(
			statistic.ExtractDepth,
			nomagique.Configure(
				statistic.Baseline,
				nmtypes.Span,
				temporal.Window,
			),
		),
	}

	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLiquidity)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceLiquidity
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

				for ticker := range symbol.MarketTickers(types.SourceLiquidity) {
					input := nomagique.Frame{}
					input.Put(nmtypes.AlphaPrice, ticker.Bid.Float64())
					input.Put(nmtypes.BetaPrice, ticker.Ask.Float64())
					input.Put(nmtypes.AlphaQuantity, ticker.BidQty)
					input.Put(nmtypes.BetaQuantity, ticker.AskQty)
					input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))

					output, err := signal.number(input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"liquidity: signal failed for "+symbol.Symbol,
							err,
						))
					}

					value, _ := output.Get(nomagique.SampleValue)

					symbol.Measurements.Push(nmtypes.NewMeasurement(
						uuid.NewString(),
						signal.Name(),
					).AddMetrics(
						nmtypes.NewMetric(
							"executable_touch_depth",
							value,
							nmtypes.Descriptor{Unit: nmtypes.UnitRate},
						),
					))
				}

				return true
			})
		}
	}
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
