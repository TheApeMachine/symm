package strategy

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/nomagique/relation"
	"github.com/theapemachine/symm/types"
)

/*
resonanceMarketModel evolves market state during MCTS rollouts using the
Resonance direction head as the transition source.

Resonance publishes a signed direction call and the uncertainty around it, not
a priced return. That distinction is preserved here: the call sets the drift's
sign and the distribution's scale sets the step magnitude, so a rollout walks a
plausible trajectory rather than a fabricated price target. This is why the
deleted RLS predictor was redundant — Resonance already answers the directional
question, and the predictor was re-deriving it through a second scalar model.

The model is deliberately conservative. A held call publishes zero, an
uncalibrated artifact yields no drift at all, and the per-step magnitude is
bounded, so the search cannot manufacture an edge from a forecast that does not
claim one.
*/
type resonanceMarketModel struct {
	// drift is the per-step expected log return, signed by the direction call.
	drift float64
	// volatility is the per-step log-return standard deviation.
	volatility float64
	// cadence is the event-time spacing one step represents.
	cadence time.Duration
}

/*
maxStepDrift bounds one rollout step's expected log return. Resonance reports a
direction and an uncertainty, never a magnitude, so the magnitude here is
strategy policy and is capped rather than left to scale with an unbounded
confidence.
*/
const maxStepDrift = 0.002

/*
minStepVolatility keeps rollouts stochastic when the forecast reports almost no
uncertainty. Without it a confident forecast would produce a deterministic path
and the search would read its own drift back as certainty.
*/
const minStepVolatility = 0.0005

/*
newResonanceMarketModel derives a transition model from one resonance artifact.
It reports not-ready when the artifact cannot support a forecast, so the caller
declines to search rather than rolling out on invented dynamics.
*/
func newResonanceMarketModel(
	artifact *types.ResonanceArtifact,
	cadence time.Duration,
) (*resonanceMarketModel, bool) {
	if artifact == nil || !artifact.Calibrated || artifact.Forecast == nil {
		return nil, false
	}

	if artifact.SupportedHorizon <= 0 || artifact.Confidence <= 0 {
		return nil, false
	}

	forecast := artifact.Forecast

	// A held call is continuity, never fresh conviction: it must not drive a
	// rollout as though it were an active directional claim.
	if forecast.Held || forecast.Call == 0 {
		return nil, false
	}

	if !forecast.Distribution.Ready {
		return nil, false
	}

	scale := forecast.Distribution.Scale

	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return nil, false
	}

	horizon := float64(max(artifact.SupportedHorizon, 1))

	if forecast.Horizon > 0 {
		horizon = float64(forecast.Horizon)
	}

	// Scale multi-step horizon variance to per-step volatility: sigma_step = sigma_H / sqrt(H).
	stepScale := scale / math.Sqrt(horizon)
	volatility := math.Max(stepScale, minStepVolatility)

	// The direction call carries the sign; confidence scales the magnitude
	// within an explicit cap.
	drift := math.Copysign(
		math.Min(maxStepDrift*artifact.Confidence, maxStepDrift),
		forecast.Call,
	)

	if cadence <= 0 {
		cadence = time.Second
	}

	return &resonanceMarketModel{
		drift:      drift,
		volatility: volatility,
		cadence:    cadence,
	}, true
}

/*
Step advances market state one rollout step. The random source samples the
transition residual so repeated visits to one action integrate over the
distribution instead of replaying a single path.
*/
func (model *resonanceMarketModel) Step(
	current mcts.MarketState,
	random *rand.Rand,
) (mcts.MarketState, float64, float64, error) {
	if model == nil {
		return current, 0, 0, fmt.Errorf("strategy: market model required")
	}

	logReturn := model.drift

	if random != nil {
		logReturn += random.NormFloat64() * model.volatility
	}

	if math.IsNaN(logReturn) || math.IsInf(logReturn, 0) {
		return current, 0, 0, fmt.Errorf("strategy: market transition was not finite")
	}

	next := mcts.MarketState{
		At:      current.At.Add(model.cadence),
		Current: make(map[relation.Coordinate]float64, len(current.Current)),
		History: current.History,
	}

	growth := math.Exp(logReturn)

	for coordinate, value := range current.Current {
		next.Current[coordinate] = value * growth
	}

	return next, logReturn, model.volatility, nil
}

/*
moveDrift is the per-step expected log return each qualitative market move
implies. These are regime magnitudes, not price predictions: an explosive pump
is a different kind of event from a weak drift, and a rollout that cannot tell
them apart cannot weigh entering one against waiting through the other.

They are declared strategy policy. The scale is deliberately large relative to
the resonance model's bounded drift, because that model answers a different
question — how a calibrated forecast expects price to creep — while these
answer what a named regime does when it happens.
*/
func moveDrift(move advisor.MarketMove) float64 {
	switch move {
	case advisor.MoveExplosivePump:
		return 0.030
	case advisor.MoveSteadyTrend:
		return 0.008
	case advisor.MoveWeakDrift:
		return 0.002
	case advisor.MoveStagnant:
		return 0
	case advisor.MoveWeakBleed:
		return -0.002
	case advisor.MoveStructuralPullback:
		return -0.008
	case advisor.MoveFlashDump:
		return -0.040
	default:
		return 0
	}
}

/*
consensusVolatilityFloor keeps a consensus-driven rollout stochastic even when
the council is unanimous. A distribution that collapsed onto one move would
otherwise produce a deterministic path and let the search read its own drift
back as certainty.
*/
const consensusVolatilityFloor = 0.004

/*
newConsensusMarketModel derives a transition model from the War Room's
deliberated distribution over the seven market moves.

This is the cold-start path. Resonance needs many completed volume bars before
it calibrates, and a newly listed or newly active symbol has none — which is
exactly when a pump is most likely and least modeled. Aborting there would make
the system structurally blind to its best setups.

The drift is the probability-weighted expectation across the moves, so a
council that is 70% confident of an explosive pump rolls out an explosive
regime while a divided council rolls out something close to flat. The
volatility is the distribution's own spread: genuine disagreement about which
regime is coming is uncertainty, and it belongs in the rollout rather than
being averaged away.
*/
func newConsensusMarketModel(
	consensus *advisor.DeliberationOutcome,
	cadence time.Duration,
) (*resonanceMarketModel, bool) {
	if consensus == nil || consensus.Participants == 0 ||
		len(consensus.Probabilities) == 0 {
		return nil, false
	}

	drift := 0.0

	for move, probability := range consensus.Probabilities {
		drift += probability * moveDrift(move)
	}

	// The spread of the move distribution is the council's disagreement about
	// which regime is coming, expressed in the same units as the drift.
	variance := 0.0

	for move, probability := range consensus.Probabilities {
		deviation := moveDrift(move) - drift
		variance += probability * deviation * deviation
	}

	volatility := math.Sqrt(variance)

	if volatility < consensusVolatilityFloor {
		volatility = consensusVolatilityFloor
	}

	if math.IsNaN(drift) || math.IsInf(drift, 0) ||
		math.IsNaN(volatility) || math.IsInf(volatility, 0) {
		return nil, false
	}

	if cadence <= 0 {
		cadence = time.Second
	}

	return &resonanceMarketModel{
		drift:      drift,
		volatility: volatility,
		cadence:    cadence,
	}, true
}
