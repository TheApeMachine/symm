package derivatives

import (
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
Input/output slot symbols for the derivatives ticker metric pipeline.
*/
var (
	symbolDerivativePrice      = nmtypes.MustIntern("derivatives/derivative_price")
	symbolReferencePrice       = nmtypes.MustIntern("derivatives/reference_price")
	symbolSpotPrice            = nmtypes.MustIntern("derivatives/spot_price")
	symbolOpenInterest         = nmtypes.MustIntern("derivatives/open_interest")
	symbolBasisNumerator       = nmtypes.MustIntern("derivatives/basis_numerator")
	symbolBasis                = nmtypes.MustIntern("derivatives/basis")
	symbolLogBasis             = nmtypes.MustIntern("derivatives/log_basis")
	symbolLogDerivative        = nmtypes.MustIntern("derivatives/log_derivative")
	symbolLogIndex             = nmtypes.MustIntern("derivatives/log_index")
	symbolLogSpot              = nmtypes.MustIntern("derivatives/log_spot")
	symbolDerivativeIndexBasis = nmtypes.MustIntern("derivatives/derivative_index_log_basis")
	symbolIndexSpotBasis       = nmtypes.MustIntern("derivatives/index_spot_log_basis")
	symbolDerivativeSpotBasis  = nmtypes.MustIntern("derivatives/derivative_spot_log_basis")
	symbolBasisClosureError    = nmtypes.MustIntern("derivatives/basis_closure_error")
	symbolOIChange             = nmtypes.MustIntern("derivatives/oi_change")
	symbolOILogChange          = nmtypes.MustIntern("derivatives/oi_log_change")
	symbolOIElapsed            = nmtypes.MustIntern("derivatives/oi_elapsed")
	symbolOIGrowthRate         = nmtypes.MustIntern("derivatives/oi_growth_rate")
	symbolBasisRate            = nmtypes.MustIntern("derivatives/basis_rate")
	symbolDerivativeLogReturn  = nmtypes.MustIntern("derivatives/derivative_log_return")
	symbolReferenceLogReturn   = nmtypes.MustIntern("derivatives/reference_log_return")
	symbolReturnGap            = nmtypes.MustIntern("derivatives/return_gap")
	symbolDivergence           = nmtypes.MustIntern("divergence")
	symbolNoiseVariance        = nmtypes.MustIntern("noise_variance")
	symbolZero                 = nmtypes.MustIntern("derivatives/zero")
)

/*
Ticker is the derivative/reference state market entity. It owns exactly a
Number pipeline and a projector, both declared in its constructor, plus Step
and Close.
*/
type Ticker struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTicker constructs the Ticker entity: one Number pipeline for the derivative
basis, open-interest, and aligned-return dynamics, and one projector that names
the output slots.
*/
func NewTicker() *Ticker {
	return &Ticker{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			nmtypes.Assign(symbolZero, 0),
			// Basis: (derivative - reference) / reference
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolDerivativePrice, calculus.PortA),
				nmtypes.In(symbolReferencePrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBasisNumerator),
			),
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolBasisNumerator, calculus.PortA),
				nmtypes.In(symbolReferencePrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBasis),
			),
			// Log basis: log(derivative / reference). Both prices must be positive.
			nmtypes.Wire(
				calculus.LogRatio,
				nmtypes.In(symbolDerivativePrice, calculus.SymbolCurrent),
				nmtypes.In(symbolReferencePrice, calculus.SymbolPrevious),
				nmtypes.Out(calculus.PortResult, symbolLogBasis),
			),
			// Three-price log basis geometry (derivative, index, spot). Each leg
			// is a log ratio composed from per-leg natural logs.
			nmtypes.Wire(
				calculus.Log,
				nmtypes.In(symbolDerivativePrice, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolLogDerivative),
			),
			nmtypes.Wire(
				calculus.Log,
				nmtypes.In(symbolReferencePrice, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolLogIndex),
			),
			nmtypes.Wire(
				calculus.Log,
				nmtypes.In(symbolSpotPrice, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolLogSpot),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolLogDerivative, calculus.PortA),
				nmtypes.In(symbolLogIndex, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolDerivativeIndexBasis),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolLogIndex, calculus.PortA),
				nmtypes.In(symbolLogSpot, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolIndexSpotBasis),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolLogDerivative, calculus.PortA),
				nmtypes.In(symbolLogSpot, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolDerivativeSpotBasis),
			),
			// basis_closure_error = derivative_spot - derivative_index - index_spot
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolDerivativeSpotBasis, calculus.PortA),
				nmtypes.In(symbolDerivativeIndexBasis, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBasisClosureError),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolBasisClosureError, calculus.PortA),
				nmtypes.In(symbolIndexSpotBasis, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBasisClosureError),
			),
			// Open-interest previous/current pair on the event clock.
			temporal.Observer("", symbolOpenInterest),
			// First observation has no previous: only point metrics are published.
			logic.If(
				readyCondition(),
				nmtypes.Pipe(
					// open_interest_change: current - previous
					nmtypes.Wire(
						calculus.Difference,
						nmtypes.In(calculus.SymbolCurrent, calculus.PortA),
						nmtypes.In(calculus.SymbolPrevious, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolOIChange),
					),
					// open_interest_log_change: log(current / previous)
					nmtypes.Wire(
						calculus.LogRatio,
						nmtypes.In(calculus.SymbolCurrent, calculus.SymbolCurrent),
						nmtypes.In(calculus.SymbolPrevious, calculus.SymbolPrevious),
						nmtypes.Out(calculus.PortResult, symbolOILogChange),
					),
					// Elapsed event time between the previous and current observation.
					nmtypes.Wire(
						temporal.Duration,
						nmtypes.In(nmtypes.EventTimeSec, temporal.SymbolCurrentSec),
						nmtypes.In(nmtypes.EventTimeNsec, temporal.SymbolCurrentNsec),
						nmtypes.In(temporal.SymbolObservedSec, temporal.SymbolPreviousSec),
						nmtypes.In(temporal.SymbolObservedNsec, temporal.SymbolPreviousNsec),
						nmtypes.Out(temporal.SymbolDelta, symbolOIElapsed),
					),
					logic.If(
						greaterThanCondition(symbolOIElapsed),
						nmtypes.Pipe(
							// open_interest_growth_rate: log change over elapsed time
							nmtypes.Wire(
								calculus.Quotient,
								nmtypes.In(symbolOILogChange, calculus.PortA),
								nmtypes.In(symbolOIElapsed, calculus.PortB),
								nmtypes.Out(calculus.PortResult, symbolOIGrowthRate),
							),
							// open_interest_growth_velocity: first difference of the rate.
							velocityOver("growth_velocity", symbolOIGrowthRate),
							// Route the growth rate into the shared value slot and
							// maintain its causal baseline, dispersion, and z-score.
							route(symbolOIGrowthRate, nmtypes.SampleValue),
							statistic.ZScore(""),
							// Carry the departure and noise power when the z-score is
							// estimable so Finalize derives the scalar SNR.
							logic.If(
								readyCondition(),
								nmtypes.Pipe(
									route(statistic.SymbolResidual, symbolDivergence),
									nmtypes.Wire(
										calculus.Product,
										nmtypes.In(statistic.SymbolDispersion, calculus.PortA),
										nmtypes.In(statistic.SymbolDispersion, calculus.PortB),
										nmtypes.Out(calculus.PortResult, symbolNoiseVariance),
									),
								),
								nmtypes.Identity,
							),
							statistic.Baseline(""),
							temporal.Window(""),
						),
						nmtypes.Identity,
					),
				),
				nmtypes.Identity,
			),
			// Basis change: first difference of basis, then its event rate and
			// the first difference of that rate.
			velocityOver("basis", symbolBasis),
			logic.If(
				seriesReadyCondition("basis"),
				logic.If(
					greaterThanCondition(prefixed("basis", "velocity/elapsed_sec")),
					nmtypes.Pipe(
						nmtypes.Wire(
							calculus.Quotient,
							nmtypes.In(prefixed("basis", "velocity/delta"), calculus.PortA),
							nmtypes.In(prefixed("basis", "velocity/elapsed_sec"), calculus.PortB),
							nmtypes.Out(calculus.PortResult, symbolBasisRate),
						),
						velocityOver("basis_rate", symbolBasisRate),
					),
					nmtypes.Identity,
				),
				nmtypes.Identity,
			),
			// Basis baseline and z-score over the basis series.
			baselineZScore("basis_baseline", symbolBasis),
			// derivative_log_return: log(current / previous) over the derivative price.
			temporal.Observer("derivative_return", symbolDerivativePrice),
			logic.If(
				readyCondition(),
				nmtypes.Wire(
					calculus.LogRatio,
					nmtypes.In(calculus.SymbolCurrent, calculus.SymbolCurrent),
					nmtypes.In(calculus.SymbolPrevious, calculus.SymbolPrevious),
					nmtypes.Out(calculus.PortResult, symbolDerivativeLogReturn),
				),
				nmtypes.Identity,
			),
			// reference_log_return: log(current / previous) over the reference price.
			temporal.Observer("reference_return", symbolReferencePrice),
			logic.If(
				readyCondition(),
				nmtypes.Wire(
					calculus.LogRatio,
					nmtypes.In(calculus.SymbolCurrent, calculus.SymbolCurrent),
					nmtypes.In(calculus.SymbolPrevious, calculus.SymbolPrevious),
					nmtypes.Out(calculus.PortResult, symbolReferenceLogReturn),
				),
				nmtypes.Identity,
			),
			// return_gap = derivative_log_return - reference_log_return, then its
			// velocity and its own causal baseline + z-score.
			logic.If(
				readyCondition(),
				nmtypes.Pipe(
					nmtypes.Wire(
						calculus.Difference,
						nmtypes.In(symbolDerivativeLogReturn, calculus.PortA),
						nmtypes.In(symbolReferenceLogReturn, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolReturnGap),
					),
					velocityOver("return_gap", symbolReturnGap),
					baselineZScore("return_gap_zscore", symbolReturnGap),
				),
				nmtypes.Identity,
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolDerivativePrice, Name: "derivative_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolReferencePrice, Name: "reference_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolOpenInterest, Name: "open_interest", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBasis, Name: "basis", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolLogBasis, Name: "log_basis", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDerivativeIndexBasis, Name: "derivative_index_log_basis", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolIndexSpotBasis, Name: "index_spot_log_basis", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDerivativeSpotBasis, Name: "derivative_spot_log_basis", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBasisClosureError, Name: "basis_closure_error", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolOIChange, Name: "open_interest_change", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolOILogChange, Name: "open_interest_log_change", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolOIGrowthRate, Name: "open_interest_growth_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: prefixed("growth_velocity", "velocity/delta"), Name: "open_interest_growth_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolBaselineValue, Name: "open_interest_growth_baseline", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: statistic.SymbolZScore, Name: "open_interest_growth_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("basis", "velocity/delta"), Name: "basis_change", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBasisRate, Name: "basis_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: prefixed("basis_rate", "velocity/delta"), Name: "basis_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: prefixed("basis_baseline", "baseline/value"), Name: "basis_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("basis_baseline", "z/value"), Name: "basis_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDerivativeLogReturn, Name: "derivative_log_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolReferenceLogReturn, Name: "reference_log_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolReturnGap, Name: "return_gap", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("return_gap", "velocity/delta"), Name: "return_gap_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: prefixed("return_gap_zscore", "z/value"), Name: "return_gap_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
Step receives one futures ticker data point, loads the derivative facts, runs
the Number pipeline, and projects exactly one Measurement. Validation happens
inside the pipeline; invalid input surfaces as a pipeline failure carried on
the Measurement's own Err field rather than a Go error return.
*/
func (ticker *Ticker) Step(point kraken.FuturesTickerData) *data.Measurement[float64] {
	input := nmtypes.Frame{}
	input.Put(symbolDerivativePrice, point.Last.Float64())
	input.Put(symbolReferencePrice, point.IndexPrice.Float64())
	input.Put(symbolSpotPrice, point.MarkPrice.Float64())
	input.Put(symbolOpenInterest, point.OpenInterest)
	input.Put(nmtypes.EventTimeSec, float64(point.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(point.Timestamp.Nanosecond()))

	return ticker.projector.Project(
		point.Symbol,
		"derivatives",
		point.Timestamp,
		point.Timestamp,
		ticker.number.Step(point.Symbol, input),
	)
}

func (ticker *Ticker) Close() error { return nil }

func readyCondition() nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(calculus.SymbolReady, logic.SymbolCondition),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}

func seriesReadyCondition(prefix string) nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(prefixed(prefix, "ready"), logic.SymbolCondition),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}

func greaterThanCondition(fact nmtypes.Symbol) nmtypes.Primitive {
	return nmtypes.Wire(
		logic.GreaterThan,
		nmtypes.In(fact, calculus.PortA),
		nmtypes.In(symbolZero, calculus.PortB),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}

func route(from nmtypes.Symbol, to nmtypes.Symbol) nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(from, calculus.PortX),
		nmtypes.Out(calculus.PortX, to),
	)
}

func prefixed(prefix string, name string) nmtypes.Symbol {
	return nmtypes.MustIntern(temporal.JoinPrefix(prefix, name))
}

/*
velocityOver routes one fact plus the event clock into a namespaced series and
runs the first-difference estimator over it.
*/
func velocityOver(prefix string, value nmtypes.Symbol) nmtypes.Primitive {
	series := temporal.NewSeries(prefix)

	return nmtypes.Pipe(
		route(value, series.ValueSymbol),
		route(nmtypes.EventTimeSec, series.SecSymbol),
		route(nmtypes.EventTimeNsec, series.NsecSymbol),
		statistic.Velocity(prefix),
	)
}

/*
baselineZScore routes one fact plus the event clock into a namespaced series and
runs the causal baseline/z-score chain: evaluate against the prior baseline
first, then update the baseline, then retain the observation.
*/
func baselineZScore(prefix string, value nmtypes.Symbol) nmtypes.Primitive {
	series := temporal.NewSeries(prefix)

	return nmtypes.Pipe(
		route(value, series.ValueSymbol),
		route(nmtypes.EventTimeSec, series.SecSymbol),
		route(nmtypes.EventTimeNsec, series.NsecSymbol),
		statistic.ZScore(prefix),
		statistic.Baseline(prefix),
		temporal.Window(prefix),
	)
}
