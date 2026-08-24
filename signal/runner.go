package signal

import (
	"context"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	signalcorrelation "github.com/theapemachine/symm/signal/correlation"
	signalcvd "github.com/theapemachine/symm/signal/cvd"
	signaldepthflow "github.com/theapemachine/symm/signal/depthflow"
	signalderivatives "github.com/theapemachine/symm/signal/derivatives"
	signalexhaust "github.com/theapemachine/symm/signal/exhaust"
	signalhawkes "github.com/theapemachine/symm/signal/hawkes"
	signalleadlag "github.com/theapemachine/symm/signal/leadlag"
	signalliquidity "github.com/theapemachine/symm/signal/liquidity"
	signalpumpdump "github.com/theapemachine/symm/signal/pumpdump"
	signalsentiment "github.com/theapemachine/symm/signal/sentiment"
	signaltoxicity "github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/types"
)

/*
Runner coordinates the concurrent execution of all trading signals over the
workspace streaming bus. Each signal subscribes independently to the market
channels it consumes, executing steps concurrently on the bus worker pool
per symbol and publishing projected measurements to ChannelMeasurements.
*/
type Runner struct {
	ctx          context.Context
	cancel       context.CancelFunc
	bus          *runtime.Workspace
	measurements *runtime.Channel[*nmtypes.Measurement]

	ObserveModule func(string, time.Duration)

	correlation *signalcorrelation.Signal
	cvd         *signalcvd.Signal
	derivatives *signalderivatives.Signal
	depthflow   *signaldepthflow.Signal
	exhaust     *signalexhaust.Signal
	hawkes      *signalhawkes.Signal
	leadlag     *signalleadlag.Signal
	liquidity   *signalliquidity.Signal
	pumpdump    *signalpumpdump.Signal
	sentiment   *signalsentiment.Signal
	toxicity    *signaltoxicity.Signal
}

/*
NewRunner constructs and wires all signals to the workspace streaming bus.
*/
func NewRunner(
	ctx context.Context,
	bus *runtime.Workspace,
) *Runner {
	ctx, cancel := context.WithCancel(ctx)

	runner := &Runner{
		ctx:    ctx,
		cancel: cancel,
		bus:    bus,
		measurements: runtime.ChannelOf[*nmtypes.Measurement](
			bus, types.ChannelMeasurements,
			func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
		),
		correlation: signalcorrelation.NewSignal(ctx, bus),
		cvd:         signalcvd.NewSignal(ctx),
		derivatives: signalderivatives.NewSignal(ctx, bus),
		depthflow:   signaldepthflow.NewSignal(ctx, bus),
		exhaust:     signalexhaust.NewSignal(ctx),
		hawkes:      signalhawkes.NewSignal(ctx),
		leadlag:     signalleadlag.NewSignal(ctx, bus),
		liquidity:   signalliquidity.NewSignal(ctx),
		pumpdump:    signalpumpdump.NewSignal(ctx, bus),
		sentiment:   signalsentiment.NewSignal(ctx, bus),
		toxicity:    signaltoxicity.NewSignal(ctx, bus),
	}

	runner.subscribeTickers()
	runner.subscribeTrades()
	runner.subscribeLevel3()
	runner.subscribeFutures()

	return runner
}

func (runner *Runner) subscribeTickers() {
	tickersChannel := runtime.ChannelOf[kraken.TickerData](
		runner.bus, types.ChannelTickers,
		func(ticker kraken.TickerData) string { return ticker.Symbol },
	)

	tickersChannel.Subscribe("signal-liquidity", func(ticker kraken.TickerData) error {
		start := time.Now()
		res := runner.liquidity.Step(ticker)
		if runner.ObserveModule != nil {
			runner.ObserveModule("liquidity", time.Since(start))
		}
		runner.publish(res)
		return nil
	})

	tickersChannel.Subscribe("signal-correlation", func(ticker kraken.TickerData) error {
		start := time.Now()
		res := runner.correlation.Step(ticker)
		if runner.ObserveModule != nil {
			runner.ObserveModule("correlation", time.Since(start))
		}
		runner.publish(res)
		return nil
	})

	tickersChannel.Subscribe("signal-exhaust", func(ticker kraken.TickerData) error {
		start := time.Now()
		res := runner.exhaust.Step(ticker)
		if runner.ObserveModule != nil {
			runner.ObserveModule("exhaustion", time.Since(start))
		}
		runner.publish(res)
		return nil
	})

	tickersChannel.Subscribe("signal-leadlag", func(ticker kraken.TickerData) error {
		start := time.Now()
		res := runner.leadlag.Step(ticker)
		if runner.ObserveModule != nil {
			runner.ObserveModule("leadlag", time.Since(start))
		}
		runner.publish(res)
		return nil
	})

	tickersChannel.Subscribe("signal-pumpdump-ticker", func(ticker kraken.TickerData) error {
		start := time.Now()
		res := runner.pumpdump.StepTicker(ticker)
		if runner.ObserveModule != nil {
			runner.ObserveModule("pumpdump", time.Since(start))
		}
		runner.publish(res)
		return nil
	})

	tickersChannel.Subscribe("signal-sentiment", func(ticker kraken.TickerData) error {
		start := time.Now()
		res := runner.sentiment.Step(ticker)
		if runner.ObserveModule != nil {
			runner.ObserveModule("sentiment", time.Since(start))
		}
		runner.publish(res)
		return nil
	})
}

func (runner *Runner) subscribeTrades() {
	tradesChannel := runtime.ChannelOf[kraken.TradeData](
		runner.bus, types.ChannelTrades,
		func(trade kraken.TradeData) string { return trade.Symbol },
	)

	tradesChannel.Subscribe("signal-cvd", func(trade kraken.TradeData) error {
		start := time.Now()
		res := runner.cvd.Step(trade)
		if runner.ObserveModule != nil {
			runner.ObserveModule("cvd", time.Since(start))
		}
		runner.publish(res)
		return nil
	})

	tradesChannel.Subscribe("signal-hawkes", func(trade kraken.TradeData) error {
		start := time.Now()
		res := runner.hawkes.Step(trade)
		if runner.ObserveModule != nil {
			runner.ObserveModule("hawkes", time.Since(start))
		}
		runner.publish(res)
		return nil
	})

	tradesChannel.Subscribe("signal-pumpdump-trade", func(trade kraken.TradeData) error {
		start := time.Now()
		res := runner.pumpdump.StepTrade(trade)
		if runner.ObserveModule != nil {
			runner.ObserveModule("pumpdump", time.Since(start))
		}
		runner.publish(res)
		return nil
	})

	tradesChannel.Subscribe("signal-toxicity-trade", func(trade kraken.TradeData) error {
		if runner.bus != nil {
			if _, found := runner.bus.Shared("book", trade.Symbol); found {
				start := time.Now()
				res := runner.toxicity.StepTrade(trade)
				if runner.ObserveModule != nil {
					runner.ObserveModule("toxicity", time.Since(start))
				}
				runner.publish(res)
			}
		}
		return nil
	})
}

func (runner *Runner) subscribeLevel3() {
	level3Channel := runtime.ChannelOf[kraken.Level3Data](
		runner.bus, types.ChannelLevel3,
		func(frame kraken.Level3Data) string { return frame.Symbol },
	)

	level3Channel.Subscribe("signal-depthflow-level3", func(frame kraken.Level3Data) error {
		if runner.bus != nil {
			if _, found := runner.bus.Shared("book", frame.Symbol); found {
				start := time.Now()
				res := runner.depthflow.Step(frame.Symbol, frame.Timestamp)
				if runner.ObserveModule != nil {
					runner.ObserveModule("depthflow", time.Since(start))
				}
				runner.publish(res)
			}
		}
		return nil
	})

	level3Channel.Subscribe("signal-pumpdump-level3", func(frame kraken.Level3Data) error {
		if runner.bus != nil {
			if _, found := runner.bus.Shared("book", frame.Symbol); found {
				start := time.Now()
				res := runner.pumpdump.StepLevel3(frame.Symbol, frame.Timestamp)
				if runner.ObserveModule != nil {
					runner.ObserveModule("pumpdump", time.Since(start))
				}
				runner.publish(res)
			}
		}
		return nil
	})

	level3Channel.Subscribe("signal-toxicity-level3", func(frame kraken.Level3Data) error {
		if runner.bus != nil {
			if _, found := runner.bus.Shared("book", frame.Symbol); found {
				start := time.Now()
				res := runner.toxicity.Step(frame.Symbol, frame.Timestamp)
				if runner.ObserveModule != nil {
					runner.ObserveModule("toxicity", time.Since(start))
				}
				runner.publish(res)
			}
		}
		return nil
	})
}

func (runner *Runner) subscribeFutures() {
	futuresTickersChannel := runtime.ChannelOf[kraken.FuturesTickerData](
		runner.bus, types.ChannelFuturesTickers,
		func(ticker kraken.FuturesTickerData) string { return ticker.Symbol },
	)

	futuresTickersChannel.Subscribe("signal-derivatives-ticker", func(ticker kraken.FuturesTickerData) error {
		start := time.Now()
		res := runner.derivatives.StepTicker(ticker)
		if runner.ObserveModule != nil {
			runner.ObserveModule("derivatives", time.Since(start))
		}
		runner.publish(res)
		return nil
	})

	futuresTradesChannel := runtime.ChannelOf[kraken.FuturesTradeData](
		runner.bus, types.ChannelFuturesTrades,
		func(trade kraken.FuturesTradeData) string { return trade.Symbol },
	)

	futuresTradesChannel.Subscribe("signal-derivatives-trade", func(trade kraken.FuturesTradeData) error {
		start := time.Now()
		res := runner.derivatives.StepTrade(trade)
		if runner.ObserveModule != nil {
			runner.ObserveModule("derivatives", time.Since(start))
		}
		runner.publish(res)
		return nil
	})
}

func (runner *Runner) publish(dataMeasurement *data.Measurement[float64]) {
	if runner == nil || dataMeasurement == nil || dataMeasurement.Err != nil || len(dataMeasurement.Metrics) == 0 {
		return
	}

	measurement := dataMeasurement.ToTypesMeasurement()

	if measurement == nil {
		return
	}

	runner.measurements.Publish(measurement)
}

/*
Close tears down all running signal instruments and subscriptions.
*/
func (runner *Runner) Close() error {
	if runner == nil {
		return nil
	}

	if runner.cancel != nil {
		runner.cancel()
	}

	_ = runner.correlation.Close()
	_ = runner.cvd.Close()
	_ = runner.derivatives.Close()
	_ = runner.depthflow.Close()
	_ = runner.exhaust.Close()
	_ = runner.hawkes.Close()
	_ = runner.leadlag.Close()
	_ = runner.liquidity.Close()
	_ = runner.pumpdump.Close()
	_ = runner.sentiment.Close()
	_ = runner.toxicity.Close()

	return nil
}
