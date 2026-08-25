package signal

import (
	"context"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/derivatives"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/types"
)

/*
Runner owns and coordinates all analytical signal instances wired to the Workspace.
*/
type Runner struct {
	ctx           context.Context
	workspace     *runtime.Workspace
	ObserveModule func(name string, duration time.Duration)
	signals       []interface{ Close() error }
}

func NewRunner(ctx context.Context, workspace *runtime.Workspace) *Runner {
	runner := &Runner{
		ctx:       ctx,
		workspace: workspace,
	}

	if workspace != nil {
		hawkesSignal := hawkes.NewSignal(ctx)
		hawkesSignal.RegisterWire(workspace)

		correlationSignal := correlation.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, correlationSignal.Step,
		)

		cvdSignal := cvd.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements, cvdSignal.Step,
		)

		depthflowSignal := depthflow.NewSignal(ctx, workspace)

		derivativesSignal := derivatives.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.FuturesTickerData, *data.Measurement[float64]](
			workspace, types.ChannelFuturesTickers, types.ChannelMeasurements, derivativesSignal.StepTicker,
		)
		runtime.WireFunc[kraken.FuturesTradeData, *data.Measurement[float64]](
			workspace, types.ChannelFuturesTrades, types.ChannelMeasurements, derivativesSignal.StepTrade,
		)

		exhaustSignal := exhaust.NewSignal(ctx)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, exhaustSignal.Step,
		)

		leadlagSignal := leadlag.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, leadlagSignal.Step,
		)

		liquiditySignal := liquidity.NewSignal(ctx)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, liquiditySignal.Step,
		)

		pumpdumpSignal := pumpdump.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, pumpdumpSignal.StepTicker,
		)
		runtime.WireFunc[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements, pumpdumpSignal.StepTrade,
		)

		sentimentSignal := sentiment.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, sentimentSignal.Step,
		)

		toxicitySignal := toxicity.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements, toxicitySignal.StepTrade,
		)

		runner.signals = []interface{ Close() error }{
			correlationSignal,
			cvdSignal,
			depthflowSignal,
			derivativesSignal,
			exhaustSignal,
			hawkesSignal,
			leadlagSignal,
			liquiditySignal,
			pumpdumpSignal,
			sentimentSignal,
			toxicitySignal,
		}
	}

	return runner
}

func (runner *Runner) Close() error {
	for _, sig := range runner.signals {
		if sig != nil {
			_ = sig.Close()
		}
	}

	return nil
}
