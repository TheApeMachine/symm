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

func (runner *Runner) timeStep(name string, duration time.Duration) {
	if runner.ObserveModule != nil {
		runner.ObserveModule(name, duration)
	}
}

func NewRunner(ctx context.Context, workspace *runtime.Workspace) *Runner {
	runner := &Runner{
		ctx:       ctx,
		workspace: workspace,
	}

	if workspace != nil {
		hawkesSignal := hawkes.NewSignal(ctx)
		runtime.WireFunc[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements, func(trade kraken.TradeData) *data.Measurement[float64] {
				started := time.Now()
				measurement := hawkesSignal.Step(trade)
				runner.timeStep("hawkes", time.Since(started))
				return measurement
			},
		)

		correlationSignal := correlation.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, func(ticker kraken.TickerData) *data.Measurement[float64] {
				started := time.Now()
				measurement := correlationSignal.Step(ticker)
				runner.timeStep("correlation", time.Since(started))
				return measurement
			},
		)

		cvdSignal := cvd.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements, func(trade kraken.TradeData) *data.Measurement[float64] {
				started := time.Now()
				measurement := cvdSignal.Step(trade)
				runner.timeStep("cvd", time.Since(started))
				return measurement
			},
		)

		depthflowSignal := depthflow.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.Level3Data, *data.Measurement[float64]](
			workspace, types.ChannelLevel3, types.ChannelMeasurements, func(frame kraken.Level3Data) *data.Measurement[float64] {
				started := time.Now()
				measurement := depthflowSignal.Step(frame.Symbol, frame.Timestamp)
				runner.timeStep("depthflow", time.Since(started))
				return measurement
			},
		)

		derivativesSignal := derivatives.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.FuturesTickerData, *data.Measurement[float64]](
			workspace, types.ChannelFuturesTickers, types.ChannelMeasurements, func(ticker kraken.FuturesTickerData) *data.Measurement[float64] {
				started := time.Now()
				measurement := derivativesSignal.StepTicker(ticker)
				runner.timeStep("derivatives", time.Since(started))
				return measurement
			},
		)
		runtime.WireFunc[kraken.FuturesTradeData, *data.Measurement[float64]](
			workspace, types.ChannelFuturesTrades, types.ChannelMeasurements, func(trade kraken.FuturesTradeData) *data.Measurement[float64] {
				started := time.Now()
				measurement := derivativesSignal.StepTrade(trade)
				runner.timeStep("derivatives", time.Since(started))
				return measurement
			},
		)

		exhaustSignal := exhaust.NewSignal(ctx)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, func(ticker kraken.TickerData) *data.Measurement[float64] {
				started := time.Now()
				measurement := exhaustSignal.Step(ticker)
				runner.timeStep("exhaustion", time.Since(started))
				return measurement
			},
		)

		leadlagSignal := leadlag.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, func(ticker kraken.TickerData) *data.Measurement[float64] {
				started := time.Now()
				measurement := leadlagSignal.Step(ticker)
				runner.timeStep("leadlag", time.Since(started))
				return measurement
			},
		)

		liquiditySignal := liquidity.NewSignal(ctx)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, func(ticker kraken.TickerData) *data.Measurement[float64] {
				started := time.Now()
				measurement := liquiditySignal.Step(ticker)
				runner.timeStep("liquidity", time.Since(started))
				return measurement
			},
		)

		pumpdumpSignal := pumpdump.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, func(ticker kraken.TickerData) *data.Measurement[float64] {
				started := time.Now()
				measurement := pumpdumpSignal.StepTicker(ticker)
				runner.timeStep("pumpdump", time.Since(started))
				return measurement
			},
		)
		runtime.WireFunc[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements, func(trade kraken.TradeData) *data.Measurement[float64] {
				started := time.Now()
				measurement := pumpdumpSignal.StepTrade(trade)
				runner.timeStep("pumpdump", time.Since(started))
				return measurement
			},
		)

		sentimentSignal := sentiment.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements, func(ticker kraken.TickerData) *data.Measurement[float64] {
				started := time.Now()
				measurement := sentimentSignal.Step(ticker)
				runner.timeStep("sentiment", time.Since(started))
				return measurement
			},
		)

		toxicitySignal := toxicity.NewSignal(ctx, workspace)
		runtime.WireFunc[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements, func(trade kraken.TradeData) *data.Measurement[float64] {
				started := time.Now()
				measurement := toxicitySignal.StepTrade(trade)
				runner.timeStep("toxicity", time.Since(started))
				return measurement
			},
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
