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
	"github.com/theapemachine/symm/signal/morphology"
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
		// Hawkes is the manifold forcing term: it publishes once onto the
		// dedicated Hawkes topic (raw), and a pass-through forwards that same
		// measurement onto ChannelMeasurements so Category/Graph/Planner keep
		// receiving it without Hawkes computing twice.
		runtime.WireKeyed[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelHawkes,
			func(trade kraken.TradeData) string { return trade.Symbol },
			func(trade kraken.TradeData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return hawkesSignal.Step(trade)
				}

				started := time.Now()
				measurement := hawkesSignal.Step(trade)
				runner.ObserveModule("hawkes", time.Since(started))

				return measurement
			},
		)
		runtime.WireFunc[*data.Measurement[float64], *data.Measurement[float64]](
			workspace, types.ChannelHawkes, types.ChannelMeasurements, func(m *data.Measurement[float64]) *data.Measurement[float64] {
				return m
			},
		)

		correlationSignal := correlation.NewSignal(ctx, workspace)
		runtime.WireKeyed[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements,
			func(ticker kraken.TickerData) string { return ticker.Symbol },
			func(ticker kraken.TickerData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return correlationSignal.Step(ticker)
				}

				started := time.Now()
				measurement := correlationSignal.Step(ticker)
				runner.ObserveModule("correlation", time.Since(started))

				return measurement
			},
		)

		cvdSignal := cvd.NewSignal(ctx, workspace)
		runtime.WireKeyed[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements,
			func(trade kraken.TradeData) string { return trade.Symbol },
			func(trade kraken.TradeData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return cvdSignal.Step(trade)
				}

				started := time.Now()
				measurement := cvdSignal.Step(trade)
				runner.ObserveModule("cvd", time.Since(started))

				return measurement
			},
		)

		depthflowSignal := depthflow.NewSignal(ctx, workspace)
		runtime.WireKeyed[kraken.Level3Data, *data.Measurement[float64]](
			workspace, types.ChannelLevel3, types.ChannelMeasurements,
			func(frame kraken.Level3Data) string { return frame.Symbol },
			func(frame kraken.Level3Data) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return depthflowSignal.Step(frame.Symbol, frame.Timestamp)
				}

				started := time.Now()
				measurement := depthflowSignal.Step(frame.Symbol, frame.Timestamp)
				runner.ObserveModule("depthflow", time.Since(started))

				return measurement
			},
		)

		morphologySignal := morphology.NewSignal(ctx, workspace)
		runtime.WireKeyed[kraken.Level3Data, *data.Measurement[float64]](
			workspace, types.ChannelLevel3, types.ChannelMeasurements,
			func(frame kraken.Level3Data) string { return frame.Symbol },
			func(frame kraken.Level3Data) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return morphologySignal.Step(frame.Symbol, frame.Timestamp)
				}

				started := time.Now()
				measurement := morphologySignal.Step(frame.Symbol, frame.Timestamp)
				runner.ObserveModule("morphology", time.Since(started))

				return measurement
			},
		)

		derivativesSignal := derivatives.NewSignal(ctx, workspace)
		runtime.WireKeyed[kraken.FuturesTickerData, *data.Measurement[float64]](
			workspace, types.ChannelFuturesTickers, types.ChannelMeasurements,
			func(ticker kraken.FuturesTickerData) string { return ticker.Symbol },
			func(ticker kraken.FuturesTickerData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return derivativesSignal.StepTicker(ticker)
				}

				started := time.Now()
				measurement := derivativesSignal.StepTicker(ticker)
				runner.ObserveModule("derivatives", time.Since(started))

				return measurement
			},
		)
		runtime.WireKeyed[kraken.FuturesTradeData, *data.Measurement[float64]](
			workspace, types.ChannelFuturesTrades, types.ChannelMeasurements,
			func(trade kraken.FuturesTradeData) string { return trade.Symbol },
			func(trade kraken.FuturesTradeData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return derivativesSignal.StepTrade(trade)
				}

				started := time.Now()
				measurement := derivativesSignal.StepTrade(trade)
				runner.ObserveModule("derivatives", time.Since(started))

				return measurement
			},
		)

		exhaustSignal := exhaust.NewSignal(ctx)
		runtime.WireKeyed[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements,
			func(ticker kraken.TickerData) string { return ticker.Symbol },
			func(ticker kraken.TickerData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return exhaustSignal.Step(ticker)
				}

				started := time.Now()
				measurement := exhaustSignal.Step(ticker)
				runner.ObserveModule("exhaustion", time.Since(started))

				return measurement
			},
		)

		leadlagSignal := leadlag.NewSignal(ctx, workspace)
		runtime.WireKeyed[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements,
			func(ticker kraken.TickerData) string { return ticker.Symbol },
			func(ticker kraken.TickerData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return leadlagSignal.Step(ticker)
				}

				started := time.Now()
				measurement := leadlagSignal.Step(ticker)
				runner.ObserveModule("leadlag", time.Since(started))

				return measurement
			},
		)

		liquiditySignal := liquidity.NewSignal(ctx)
		runtime.WireKeyed[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements,
			func(ticker kraken.TickerData) string { return ticker.Symbol },
			func(ticker kraken.TickerData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return liquiditySignal.Step(ticker)
				}

				started := time.Now()
				measurement := liquiditySignal.Step(ticker)
				runner.ObserveModule("liquidity", time.Since(started))

				return measurement
			},
		)

		pumpdumpSignal := pumpdump.NewSignal(ctx, workspace)
		runtime.WireKeyed[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements,
			func(ticker kraken.TickerData) string { return ticker.Symbol },
			func(ticker kraken.TickerData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return pumpdumpSignal.StepTicker(ticker)
				}

				started := time.Now()
				measurement := pumpdumpSignal.StepTicker(ticker)
				runner.ObserveModule("pumpdump", time.Since(started))

				return measurement
			},
		)
		runtime.WireKeyed[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements,
			func(trade kraken.TradeData) string { return trade.Symbol },
			func(trade kraken.TradeData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return pumpdumpSignal.StepTrade(trade)
				}

				started := time.Now()
				measurement := pumpdumpSignal.StepTrade(trade)
				runner.ObserveModule("pumpdump", time.Since(started))

				return measurement
			},
		)

		sentimentSignal := sentiment.NewSignal(ctx, workspace)
		runtime.WireKeyed[kraken.TickerData, *data.Measurement[float64]](
			workspace, types.ChannelTickers, types.ChannelMeasurements,
			func(ticker kraken.TickerData) string { return ticker.Symbol },
			func(ticker kraken.TickerData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return sentimentSignal.Step(ticker)
				}

				started := time.Now()
				measurement := sentimentSignal.Step(ticker)
				runner.ObserveModule("sentiment", time.Since(started))

				return measurement
			},
		)

		toxicitySignal := toxicity.NewSignal(ctx, workspace)
		runtime.WireKeyed[kraken.TradeData, *data.Measurement[float64]](
			workspace, types.ChannelTrades, types.ChannelMeasurements,
			func(trade kraken.TradeData) string { return trade.Symbol },
			func(trade kraken.TradeData) *data.Measurement[float64] {
				if runner.ObserveModule == nil {
					return toxicitySignal.StepTrade(trade)
				}

				started := time.Now()
				measurement := toxicitySignal.StepTrade(trade)
				runner.ObserveModule("toxicity", time.Since(started))

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
			morphologySignal,
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
