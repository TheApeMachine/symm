package hawkes

import (
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Estimator series prefixes for the arrival-dynamics velocity metrics.
*/
const (
	prefixLambda  = "hawkes/lambda"
	prefixSpectral = "hawkes/spectral"
)

/*
Derived metric slots the arrival-dynamics pipeline composes on top of the
Hawkes algorithm output.
*/
var (
	symbolBuyFraction            = nmtypes.MustIntern("hawkes/event_fraction_buy")
	symbolSellFraction           = nmtypes.MustIntern("hawkes/event_fraction_sell")
	symbolArrivalRate            = nmtypes.MustIntern("hawkes/arrival_rate")
	symbolConditionalIntensity   = nmtypes.MustIntern("hawkes/conditional_intensity")
	symbolBackgroundRate         = nmtypes.MustIntern("hawkes/background_rate")
	symbolExcitationBuy          = nmtypes.MustIntern("hawkes/excitation_intensity_buy")
	symbolExcitationSell         = nmtypes.MustIntern("hawkes/excitation_intensity_sell")
	symbolExcitationFractionBuy  = nmtypes.MustIntern("hawkes/excitation_fraction_buy")
	symbolExcitationFractionSell = nmtypes.MustIntern("hawkes/excitation_fraction_sell")

	symbolLLHawkesTotal        = nmtypes.MustIntern("hawkes/state/ll_hawkes_total")
	symbolLLPoissonTotal       = nmtypes.MustIntern("hawkes/state/ll_poisson_total")
	symbolLLSelfTotal          = nmtypes.MustIntern("hawkes/state/ll_self_total")
	symbolDeltaPoissonTotal    = nmtypes.MustIntern("hawkes/state/ll_delta_poisson_total")
	symbolDeltaSelfTotal       = nmtypes.MustIntern("hawkes/state/ll_delta_self_total")
	symbolLLPerEventHawkes     = nmtypes.MustIntern("hawkes/log_likelihood_per_event_hawkes")
	symbolLLPerEventPoisson    = nmtypes.MustIntern("hawkes/log_likelihood_gain_per_event_poisson")
	symbolLLPerEventSelf       = nmtypes.MustIntern("hawkes/log_likelihood_gain_per_event_self")
	symbolLogConditional       = nmtypes.MustIntern("hawkes/log_conditional_intensity")
	symbolLambdaVelocity       = nmtypes.MustIntern("hawkes/conditional_intensity_velocity")
	symbolSpectralVelocity     = nmtypes.MustIntern("hawkes/spectral_radius_velocity")
)

/*
prefixed resolves one namespaced estimator slot for a series prefix.
*/
func prefixed(prefix string, name string) nmtypes.Symbol {
	return nmtypes.MustIntern(temporal.JoinPrefix(prefix, name))
}

/*
velocityEstimator composes one causal first-difference velocity over a series.
*/
func velocityEstimator(prefix string, quantity nmtypes.Symbol) nmtypes.Primitive {
	series := temporal.NewSeries(prefix)

	return nmtypes.Pipe(
		nmtypes.Relay(quantity, series.ValueSymbol),
		nmtypes.Relay(nmtypes.EventTimeSec, series.SecSymbol),
		nmtypes.Relay(nmtypes.EventTimeNsec, series.NsecSymbol),
		statistic.Velocity(prefix),
	)
}

/*
velocityRate projects a series' first difference into its per-second rate,
gated on the differencer having produced a delta.
*/
func velocityRate(prefix string, target nmtypes.Symbol) nmtypes.Primitive {
	delta := prefixed(prefix, "velocity/delta")
	elapsed := prefixed(prefix, "velocity/elapsed_sec")
	ready := prefixed(prefix, "ready")

	return logic.If(
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(ready, nmtypes.PortX),
			nmtypes.Out(nmtypes.PortX, logic.SymbolCondition),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(delta, calculus.PortA),
			nmtypes.In(elapsed, calculus.PortB),
			nmtypes.Out(calculus.PortResult, target),
		),
		nmtypes.Identity,
	)
)

/*
hawkesPipeline composes the Hawkes point process and its derived arrival
statistics. The Hawkes math lives in nomagique/algo; this pipeline only
projects the algorithm's facts and composes scale-free mark and intensity
decompositions.
*/
func hawkesPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		algo.Hawkes(),

		// Effective model support is the retained event count.
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(statistic.SymbolEventCount, nmtypes.PortX),
			nmtypes.Out(nmtypes.PortX, nmtypes.SampleCount),
		),

		// Mark composition.
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(statistic.SymbolAlphaEventCount, calculus.PortA),
			nmtypes.In(statistic.SymbolEventCount, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolBuyFraction),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(statistic.SymbolBetaEventCount, calculus.PortA),
			nmtypes.In(statistic.SymbolEventCount, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolSellFraction),
		),

		// Total arrival and intensity decompositions.
		nmtypes.Wire(
			calculus.Sum,
			nmtypes.In(statistic.SymbolAlphaArrivalRate, calculus.PortA),
			nmtypes.In(statistic.SymbolBetaArrivalRate, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolArrivalRate),
		),
		nmtypes.Wire(
			calculus.Sum,
			nmtypes.In(statistic.SymbolLambdaAlpha, calculus.PortA),
			nmtypes.In(statistic.SymbolLambdaBeta, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolConditionalIntensity),
		),
		nmtypes.Wire(
			calculus.Sum,
			nmtypes.In(statistic.SymbolMuAlpha, calculus.PortA),
			nmtypes.In(statistic.SymbolMuBeta, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolBackgroundRate),
		),

		// Excess intensity above the fitted background rate.
		nmtypes.Wire(
			calculus.Difference,
			nmtypes.In(statistic.SymbolLambdaAlpha, calculus.PortA),
			nmtypes.In(statistic.SymbolMuAlpha, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolExcitationBuy),
		),
		nmtypes.Wire(
			calculus.Difference,
			nmtypes.In(statistic.SymbolLambdaBeta, calculus.PortA),
			nmtypes.In(statistic.SymbolMuBeta, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolExcitationSell),
		),

		// Fraction of current intensity attributable to prior-event excitation.
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(symbolExcitationBuy, calculus.PortA),
			nmtypes.In(statistic.SymbolLambdaAlpha, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolExcitationFractionBuy),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(symbolExcitationSell, calculus.PortA),
			nmtypes.In(statistic.SymbolLambdaBeta, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolExcitationFractionSell),
		),
	)
}

/*
Trade is the arrival-dynamics market entity. It owns exactly a Number pipeline
and a projector, both declared in its constructor, plus Step and Close.
*/
type Trade struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTrade constructs the Trade entity: one Number pipeline for the Hawkes
process and one projector that names the output slots.
*/
func NewTrade() *Trade {
	return &Trade{
		number: nomagique.NewNumber[string](hawkesPipeline()),
		projector: data.NewProjector(
			data.Binding{From: statistic.SymbolEventCount, Name: "event_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolAlphaEventCount, Name: "event_count:buy", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolBetaEventCount, Name: "event_count:sell", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBuyFraction, Name: "event_fraction:buy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSellFraction, Name: "event_fraction:sell", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolArrivalRate, Name: "arrival_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolAlphaArrivalRate, Name: "arrival_rate:buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolBetaArrivalRate, Name: "arrival_rate:sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolConditionalIntensity, Name: "conditional_intensity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolLambdaAlpha, Name: "conditional_intensity:buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolLambdaBeta, Name: "conditional_intensity:sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolBackgroundRate, Name: "background_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolMuAlpha, Name: "background_rate:buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolMuBeta, Name: "background_rate:sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolExcitationBuy, Name: "excitation_intensity:buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolExcitationSell, Name: "excitation_intensity:sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolExcitationFractionBuy, Name: "excitation_fraction:buy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolExcitationFractionSell, Name: "excitation_fraction:sell", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolAlphaAA, Name: "excitation_amplitude:buy_from_buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolAlphaAB, Name: "excitation_amplitude:buy_from_sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolAlphaBA, Name: "excitation_amplitude:sell_from_buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolAlphaBB, Name: "excitation_amplitude:sell_from_sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolBeta, Name: "excitation_decay:buy_from_buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolBeta, Name: "excitation_decay:buy_from_sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolBeta, Name: "excitation_decay:sell_from_buy", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolBeta, Name: "excitation_decay:sell_from_sell", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolFold, Name: "excitation_timescale:buy_from_buy", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolFold, Name: "excitation_timescale:buy_from_sell", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolFold, Name: "excitation_timescale:sell_from_buy", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolFold, Name: "excitation_timescale:sell_from_sell", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolOffspringAA, Name: "offspring:buy_from_buy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolOffspringAB, Name: "offspring:buy_from_sell", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolOffspringBA, Name: "offspring:sell_from_buy", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolOffspringBB, Name: "offspring:sell_from_sell", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolSpectralRadius, Name: "branching_spectral_radius", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolDescendantsAlpha, Name: "expected_descendants_from_buy", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolDescendantsBeta, Name: "expected_descendants_from_sell", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolLLHawkes, Name: "log_likelihood:hawkes", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolLLPoisson, Name: "log_likelihood:poisson", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolLLSelf, Name: "log_likelihood:self_only", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolDeltaPoisson, Name: "log_likelihood_gain_vs_poisson", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolDeltaSelf, Name: "log_likelihood_gain_vs_self_only", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
Step receives one trade, loads its mark and event time, runs the Number
pipeline, and projects exactly one Measurement.
*/
func (trade *Trade) Step(observation kraken.TradeData) *data.Measurement[float64] {
	input := nmtypes.Frame{}
	input.Put(statistic.SymbolMark, markForSide(observation.Side))
	input.Put(statistic.SymbolUnixSec, float64(observation.Timestamp.Unix()))
	input.Put(statistic.SymbolUnixNsec, float64(observation.Timestamp.Nanosecond()))

	output := trade.number.Step(observation.Symbol, input)

	from := observation.Timestamp

	if seconds, found := output.Get(statistic.SymbolObservedFromSec); found {
		if nanoseconds, foundNanoseconds := output.Get(statistic.SymbolObservedFromNsec); foundNanoseconds {
			from = time.Unix(int64(seconds), int64(nanoseconds))
		}
	}

	return trade.projector.Project(observation.Symbol, "hawkes", observation.Timestamp, from, output)
}

func (trade *Trade) Close() error { return nil }

/*
markForSide encodes one trade's aggressor side into the process mark: buys are
the positive (alpha) channel, every other side the negative (beta) channel.
*/
func markForSide(side string) float64 {
	if side == "buy" {
		return 1
	}

	return -1
}
