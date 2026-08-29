package sentiment

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Output slots for the cross-sectional metrics folded from the shared
*data.CrossSection. Each slot maps to one README metric name in the projector.
*/
var (
	sAdvanceCount               = metricSlot("advance_count")
	sDeclineCount               = metricSlot("decline_count")
	sUnchangedCount             = metricSlot("unchanged_count")
	sValidMemberCount           = metricSlot("valid_member_count")
	sCohortMemberCount          = metricSlot("cohort_member_count")
	sExcludedMemberCount        = metricSlot("excluded_member_count")
	sSameDirectionPeerCount     = metricSlot("same_direction_peer_count")
	sOppositeDirectionPeerCount = metricSlot("opposite_direction_peer_count")
	sZeroDirectionPeerCount     = metricSlot("zero_return_peer_count")

	sAdvanceFraction               = metricSlot("advance_fraction")
	sDeclineFraction               = metricSlot("decline_fraction")
	sUnchangedFraction             = metricSlot("unchanged_fraction")
	sDirectionalParticipation      = metricSlot("directional_participation")
	sDirectionalAgreement          = metricSlot("directional_agreement")
	sDirectionalConsensus          = metricSlot("directional_consensus")
	sSameDirectionPeerFraction     = metricSlot("same_direction_peer_fraction")
	sOppositeDirectionPeerFraction = metricSlot("opposite_direction_peer_fraction")
	sZeroDirectionPeerFraction     = metricSlot("zero_return_peer_fraction")

	sBreadth           = metricSlot("breadth")
	sBreadthBaseline   = metricSlot("breadth_baseline")
	sBreadthDivergence = metricSlot("breadth_divergence")
	sBreadthVelocity   = metricSlot("breadth_velocity")
	sBreadthZScore     = metricSlot("breadth_zscore")

	sMedianReturn           = metricSlot("median_return")
	sMedianReturnBaseline   = metricSlot("median_return_baseline")
	sMedianReturnDivergence = metricSlot("median_return_divergence")
	sMedianReturnVelocity   = metricSlot("median_return_velocity")
	sMedianReturnZScore     = metricSlot("median_return_zscore")

	sMedianAbsReturn         = metricSlot("median_absolute_return")
	sMedianAbsReturnBaseline = metricSlot("median_absolute_return_baseline")
	sMedianAbsReturnRatio    = metricSlot("median_absolute_return_ratio")
	sMedianAbsReturnVelocity = metricSlot("median_absolute_return_velocity")
	sMedianAbsReturnZScore   = metricSlot("median_absolute_return_zscore")

	sMeanAbsReturn = metricSlot("mean_absolute_return")
	sRmsReturn     = metricSlot("rms_return")
	sIqrReturn     = metricSlot("return_interquartile_range")

	sReturnDispersionBaseline = metricSlot("return_dispersion_baseline")
	sReturnDispersionRatio    = metricSlot("return_dispersion_ratio")
	sReturnDispersionVelocity = metricSlot("return_dispersion_velocity")
	sReturnDispersionZScore   = metricSlot("return_dispersion_zscore")

	sReturnMad    = metricSlot("return_mad")
	sMagnitudeMad = metricSlot("magnitude_mad")

	sLargestAbsReturn         = metricSlot("largest_absolute_return")
	sLargestTieCount          = metricSlot("largest_move_tie_count")
	sLargestMoveExcess        = metricSlot("largest_move_excess")
	sLargestMadExcess         = metricSlot("largest_move_mad_excess")
	sLargestSignedReturn      = metricSlot("largest_signed_return")
	sLargestMoveRatio         = metricSlot("largest_move_ratio")
	sLargestMoveRatioBaseline = metricSlot("largest_move_ratio_baseline")
	sLargestMoveRatioZScore   = metricSlot("largest_move_ratio_zscore")
	sLargestMoveShare         = metricSlot("largest_move_share")
	sLargestMoveShareBaseline = metricSlot("largest_move_share_baseline")
	sLargestMoveShareZScore   = metricSlot("largest_move_share_zscore")

	sPeerMedianAbsReturn = metricSlot("peer_median_absolute_return")
	sPeerMad             = metricSlot("peer_magnitude_mad")

	sMedianAsofAge = metricSlot("median_asof_age_seconds")
	sMaxAsofAge    = metricSlot("max_asof_age_seconds")
	sMedianFromAge = metricSlot("median_from_age_seconds")
	sCohortHorizon = metricSlot("cohort_horizon_seconds")
	sAsofAge       = metricSlot("asof_age_seconds")
	sFromAge       = metricSlot("from_age_seconds")
)

func metricSlot(name string) nmtypes.Symbol {
	return nmtypes.MustIntern("sentiment/" + name)
}

/*
Ticker is the per-symbol price-state market entity. It owns a Number pipeline
that retains each symbol's price path and derives its log return, plus a
projector. The cross-sectional stage is the shared *data.CrossSection held in
the workspace pool: each Step folds the member's price into it and folds the
resulting Snapshot into the same single measurement.
*/
type Ticker struct {
	section   *data.CrossSection
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTicker constructs the Ticker entity and its cross-section. There is exactly
one Ticker per process (the sentiment stage is a single Node), so the
cross-section needs no cross-instance coordination.
*/
func NewTicker() *Ticker {
	section := data.NewCrossSection()

	return &Ticker{
		section:   section,
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			temporal.Path(""),
			correlation.Return,
		)),
		projector: data.NewProjector(
			data.Binding{From: correlation.SymbolReturn, Name: "return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: correlation.SymbolMagnitude, Name: "absolute_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sAdvanceCount, Name: "advance_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sDeclineCount, Name: "decline_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sUnchangedCount, Name: "unchanged_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sValidMemberCount, Name: "valid_member_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sCohortMemberCount, Name: "cohort_member_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sExcludedMemberCount, Name: "excluded_member_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sSameDirectionPeerCount, Name: "same_direction_peer_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sOppositeDirectionPeerCount, Name: "opposite_direction_peer_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sZeroDirectionPeerCount, Name: "zero_return_peer_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sAdvanceFraction, Name: "advance_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sDeclineFraction, Name: "decline_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sUnchangedFraction, Name: "unchanged_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sDirectionalParticipation, Name: "directional_participation", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sDirectionalAgreement, Name: "directional_agreement", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sDirectionalConsensus, Name: "directional_consensus", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sSameDirectionPeerFraction, Name: "same_direction_peer_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sOppositeDirectionPeerFraction, Name: "opposite_direction_peer_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sZeroDirectionPeerFraction, Name: "zero_return_peer_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sBreadth, Name: "breadth", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sBreadthBaseline, Name: "breadth_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sBreadthDivergence, Name: "breadth_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sBreadthVelocity, Name: "breadth_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sBreadthZScore, Name: "breadth_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sMedianReturn, Name: "median_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMedianReturnBaseline, Name: "median_return_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMedianReturnDivergence, Name: "median_return_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMedianReturnVelocity, Name: "median_return_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMedianReturnZScore, Name: "median_return_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sMedianAbsReturn, Name: "median_absolute_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMedianAbsReturnBaseline, Name: "median_absolute_return_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMedianAbsReturnRatio, Name: "median_absolute_return_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMedianAbsReturnVelocity, Name: "median_absolute_return_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMedianAbsReturnZScore, Name: "median_absolute_return_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sMeanAbsReturn, Name: "mean_absolute_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sRmsReturn, Name: "rms_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sIqrReturn, Name: "return_interquartile_range", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sReturnDispersionBaseline, Name: "return_dispersion_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sReturnDispersionRatio, Name: "return_dispersion_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sReturnDispersionVelocity, Name: "return_dispersion_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sReturnDispersionZScore, Name: "return_dispersion_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sReturnMad, Name: "return_mad", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMagnitudeMad, Name: "magnitude_mad", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sLargestAbsReturn, Name: "largest_absolute_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestTieCount, Name: "largest_move_tie_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestMoveExcess, Name: "largest_move_excess", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestMadExcess, Name: "largest_move_mad_excess", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestSignedReturn, Name: "largest_signed_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestMoveRatio, Name: "largest_move_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestMoveRatioBaseline, Name: "largest_move_ratio_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestMoveRatioZScore, Name: "largest_move_ratio_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestMoveShare, Name: "largest_move_share", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestMoveShareBaseline, Name: "largest_move_share_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sLargestMoveShareZScore, Name: "largest_move_share_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sPeerMedianAbsReturn, Name: "peer_median_absolute_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sPeerMad, Name: "peer_magnitude_mad", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			data.Binding{From: sMedianAsofAge, Name: "median_asof_age_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMaxAsofAge, Name: "max_asof_age_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sMedianFromAge, Name: "median_from_age_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sCohortHorizon, Name: "cohort_horizon_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sAsofAge, Name: "asof_age_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: sFromAge, Name: "from_age_seconds", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
Step receives one ticker data point, loads the price into the per-symbol path,
runs the Number pipeline, folds the price into the shared cross-section, and
folds the resulting Snapshot into the same single measurement before
projecting.
*/
func (ticker *Ticker) Step(tick kraken.TickerData) *data.Measurement[float64] {
	if tick.Last == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("sentiment: ticker requires a last price")}
	}

	input := nmtypes.Frame{}
	input.Put(nmtypes.SampleValue, tick.Last.Float64())
	input.Put(nmtypes.EventTimeSec, float64(tick.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(tick.Timestamp.Nanosecond()))

	frame := ticker.number.Step(tick.Symbol, input)

	snapshot, hasSnapshot := ticker.section.Process(
		tick.Symbol,
		tick.Last.Float64(),
		tick.Timestamp,
		tick.Symbol,
	)

	if hasSnapshot {
		foldSnapshot(&frame, snapshot)
	}

	measurement := ticker.projector.Project(
		tick.Symbol,
		"sentiment",
		tick.Timestamp,
		tick.Timestamp,
		frame,
	)

	if hasSnapshot {
		if measurement.Provenance == nil {
			measurement.Provenance = map[string]string{}
		}

		measurement.Provenance["largest_move_symbol"] = snapshot.ExtremeKey
	}

	return measurement
}

func (ticker *Ticker) Close() error { return nil }

/*
foldSnapshot is the data-container fold that maps one cross-sectional Snapshot
onto the measurement's output slots. Ratios with a zero denominator are left
undefined (their slot stays absent) rather than fabricated as zero.

previous_level_disposition is intentionally not emitted: it is qualitative
per-side provenance (touch-only vs full-book attribution, unchanged vs
retreat vs improve) that does not map to a single numeric metric.
*/
func foldSnapshot(frame *nmtypes.Frame, snapshot data.Snapshot) {
	valid := float64(snapshot.Count)
	positive := float64(snapshot.PositiveCount)
	negative := float64(snapshot.NegativeCount)
	zero := float64(snapshot.ZeroCount)

	frame.Put(sAdvanceCount, positive)
	frame.Put(sDeclineCount, negative)
	frame.Put(sUnchangedCount, zero)
	frame.Put(sValidMemberCount, valid)
	frame.Put(sCohortMemberCount, float64(snapshot.TotalMembers))
	frame.Put(sExcludedMemberCount, float64(snapshot.TotalMembers-snapshot.Count))

	putRatio(frame, sAdvanceFraction, positive, valid)
	putRatio(frame, sDeclineFraction, negative, valid)
	putRatio(frame, sUnchangedFraction, zero, valid)
	putRatio(frame, sDirectionalParticipation, positive+negative, valid)
	putRatio(frame, sDirectionalAgreement, math.Max(positive, negative), positive+negative)
	putRatio(frame, sDirectionalConsensus, math.Abs(positive-negative), positive+negative)

	peerCount := float64(snapshot.Count - 1)
	frame.Put(sSameDirectionPeerCount, float64(snapshot.SameDirectionCount))
	frame.Put(sOppositeDirectionPeerCount, float64(snapshot.OppositeDirectionCount))
	frame.Put(sZeroDirectionPeerCount, float64(snapshot.ZeroDirectionCount))
	putRatio(frame, sSameDirectionPeerFraction, float64(snapshot.SameDirectionCount), peerCount)
	putRatio(frame, sOppositeDirectionPeerFraction, float64(snapshot.OppositeDirectionCount), peerCount)
	putRatio(frame, sZeroDirectionPeerFraction, float64(snapshot.ZeroDirectionCount), peerCount)

	emitAggregate(
		frame,
		snapshot.Aggregates["signed_fraction"],
		sBreadth, sBreadthBaseline, sBreadthDivergence, sBreadthZScore, sBreadthVelocity,
	)
	emitAggregate(
		frame,
		snapshot.Aggregates["signed_median"],
		sMedianReturn, sMedianReturnBaseline, sMedianReturnDivergence, sMedianReturnZScore, sMedianReturnVelocity,
	)
	emitAggregate(
		frame,
		snapshot.Aggregates["median_absolute"],
		sMedianAbsReturn, sMedianAbsReturnBaseline, 0, sMedianAbsReturnZScore, sMedianAbsReturnVelocity,
	)
	emitRatio(frame, sMedianAbsReturnRatio, snapshot.Aggregates["median_absolute"])
	emitAggregate(frame, snapshot.Aggregates["mean_absolute"], sMeanAbsReturn, 0, 0, 0, 0)
	emitAggregate(frame, snapshot.Aggregates["rms"], sRmsReturn, 0, 0, 0, 0)
	emitAggregate(
		frame,
		snapshot.Aggregates["iqr"],
		sIqrReturn, sReturnDispersionBaseline, 0, sReturnDispersionZScore, sReturnDispersionVelocity,
	)
	emitRatio(frame, sReturnDispersionRatio, snapshot.Aggregates["iqr"])

	frame.Put(sReturnMad, snapshot.Mad)
	frame.Put(sMagnitudeMad, snapshot.MagnitudeMad)

	frame.Put(sLargestAbsReturn, snapshot.ExtremeMagnitude)
	frame.Put(sLargestTieCount, float64(snapshot.ExtremeTieCount))

	// largest_signed_return is defined only for a unique largest mover.
	if snapshot.ExtremeTieCount == 0 {
		frame.Put(sLargestSignedReturn, snapshot.ExtremeSigned)
	}

	frame.Put(sLargestMoveExcess, snapshot.ExtremeMagnitude-snapshot.PeerMedianAbsolute)
	putRatio(frame, sLargestMadExcess, snapshot.ExtremeMagnitude-snapshot.PeerMedianAbsolute, snapshot.PeerMad)

	emitAggregate(
		frame,
		snapshot.Aggregates["extreme_ratio"],
		sLargestMoveRatio, sLargestMoveRatioBaseline, 0, sLargestMoveRatioZScore, 0,
	)
	emitAggregate(
		frame,
		snapshot.Aggregates["extreme_share"],
		sLargestMoveShare, sLargestMoveShareBaseline, 0, sLargestMoveShareZScore, 0,
	)

	frame.Put(sPeerMedianAbsReturn, snapshot.PeerMedianAbsolute)
	frame.Put(sPeerMad, snapshot.PeerMad)

	frame.Put(sMedianAsofAge, snapshot.MedianAge)
	frame.Put(sMaxAsofAge, snapshot.MaxAge)
	frame.Put(sMedianFromAge, snapshot.MedianFromAge)
	frame.Put(sCohortHorizon, snapshot.MaxAge)
	frame.Put(sAsofAge, snapshot.FocalAge)
	frame.Put(sFromAge, snapshot.FocalFromAge)
}

/*
emitAggregate projects one aggregate's Value and, when its causal estimator is
ready, its Baseline, Divergence, ZScore, and Velocity. The ratio slot, when
non-zero, carries Value/Baseline for aggregates that define a ratio metric.
*/
func emitAggregate(
	frame *nmtypes.Frame,
	view data.AggregateView,
	value nmtypes.Symbol,
	baseline nmtypes.Symbol,
	divergence nmtypes.Symbol,
	zscore nmtypes.Symbol,
	velocity nmtypes.Symbol,
) {
	if value != 0 {
		frame.Put(value, view.Value)
	}

	if !view.Ready {
		return
	}

	if baseline != 0 {
		frame.Put(baseline, view.Baseline)
	}

	if divergence != 0 {
		frame.Put(divergence, view.Divergence)
	}

	if zscore != 0 {
		frame.Put(zscore, view.ZScore)
	}

	if velocity != 0 {
		frame.Put(velocity, view.Velocity)
	}
}

func putRatio(frame *nmtypes.Frame, slot nmtypes.Symbol, numerator float64, denominator float64) {
	if denominator == 0 {
		return
	}

	frame.Put(slot, numerator/denominator)
}

/*
emitRatio projects an aggregate's Value/Baseline ratio when the estimator is
ready and the baseline is non-zero.
*/
func emitRatio(frame *nmtypes.Frame, slot nmtypes.Symbol, view data.AggregateView) {
	if !view.Ready || view.Baseline == 0 {
		return
	}

	frame.Put(slot, view.Value/view.Baseline)
}
