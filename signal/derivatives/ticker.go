package derivatives

import (
	"fmt"
	"sync"
	"time"

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
	symbolCurrentPositive      = nmtypes.MustIntern("derivatives/current_positive")
	symbolDerivativePositive   = nmtypes.MustIntern("derivatives/derivative_positive")
	symbolReferencePositive    = nmtypes.MustIntern("derivatives/reference_positive")
	symbolSpotPositive         = nmtypes.MustIntern("derivatives/spot_positive")
	symbolPricesPositive       = nmtypes.MustIntern("derivatives/prices_positive")
	symbolPreviousPositive     = nmtypes.MustIntern("derivatives/previous_positive")
)

/*
Ticker is the derivative/reference state market entity. It owns exactly a
Number pipeline and a projector, both declared in its constructor, plus Step
and Close.
*/
type Ticker struct {
	number    *nomagique.Number[string]
	projector *data.Projector
	// last retains, per symbol, the most recent event time.Time so the
	// observer's causal "event time must not regress" invariant holds even when
	// a futures snapshot carries no server timestamp (the shared parser would
	// otherwise fabricate a different, non-monotonic wall-clock base) or arrives
	// out of order. One monotonic causal timeline per symbol is kept.
	last sync.Map
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
			// Log basis and the three-price log geometry (derivative, index,
			// spot). A contract that has not traded reports a price of ZERO --
			// a real market state, not bad data -- and both log and log-ratio
			// are undefined there. The arithmetic basis above still reports
			// the relationship; its log-space counterparts are absent rather
			// than zero, and an ungated log would fail the whole frame,
			// discarding every price metric alongside them.
			logic.If(
				allPricesPositive(),
				nmtypes.Pipe(
					// Log basis: log(derivative / reference).
					nmtypes.Wire(
						calculus.LogRatio,
						nmtypes.In(symbolDerivativePrice, calculus.SymbolCurrent),
						nmtypes.In(symbolReferencePrice, calculus.SymbolPrevious),
						nmtypes.Out(calculus.PortResult, symbolLogBasis),
					),
					// Each leg is a log ratio composed from per-leg natural logs.
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
				),
				nmtypes.Identity,
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
					// Elapsed event time between the previous and current observation.
					nmtypes.Wire(
						temporal.Duration,
						nmtypes.In(nmtypes.EventTimeSec, temporal.SymbolCurrentSec),
						nmtypes.In(nmtypes.EventTimeNsec, temporal.SymbolCurrentNsec),
						nmtypes.In(temporal.SymbolObservedSec, temporal.SymbolPreviousSec),
						nmtypes.In(temporal.SymbolObservedNsec, temporal.SymbolPreviousNsec),
						nmtypes.Out(temporal.SymbolDelta, symbolOIElapsed),
					),
					// open_interest_log_change and everything derived from it.
					// Open interest is legitimately ZERO on a contract nobody
					// holds — a real market state, not bad data — and the log
					// ratio is undefined at either endpoint being zero. The
					// arithmetic open_interest_change above still reports the
					// move; its log-space counterpart, the growth rate, and
					// that rate's estimators are absent rather than zero.
					logic.If(
						bothEndpointsPositive(),
						nmtypes.Pipe(
							nmtypes.Wire(
								calculus.LogRatio,
								nmtypes.In(calculus.SymbolCurrent, calculus.SymbolCurrent),
								nmtypes.In(calculus.SymbolPrevious, calculus.SymbolPrevious),
								nmtypes.Out(calculus.PortResult, symbolOILogChange),
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
						nil,
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
			// derivative_log_return: log(current / previous) over the derivative
			// price. Readiness alone only means a previous observation EXISTS;
			// a price of zero at either endpoint is a real market state whose
			// log ratio is undefined, so both endpoints must also be positive.
			temporal.Observer("derivative_return", symbolDerivativePrice),
			logic.If(
				bothEndpointsPositive(),
				nmtypes.Wire(
					calculus.LogRatio,
					nmtypes.In(calculus.SymbolCurrent, calculus.SymbolCurrent),
					nmtypes.In(calculus.SymbolPrevious, calculus.SymbolPrevious),
					nmtypes.Out(calculus.PortResult, symbolDerivativeLogReturn),
				),
				nmtypes.Identity,
			),
			// reference_log_return: log(current / previous) over the reference
			// price, positivity-gated for the same reason as the derivative leg.
			temporal.Observer("reference_return", symbolReferencePrice),
			logic.If(
				bothEndpointsPositive(),
				nmtypes.Wire(
					calculus.LogRatio,
					nmtypes.In(calculus.SymbolCurrent, calculus.SymbolCurrent),
					nmtypes.In(calculus.SymbolPrevious, calculus.SymbolPrevious),
					nmtypes.Out(calculus.PortResult, symbolReferenceLogReturn),
				),
				nmtypes.Identity,
			),
			// return_gap = derivative_log_return - reference_log_return, then its
			// velocity and its own causal baseline + z-score. Each leg is
			// positivity-gated above and is therefore absent when its price
			// touched zero, so the gap is computed only when BOTH legs exist.
			logic.If(
				bothLogReturnsPresent(),
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
	if point.Last == nil || point.IndexPrice == nil || point.MarkPrice == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf(
			"derivatives: ticker requires last, index, and mark prices",
		)}
	}

	point.Timestamp = ticker.monotonicClock(point.Symbol, point.Timestamp)

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

/*
monotonicClock folds one snapshot timestamp into the symbol's causal timeline.
The Kraken Futures ticker feed can carry no server timestamp for a snapshot;
the shared parser then falls back to the local wall clock, which is a different
time base from the exchange's and will periodically read as *older* than the
previous exchange-stamped event, regressing the observer's clock. It can also
deliver a snapshot whose server timestamp is older than the previous one. In
both cases the causal invariant "event time must not regress" is violated not
by bad computation but by a non-monotonic timestamp source.

The correction is to advance a single per-symbol monotonic timeline: a snapshot
whose timestamp regresses (or is missing) is re-stamped with the symbol's last
observed time. The event still ingests its price facts unchanged; only the clock
label is made causal, so the observer never sees its invariant broken. A zero
timestamp is never stamped: it would appear as a regression after any real
observation, poisoning the first valid event.
*/
func (ticker *Ticker) monotonicClock(symbol string, timestamp time.Time) time.Time {
	if timestamp.IsZero() {
		return timestamp
	}

	loaded, _ := ticker.last.Load(symbol)
	previous, _ := loaded.(time.Time)

	if timestamp.Before(previous) {
		return previous
	}

	ticker.last.Store(symbol, timestamp)

	return timestamp
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

/*
allPricesPositive is true only when the derivative, reference, and spot prices
are all above zero — the condition the log-space basis geometry requires. A
contract that has not traded reports a price of zero, which is a real market
state and not bad data.
*/
func allPricesPositive() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Wire(
			logic.GreaterThan,
			nmtypes.In(symbolDerivativePrice, calculus.PortA),
			nmtypes.In(symbolZero, calculus.PortB),
			nmtypes.Out(logic.SymbolCondition, symbolDerivativePositive),
		),
		nmtypes.Wire(
			logic.GreaterThan,
			nmtypes.In(symbolReferencePrice, calculus.PortA),
			nmtypes.In(symbolZero, calculus.PortB),
			nmtypes.Out(logic.SymbolCondition, symbolReferencePositive),
		),
		nmtypes.Wire(
			logic.GreaterThan,
			nmtypes.In(symbolSpotPrice, calculus.PortA),
			nmtypes.In(symbolZero, calculus.PortB),
			nmtypes.Out(logic.SymbolCondition, symbolSpotPositive),
		),
		nmtypes.Wire(
			logic.And,
			nmtypes.In(symbolDerivativePositive, calculus.PortA),
			nmtypes.In(symbolReferencePositive, calculus.PortB),
			nmtypes.Out(logic.SymbolCondition, symbolPricesPositive),
		),
		nmtypes.Wire(
			logic.And,
			nmtypes.In(symbolPricesPositive, calculus.PortA),
			nmtypes.In(symbolSpotPositive, calculus.PortB),
			nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
		),
	)
}

/*
bothEndpointsPositive is true only when the observer holds a previous value AND
both endpoints are above zero — the condition a log ratio between them
requires. Readiness is part of the condition because an observer's first
observation has no previous fact at all: reading it would fail the frame rather
than report the absence the gate exists to handle.
*/
func bothEndpointsPositive() nmtypes.Primitive {
	return nmtypes.Pipe(
		logic.If(
			readyCondition(),
			nmtypes.Pipe(
				nmtypes.Wire(
					logic.GreaterThan,
					nmtypes.In(calculus.SymbolCurrent, calculus.PortA),
					nmtypes.In(symbolZero, calculus.PortB),
					nmtypes.Out(logic.SymbolCondition, symbolCurrentPositive),
				),
				nmtypes.Wire(
					logic.GreaterThan,
					nmtypes.In(calculus.SymbolPrevious, calculus.PortA),
					nmtypes.In(symbolZero, calculus.PortB),
					nmtypes.Out(logic.SymbolCondition, symbolPreviousPositive),
				),
				nmtypes.Wire(
					logic.And,
					nmtypes.In(symbolCurrentPositive, calculus.PortA),
					nmtypes.In(symbolPreviousPositive, calculus.PortB),
					nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
				),
			),
			// Not ready: no previous endpoint exists, so the ratio is absent.
			nmtypes.Assign(logic.SymbolCondition, 0),
		),
	)
}

/*
bothLogReturnsPresent is true only when both the derivative and reference log
returns were actually produced this step. Each is positivity-gated, so either
can legitimately be absent on a contract whose price touched zero.
*/
func bothLogReturnsPresent() nmtypes.Primitive {
	return func(input *nmtypes.Frame) {
		_, hasDerivative := input.Get(symbolDerivativeLogReturn)
		_, hasReference := input.Get(symbolReferenceLogReturn)

		condition := 0.0

		if hasDerivative && hasReference {
			condition = 1
		}

		input.Put(logic.SymbolCondition, condition)
	}
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
