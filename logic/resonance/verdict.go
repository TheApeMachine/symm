package resonance

import (
	"math"

	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

const (
	/*
		The rank trend is slower than the pace controller it reads, so a verdict
		describes a regime rather than a tick.
	*/
	rankTrendPeriod    = 64
	rankTrendSmoothing = 2

	/*
		A rank trend at or under settledRank means the model's current errors sit
		at or below the middle of their own recent history: it is explaining the
		data at least as well as it has been. Past driftingRank its errors live in
		the upper quartile of that history, which is a model the market has moved
		away from.
	*/
	settledRank  = 0.55
	driftingRank = 0.75

	/*
		Alpha is judged pinned when it sits within this fraction of either rail.
		Inside the band the controller still has room to answer the next regime;
		on a rail it has already spent its authority and the bounds, not the
		controller, are setting the pace.
	*/
	paceRailMargin = 0.05
)

/*
learningVerdict names how well the hierarchy is currently predicting.

Readiness comes first: before the pace controller has filled its window there is
no history to rank against, and a verdict drawn from an empty comparison would
read as confidence rather than as absence.
*/
func learningVerdict(pace learning.PaceOutput, rankTrend float64) (string, float64) {
	if !pace.Ready {
		return "warming", 0
	}

	if rankTrend <= settledRank {
		return "predicting", 1
	}

	if rankTrend <= driftingRank {
		return "drifting", 0
	}

	return "lost", -1
}

/*
paceBand places alpha inside its own bounds, log spaced to match the space the
controller integrates in. A linear placement would report a resting pace of 0.03
as sitting a fifth of the way up a band it is actually centered in.
*/
func paceBand(alpha, minAlpha, maxAlpha float64) float64 {
	if !(alpha > 0) || !(minAlpha > 0) || !(maxAlpha > minAlpha) {
		return 0
	}

	span := math.Log(maxAlpha) - math.Log(minAlpha)
	position := (math.Log(alpha) - math.Log(minAlpha)) / span

	return min(1, max(0, position))
}

/*
tuningVerdict reports whether the pace controller is still free to move.

Alpha's value alone cannot answer this. A pace resting mid-band and a pace
clamped against its ceiling are both just numbers; only the position inside the
interval distinguishes a controller that is adapting from one whose bounds have
taken over.
*/
func tuningVerdict(pace learning.PaceOutput, band float64) (string, float64) {
	if !pace.Ready {
		return "warming", 0
	}

	if band >= 1-paceRailMargin {
		return "pinned fast", 0
	}

	if band <= paceRailMargin {
		return "pinned slow", 0
	}

	return "adapting", 1
}

/*
resonanceVerdict reduces one settled frame to the three readings the predictive
coding panel leads with.

Direction is the raw sign of the expected return and conviction is the forecast's
own confidence, kept apart on purpose: which way the curve points and how much
the model stands behind it are separate facts, and folding them into one number
would let a strong opinion held weakly look identical to a weak one held firmly.
*/
func resonanceVerdict(
	pace learning.PaceOutput,
	rankTrend float64,
	alpha, minAlpha, maxAlpha float64,
	forecast *types.ResonanceForecast,
) types.ResonanceVerdict {
	band := paceBand(alpha, minAlpha, maxAlpha)

	learningLabel, learningHealth := learningVerdict(pace, rankTrend)
	tuningLabel, tuningHealth := tuningVerdict(pace, band)

	verdict := types.ResonanceVerdict{
		Learning:       learningLabel,
		ErrorRank:      rankTrend,
		Tuning:         tuningLabel,
		LearningHealth: learningHealth,
		TuningHealth:   tuningHealth,
		AlphaBand:      band,
	}

	if forecast == nil {
		return verdict
	}

	if forecast.ExpectedReturn > 0 {
		verdict.Direction = 1
	}

	if forecast.ExpectedReturn < 0 {
		verdict.Direction = -1
	}

	verdict.Conviction = forecast.Confidence

	return verdict
}
