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
type MarketScenario struct {
	Probability float64
	LogReturn   float64
}

type resonanceMarketModel struct {
	// drift is the per-step expected log return, signed by the direction call.
	drift float64
	// volatility is the per-step log-return standard deviation.
	volatility float64
	// direction is the sign (+1, -1, 0) from Resonance.
	direction float64
	// confidence is the forecast confidence.
	confidence float64
	// uncertainty is the per-step forecast uncertainty.
	uncertainty float64
	// magnitude is the inferred move magnitude from Passage or economics.
	magnitude float64
	// scenarios preserves the discrete multimodal distribution across rollouts.
	scenarios []MarketScenario
	// cadence is the event-time spacing one step represents.
	cadence time.Duration
}

/*
maxStepDrift bounds one rollout step's expected log return when unscaled.
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
	magnitude ...float64,
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

	mag := maxStepDrift

	if len(magnitude) > 0 && magnitude[0] > 0 {
		stepDrift := magnitude[0] / horizon

		if stepDrift > maxStepDrift {
			stepDrift = maxStepDrift
		}

		mag = stepDrift
	}

	// The direction call carries the sign; confidence scales the magnitude.
	drift := math.Copysign(
		math.Min(mag*artifact.Confidence, maxStepDrift),
		forecast.Call,
	)

	if cadence <= 0 {
		cadence = time.Second
	}

	direction := forecast.Call
	confidence := artifact.Confidence
	uncertainty := volatility

	scenarios := []MarketScenario{
		{Probability: confidence, LogReturn: math.Copysign(mag, direction)},
		{Probability: (1 - confidence) * 0.7, LogReturn: 0},
		{Probability: (1 - confidence) * 0.3, LogReturn: math.Copysign(-mag*0.5, direction)},
	}

	return &resonanceMarketModel{
		drift:       drift,
		volatility:  volatility,
		direction:   direction,
		confidence:  confidence,
		uncertainty: uncertainty,
		magnitude:   mag,
		scenarios:   scenarios,
		cadence:     cadence,
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

	if len(model.scenarios) > 0 {
		u := 0.5

		if random != nil {
			u = random.Float64()
		}

		cum := 0.0

		for _, sc := range model.scenarios {
			cum += sc.Probability

			if u <= cum {
				logReturn = sc.LogReturn
				break
			}
		}
	}

	if random != nil && model.volatility > 0 {
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

func moveMultiplier(move advisor.MarketMove) float64 {
	switch move {
	case advisor.MoveExplosivePump:
		return 2.0
	case advisor.MoveSteadyTrend:
		return 1.0
	case advisor.MoveWeakDrift:
		return 0.25
	case advisor.MoveStagnant:
		return 0
	case advisor.MoveWeakBleed:
		return -0.25
	case advisor.MoveStructuralPullback:
		return -1.0
	case advisor.MoveFlashDump:
		return -2.0
	default:
		return 0
	}
}

/*
moveDrift scales a qualitative market move by an explicit inferred magnitude,
never by uncalibrated magic percentages.
*/
func moveDrift(move advisor.MarketMove, magnitude ...float64) float64 {
	mag := 0.008

	if len(magnitude) > 0 && magnitude[0] > 0 {
		mag = magnitude[0]
	}

	return moveMultiplier(move) * mag
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
deliberated distribution over the seven market moves, sampling discrete
multimodal scenarios rather than collapsing to a single Gaussian mean.
*/
func newConsensusMarketModel(
	consensus *advisor.DeliberationOutcome,
	cadence time.Duration,
	magnitude ...float64,
) (*resonanceMarketModel, bool) {
	if consensus == nil || consensus.Participants == 0 ||
		len(consensus.Probabilities) == 0 {
		return nil, false
	}

	mag := 0.008

	if len(magnitude) > 0 && magnitude[0] > 0 {
		mag = magnitude[0]
	}

	drift := 0.0
	var scenarios []MarketScenario

	for move, probability := range consensus.Probabilities {
		d := moveDrift(move, mag)
		drift += probability * d

		if probability > 0 {
			scenarios = append(scenarios, MarketScenario{
				Probability: probability,
				LogReturn:   d,
			})
		}
	}

	// The spread of the move distribution is the council's disagreement about
	// which regime is coming, expressed in the same units as the drift.
	variance := 0.0

	for move, probability := range consensus.Probabilities {
		deviation := moveDrift(move, mag) - drift
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
		drift:       drift,
		volatility:  volatility,
		direction:   math.Copysign(1, drift),
		confidence:  consensus.Confidence,
		uncertainty: volatility,
		magnitude:   mag,
		scenarios:   scenarios,
		cadence:     cadence,
	}, true
}
