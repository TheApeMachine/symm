package strategy

import (
	"math"

	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
Reading is the decomposed admit state for one forecast. Margin is economic
edge after uncertainty; Lead is how far cognition has committed ahead of the
manifold phase basin on a shared [0,1) scale. Reserved overflow is for
anticipatory edge only — high SNR, cognition ahead of the phase compass by
more than noise, multi-head lookahead path agreement, unconfounded Causal Do-calculus,
and a non-ambiguous short-horizon reading.
*/
type Reading struct {
	Margin             float64
	Lead               float64
	Cognitive          float64
	Basin              float64
	BasinReady         bool
	Ambiguous          bool
	Contrast           float64
	Uncertainty        float64
	Horizon            uint64
	Noise              float64
	PhaseClass         string
	PhaseReady         bool
	PhaseSimilarity    float64
	LookaheadScore     float64
	CausalReady        bool
	CausalUplift       float64
	CausalIntervention float64
	CausalNoise        float64
}

/*
Reserved reports whether this reading may consume overflow slots. Normal-lane
enter only needs positive executable utility; reserved further demands that
margin exceed residual uncertainty (SNR > 2), cognitive lead clear the same
noise share CognitiveClears uses, the phase basin be ready, the horizon be the
next event, and cognition be non-ambiguous with positive winner contrast.
*/
func (reading Reading) Reserved() bool {
	if reading.Ambiguous {
		return false
	}

	if reading.Horizon != 1 {
		return false
	}

	if reading.Contrast <= 0 {
		return false
	}

	if reading.Margin <= 0 && reading.Lead <= 0 {
		return false
	}

	if reading.CausalReady && reading.CausalNoise > 0.5 {
		return false
	}

	if reading.BasinReady {
		return reading.Lead >= reading.Noise
	}

	return reading.Lead > 0
}

/*
CognitiveClears reports whether cognition clears the forecast noise share.
Economic enter is decided by executable utility (return − uncertainty −
friction); this gate only blocks Winner=buy at ε confidence.
*/
func (reading Reading) CognitiveClears(forecast types.Forecasts) bool {
	return reading.Cognitive > noiseShare(forecast)
}

/*
isOpposingRegime reports whether a physical market category regime represents an
opposing breakdown, exhaustion, or trap state.
*/
func isOpposingRegime(class string) bool {
	switch types.CategoryType(class) {
	case types.Exhaustion, types.FadedExhaustion, types.MechanicalCollapse,
		types.ThermalExhaustion, types.Turbulent, types.TurbulentResonance,
		types.SystemicSlump, types.ToxicBluff, types.SpoofTrap,
		types.LiquidityShock, types.BookThinning, types.CausalNoise, types.StochasticNoise:
		return true
	}

	return class == "sell"
}

/*
PhaseOpposes reports whether the phase dial's strongest constructive alignment
points at an opposing breakdown/exhaustion physical category regime. Cold corpus,
destructive interference, and neutral/constructive labels do not oppose — they leave
BasinReady false without inventing a veto.
*/
func (reading Reading) PhaseOpposes() bool {
	if !reading.PhaseReady || reading.PhaseSimilarity <= 0 {
		return false
	}

	return isOpposingRegime(reading.PhaseClass)
}

/*
measureOpportunity builds OpportunityMargin and CognitiveLead from independent
estimators. Margin is the SNR term that also enters utility; Lead ranks the
reserved lane once utility has already cleared. Basin is the phase-dial
attractor strength for the cognitive winner, not instantaneous field coherence.
Incorporates multi-head predictive lookahead and Judea Pearl causal ladder output.
*/
func measureOpportunity(
	forecast types.Forecasts,
	cognition types.Cognition,
	thesis *types.Thesis,
) Reading {
	reading := Reading{
		Margin:         forecast.ExpectedReturn - forecast.Uncertainty,
		Cognitive:      cognition.Confidence,
		Ambiguous:      cognition.Ambiguous,
		Contrast:       cognition.Contrast,
		Uncertainty:    forecast.Uncertainty,
		Horizon:        forecast.HorizonEvents,
		Noise:          noiseShare(forecast),
		LookaheadScore: cognition.LookaheadScore,
	}

	if thesis != nil {
		if raw, found := thesis.Causal.Load(forecast.Symbol); found {
			if outcome, ok := raw.(*logic.CausalOutcome); ok && outcome != nil && outcome.Ready {
				reading.CausalReady = true
				reading.CausalUplift = outcome.Reading.UpliftScore
				reading.CausalIntervention = outcome.Reading.InterventionScore
				reading.CausalNoise = outcome.Reading.Noise
			}
		}
	}

	basin, ready, phaseClass, phaseReady, phaseSimilarity := basinConfidence(
		thesis, forecast.Symbol, cognition.Winner,
	)
	reading.Basin = basin
	reading.BasinReady = ready
	reading.PhaseClass = phaseClass
	reading.PhaseReady = phaseReady
	reading.PhaseSimilarity = phaseSimilarity

	if ready {
		score := cognition.Confidence
		if cognition.LookaheadScore > 0 {
			score *= (0.5 + 0.5*cognition.LookaheadScore)
		}
		reading.Lead = score - basin
	}

	return reading
}

/*
noiseShare is the fraction of forecast magnitude explained by uncertainty.
Cognition must exceed this share before a buy winner is treated as actionable.
*/
func noiseShare(forecast types.Forecasts) float64 {
	if forecast.Uncertainty <= 0 {
		return 0
	}

	magnitude := math.Abs(forecast.ExpectedReturn) + forecast.Uncertainty

	return forecast.Uncertainty / magnitude
}

/*
basinConfidence reads the manifold phase compass the way the wave-field dial
does: strongest signed scan response is the current attractor alignment. Basin
is ready only when that alignment is constructive, non-ambiguous, and labeled
with the same class cognition is acting on — otherwise Lead stays neutral
rather than inventing a coherence stand-in. A cold PhaseCorpus leaves the
basin unready so reserved overflow cannot fire without attractor memory.
*/
func basinConfidence(
	thesis *types.Thesis,
	symbol string,
	winner string,
) (float64, bool, string, bool, float64) {
	if thesis == nil || thesis.Manifold == nil || winner == "" {
		return 0, false, "", false, 0
	}

	value, found := thesis.Manifold.Load(symbol)

	if !found {
		return 0, false, "", false, 0
	}

	state, ok := value.(manifold.State)

	if !ok || !state.GasReady() {
		return 0, false, "", false, 0
	}

	alignment, phaseReady := phaseCompass(state)

	if !phaseReady {
		return 0, false, "", false, 0
	}

	if alignment.Outcome.Ambiguous || alignment.Similarity <= 0 {
		return 0, false, alignment.Outcome.Class, true, alignment.Similarity
	}

	if alignment.Outcome.Class != winner {
		return 0, false, alignment.Outcome.Class, true, alignment.Similarity
	}

	// Similarity is already the Hermitian overlap in (-1, 1]; constructive
	// matches occupy (0, 1], so Lead stays on a shared unit interval.
	return alignment.Similarity, true, alignment.Outcome.Class, true, alignment.Similarity
}

/*
phaseCompass selects the strongest signed PhaseScan response — the same
alignment the fluid chart's phase dial labels as the current compass reading.
*/
func phaseCompass(state manifold.State) (manifold.PhaseResponse, bool) {
	if !state.PhaseReady || len(state.PhaseScan) == 0 {
		return manifold.PhaseResponse{}, false
	}

	best := state.PhaseScan[0]

	for _, response := range state.PhaseScan[1:] {
		if response.Similarity > best.Similarity {
			best = response
		}
	}

	return best, true
}
