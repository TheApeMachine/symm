package liquidity

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Input/output slot symbols for the touch metric pipeline.
*/
var (
	symbolBidPrice         = nmtypes.MustIntern("liquidity/bid_price")
	symbolAskPrice         = nmtypes.MustIntern("liquidity/ask_price")
	symbolBidQty           = nmtypes.MustIntern("liquidity/bid_qty")
	symbolAskQty           = nmtypes.MustIntern("liquidity/ask_qty")
	symbolMidpoint         = nmtypes.MustIntern("liquidity/midpoint")
	symbolSpread           = nmtypes.MustIntern("liquidity/spread")
	symbolRelativeSpread   = nmtypes.MustIntern("liquidity/relative_spread")
	symbolBidNotional      = nmtypes.MustIntern("liquidity/touch_notional_bid")
	symbolAskNotional      = nmtypes.MustIntern("liquidity/touch_notional_ask")
	symbolTwoSidedNotional = nmtypes.MustIntern("liquidity/two_sided_notional")
	symbolSumNotional      = nmtypes.MustIntern("liquidity/sum_notional")
	symbolDiffNotional     = nmtypes.MustIntern("liquidity/diff_notional")
	symbolImbalance        = nmtypes.MustIntern("liquidity/touch_notional_imbalance")

	symbolLogBidNotional = nmtypes.MustIntern("liquidity/log_touch_notional_bid")
	symbolLogAskNotional = nmtypes.MustIntern("liquidity/log_touch_notional_ask")
	symbolLogRelSpread   = nmtypes.MustIntern("liquidity/log_relative_spread")

	// Observed baseline / ratio / divergence projection slots.
	symbolBidBaseline  = nmtypes.MustIntern("liquidity/obs/touch_notional_baseline_bid")
	symbolBidRatio     = nmtypes.MustIntern("liquidity/obs/depth_ratio_bid")
	symbolAskBaseline  = nmtypes.MustIntern("liquidity/obs/touch_notional_baseline_ask")
	symbolAskRatio     = nmtypes.MustIntern("liquidity/obs/depth_ratio_ask")
	symbolSpreadBase   = nmtypes.MustIntern("liquidity/obs/relative_spread_baseline")
	symbolSpreadRatio  = nmtypes.MustIntern("liquidity/obs/spread_ratio")

	// Divergence-velocity regression slots (one causal local regression per
	// divergence path, sharing the joint estimator's derived horizon).
	symbolBidVelocity     = nmtypes.MustIntern(temporal.JoinPrefix("liquidity/vel_bid", "slope/beta"))
	symbolAskVelocity     = nmtypes.MustIntern(temporal.JoinPrefix("liquidity/vel_ask", "slope/beta"))
	symbolSpreadVelocity  = nmtypes.MustIntern(temporal.JoinPrefix("liquidity/vel_spread", "slope/beta"))
)

/*
jointEstimator is the single coherent causal estimator over the three log-space
dimensions. It drives baseline/ratio/divergence/noise/zscore per dimension,
the joint SNR and the effective support from one event-time weighting.
*/
var jointEstimator = statistic.NewJointDecayedEstimator("liquidity/joint", 3)

/*
Ticker is the touch-snapshot market entity.
*/
type Ticker struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTicker constructs the Ticker entity.
*/
func NewTicker() *Ticker {
	inValues := []nmtypes.Symbol{symbolLogBidNotional, symbolLogAskNotional, symbolLogRelSpread}

	return &Ticker{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			logic.PositiveOrder(symbolBidPrice, symbolAskPrice),

			nmtypes.Wire(calculus.Product, nmtypes.In(symbolBidPrice, calculus.PortA), nmtypes.In(symbolBidQty, calculus.PortB), nmtypes.Out(calculus.PortResult, symbolBidNotional)),
			nmtypes.Wire(calculus.Product, nmtypes.In(symbolAskPrice, calculus.PortA), nmtypes.In(symbolAskQty, calculus.PortB), nmtypes.Out(calculus.PortResult, symbolAskNotional)),
			nmtypes.Wire(calculus.Average, nmtypes.In(symbolBidPrice, calculus.PortA), nmtypes.In(symbolAskPrice, calculus.PortB), nmtypes.Out(calculus.PortResult, symbolMidpoint)),
			nmtypes.Wire(calculus.Difference, nmtypes.In(symbolAskPrice, calculus.PortA), nmtypes.In(symbolBidPrice, calculus.PortB), nmtypes.Out(calculus.PortResult, symbolSpread)),
			nmtypes.Wire(calculus.Quotient, nmtypes.In(symbolSpread, calculus.PortA), nmtypes.In(symbolMidpoint, calculus.PortB), nmtypes.Out(calculus.PortResult, symbolRelativeSpread)),
			nmtypes.Wire(calculus.Minimum, nmtypes.In(symbolBidNotional, calculus.PortA), nmtypes.In(symbolAskNotional, calculus.PortB), nmtypes.Out(calculus.PortResult, symbolTwoSidedNotional)),
			nmtypes.Wire(calculus.Sum, nmtypes.In(symbolBidNotional, calculus.PortA), nmtypes.In(symbolAskNotional, calculus.PortB), nmtypes.Out(calculus.PortResult, symbolSumNotional)),
			nmtypes.Wire(calculus.Difference, nmtypes.In(symbolBidNotional, calculus.PortA), nmtypes.In(symbolAskNotional, calculus.PortB), nmtypes.Out(calculus.PortResult, symbolDiffNotional)),
			nmtypes.Wire(calculus.Quotient, nmtypes.In(symbolDiffNotional, calculus.PortA), nmtypes.In(symbolSumNotional, calculus.PortB), nmtypes.Out(calculus.PortResult, symbolImbalance)),

			nmtypes.Wire(calculus.Log, nmtypes.In(symbolBidNotional, calculus.PortX), nmtypes.Out(calculus.PortResult, symbolLogBidNotional)),
			nmtypes.Wire(calculus.Log, nmtypes.In(symbolAskNotional, calculus.PortX), nmtypes.Out(calculus.PortResult, symbolLogAskNotional)),
			nmtypes.Wire(calculus.Log, nmtypes.In(symbolRelativeSpread, calculus.PortX), nmtypes.Out(calculus.PortResult, symbolLogRelSpread)),

			// The one coherent estimator drives every historical fact.
			jointEstimator.Primitive(inValues, nil),

			// Derive baseline and ratio from the emitted residuals (divergence)
			// and the current log values: baseline = D_t * exp(-divergence),
			// ratio = exp(divergence). log(ratio) == divergence holds exactly.
			baselineRatioWiring(),

			// Project estimator quality to the measurement: N_eff -> support
			// (so Finalize yields Maturity = 1 - 1/N_eff), and the joint SNR
			// only when defined — undefined SNR stays absent, never 0.
			qualityWiring(),

			// Divergence velocity: causal local-time regression over each
			// divergence path, restricted to the derived horizon. Gated on the
			// residual being produced (a causal baseline exists); before that
			// the divergence and its velocity are undefined.
			logic.If(
				residualPredicate(jointEstimator.Residual(0)),
				divergenceVelocityWiring(),
				nil,
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolBidPrice, Name: "best_bid_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskPrice, Name: "best_ask_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidQty, Name: "touch_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskQty, Name: "touch_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidNotional, Name: "touch_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskNotional, Name: "touch_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpoint, Name: "midpoint", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpread, Name: "spread", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeSpread, Name: "relative_spread", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTwoSidedNotional, Name: "two_sided_touch_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolImbalance, Name: "touch_notional_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: symbolBidBaseline, Name: "touch_notional_baseline:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidRatio, Name: "depth_ratio:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: jointEstimator.Residual(0), Name: "depth_divergence:bid", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: jointEstimator.Noise(0), Name: "depth_noise_scale:bid", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: jointEstimator.ZScore(0), Name: "depth_zscore:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: symbolAskBaseline, Name: "touch_notional_baseline:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskRatio, Name: "depth_ratio:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: jointEstimator.Residual(1), Name: "depth_divergence:ask", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: jointEstimator.Noise(1), Name: "depth_noise_scale:ask", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: jointEstimator.ZScore(1), Name: "depth_zscore:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: symbolSpreadBase, Name: "relative_spread_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpreadRatio, Name: "spread_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: jointEstimator.Residual(2), Name: "spread_divergence", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: jointEstimator.Noise(2), Name: "spread_noise_scale", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: jointEstimator.ZScore(2), Name: "spread_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: symbolBidVelocity, Name: "divergence_velocity:bid", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolAskVelocity, Name: "divergence_velocity:ask", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolSpreadVelocity, Name: "spread_divergence_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
		),
	}
}

/*
baselineRatioWiring derives the projected baseline and ratio from the emitted
residuals. For dimension j: baseline_j = exp(log(X_j) - divergence_j) and
ratio_j = exp(divergence_j), so log(ratio_j) == divergence_j exactly.
*/
func baselineRatioWiring() nmtypes.Primitive {
	return func(frame *nmtypes.Frame) {
		if divergence, found := frame.Get(jointEstimator.Residual(0)); found {
			if logBid, found := frame.Get(symbolLogBidNotional); found {
				frame.Put(symbolBidBaseline, math.Exp(logBid-divergence))
				frame.Put(symbolBidRatio, math.Exp(divergence))
			}
		}

		if divergence, found := frame.Get(jointEstimator.Residual(1)); found {
			if logAsk, found := frame.Get(symbolLogAskNotional); found {
				frame.Put(symbolAskBaseline, math.Exp(logAsk-divergence))
				frame.Put(symbolAskRatio, math.Exp(divergence))
			}
		}

		if divergence, found := frame.Get(jointEstimator.Residual(2)); found {
			if logSpread, found := frame.Get(symbolLogRelSpread); found {
				frame.Put(symbolSpreadBase, math.Exp(logSpread-divergence))
				frame.Put(symbolSpreadRatio, math.Exp(divergence))
			}
		}
	}
}

/*
qualityWiring projects the single estimator's effective support and joint SNR
onto the measurement-quality slots data.Measurement.Finalize consumes:

	N_eff -> support      (Maturity = 1 - 1/N_eff, 0 when N_eff <= 1)
	snr   -> mahalanobis/snr   only when ready (defined), else absent

An undefined joint SNR is never mapped to a numeric zero: the slot simply stays
absent, so Finalize leaves SNRDefined false.
*/
func qualityWiring() nmtypes.Primitive {
	return func(frame *nmtypes.Frame) {
		if neff, found := frame.Get(jointEstimator.Neff()); found {
			frame.Put(nmtypes.SampleCount, neff)
		}

		if ready, found := frame.Get(jointEstimator.JointReady()); found && ready != 0 {
			if snr, found := frame.Get(jointEstimator.SNR()); found {
				frame.Put(nmtypes.MustIntern("mahalanobis/snr"), snr)
			}
		}
	}
}

/*
residualPredicate emits a logic condition that is true when the given residual
slot is present (its estimator has a causal baseline) and false otherwise.
Absence is a false condition, never a wire error: on the first observation the
residual is legitimately undefined.
*/
func residualPredicate(slot nmtypes.Symbol) nmtypes.Primitive {
	return func(frame *nmtypes.Frame) {
		condition := 0.0

		if _, found := frame.Get(slot); found {
			condition = 1.0
		}

		frame.Put(logic.SymbolCondition, condition)
	}
}

/*
divergenceVelocityWiring routes each divergence residual into its own event-time
path and runs a causal local-time regression restricted to the joint estimator's
derived horizon. temporal.Path appends AFTER the fit, so the current divergence
never participates in its own slope.
*/
func divergenceVelocityWiring() nmtypes.Primitive {
	// One path + regression per divergence dimension, all sharing the joint
	// estimator's derived horizon as the regression window.
	horizon := jointEstimator.Horizon()

	bidSeries := temporal.NewSeries("liquidity/vel_bid")
	askSeries := temporal.NewSeries("liquidity/vel_ask")
	spreadSeries := temporal.NewSeries("liquidity/vel_spread")

	return nmtypes.Pipe(
		// Feed each residual into its path's value+time slots.
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(jointEstimator.Residual(0), bidSeries.ValueSymbol), nmtypes.Out(bidSeries.ValueSymbol, bidSeries.ValueSymbol)),
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(nmtypes.EventTimeSec, bidSeries.SecSymbol), nmtypes.Out(bidSeries.SecSymbol, bidSeries.SecSymbol)),
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(nmtypes.EventTimeNsec, bidSeries.NsecSymbol), nmtypes.Out(bidSeries.NsecSymbol, bidSeries.NsecSymbol)),
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(jointEstimator.Residual(1), askSeries.ValueSymbol), nmtypes.Out(askSeries.ValueSymbol, askSeries.ValueSymbol)),
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(nmtypes.EventTimeSec, askSeries.SecSymbol), nmtypes.Out(askSeries.SecSymbol, askSeries.SecSymbol)),
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(nmtypes.EventTimeNsec, askSeries.NsecSymbol), nmtypes.Out(askSeries.NsecSymbol, askSeries.NsecSymbol)),
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(jointEstimator.Residual(2), spreadSeries.ValueSymbol), nmtypes.Out(spreadSeries.ValueSymbol, spreadSeries.ValueSymbol)),
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(nmtypes.EventTimeSec, spreadSeries.SecSymbol), nmtypes.Out(spreadSeries.SecSymbol, spreadSeries.SecSymbol)),
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(nmtypes.EventTimeNsec, spreadSeries.NsecSymbol), nmtypes.Out(spreadSeries.NsecSymbol, spreadSeries.NsecSymbol)),

		// Horizon fact (shared derived cadence) made available to the fit.
		nmtypes.Wire(nmtypes.Identity, nmtypes.In(horizon, statistic.SymbolSlopeHorizon), nmtypes.Out(statistic.SymbolSlopeHorizon, statistic.SymbolSlopeHorizon)),

		statistic.LocalRegression("liquidity/vel_bid"),
		statistic.LocalRegression("liquidity/vel_ask"),
		statistic.LocalRegression("liquidity/vel_spread"),
		temporal.Path("liquidity/vel_bid"),
		temporal.Path("liquidity/vel_ask"),
		temporal.Path("liquidity/vel_spread"),
	)
}

/*
Step receives one market data point, loads the touch facts, runs the Number
pipeline, and projects exactly one Measurement.
*/
func (ticker *Ticker) Step(trade kraken.TickerData) *data.Measurement[float64] {
	if trade.Bid == nil || trade.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: ticker requires bid and ask")}
	}

	input := nmtypes.Frame{}
	input.Put(symbolBidPrice, trade.Bid.Float64())
	input.Put(symbolAskPrice, trade.Ask.Float64())
	input.Put(symbolBidQty, trade.BidQty)
	input.Put(symbolAskQty, trade.AskQty)
	input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

	return ticker.projector.Project(
		trade.Symbol,
		"liquidity",
		trade.Timestamp,
		trade.Timestamp,
		ticker.number.Step(trade.Symbol, input),
	)
}

func (ticker *Ticker) Close() error { return nil }
