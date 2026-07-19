package logic

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
observeStates walks manifold state into resonance, causal, and forecast outputs.
*/
func (analyzer *Analyzer) observeStates(thesis *types.Thesis) []manifold.State {
	states := make([]manifold.State, 0)
	observeStarted := time.Now()

	thesis.Manifold.Range(func(key, value any) bool {
		state, ok := value.(manifold.State)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"analyzer received invalid manifold state",
				nil,
			))

			return true
		}

		states = append(states, state)

		// Unchanged excitation epochs still paint the field for UI; they do not
		// mint a forecast. Cached republish of yesterday's calibration is not
		// an observation.
		if state.Replay {
			return true
		}

		analyzer.observe(thesis, state)

		return true
	})

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "observe", map[string]any{
		"states": len(states),
		"ns":     time.Since(observeStarted).Nanoseconds(),
	}))

	return states
}

/*
observe connects one manifold state to the existing resonance, causal, and
forecast outputs through the Thesis without making those components depend on
the manifold solver.
*/
func (analyzer *Analyzer) observe(
	thesis *types.Thesis,
	state manifold.State,
) {
	if analyzer.resonance == nil {
		analyzer.resonance = make(map[string]*Resonance)
	}

	if analyzer.causal == nil {
		analyzer.causal = make(map[string]*Causal)
	}

	resonance := analyzer.resonance[state.Symbol]

	if resonance == nil {
		resonance = NewResonance(state.Symbol, manifold.DefaultBaselineHalflife())

		if resonance == nil {
			return
		}

		analyzer.resonance[state.Symbol] = resonance
	}

	measurements, resonanceOutcome := resonance.Update(state)
	thesis.Measurements = append(thesis.Measurements, measurements...)

	if resonanceOutcome != nil {
		thesis.Resonance = append(thesis.Resonance, resonanceOutcome)
	}

	causal := analyzer.causal[state.Symbol]

	if causal == nil {
		causal = NewCausal(state.Symbol)
		analyzer.causal[state.Symbol] = causal
	}

	hypothesis, causalOutcome, err := causal.Update(state)

	if err != nil {
		errnie.Error(err)
		return
	}

	if causalOutcome != nil {
		thesis.Hypotheses = append(thesis.Hypotheses, hypothesis)
		thesis.Causal = append(thesis.Causal, causalOutcome)
	}

	analyzer.forecast(thesis, state, resonanceOutcome, causalOutcome)
}

/*
forecast appends a joint resonance+causal forecast when both heads are ready.
*/
func (analyzer *Analyzer) forecast(
	thesis *types.Thesis,
	state manifold.State,
	resonanceOutcome *ResonanceOutcome,
	causalOutcome *CausalOutcome,
) {
	if resonanceOutcome == nil || causalOutcome == nil ||
		!resonanceOutcome.ReturnReady ||
		resonanceOutcome.CalibrationSamples == 0 ||
		!causalOutcome.Ready {
		return
	}

	// Candidate notional is capped at the best ask, so the forecast does not
	// claim depth-crossing impact beyond the directly observed touch spread.
	forecast := types.Forecasts{
		Source:                     "resonance+causal",
		Symbol:                     state.Symbol,
		At:                         state.At,
		ObservedInterval:           state.Duration,
		SourceEpoch:                state.Epoch,
		HorizonEvents:              1,
		ExpiresEpoch:               state.Epoch + 1,
		Target:                     resonanceOutcome.Target,
		ModelVersion:               "resonance_return_head_v2_rls",
		Ready:                      true,
		Calibrated:                 true,
		CalibrationSamples:         resonanceOutcome.CalibrationSamples,
		IncrementalMSE:             resonanceOutcome.IncrementalMSE,
		IncrementalSkillLowerBound: resonanceOutcome.IncrementalSkillLowerBound,
		ExpectedReturn:             resonanceOutcome.ExpectedReturn,
		ReferencePrice:             state.ReferencePrice,
		BuyCapacity:                state.BuyCapacity,
		SellCapacity:               state.SellCapacity,
		ExpectedSpread:             state.Spread,
		ExpectedImpact:             analyzer.forecastImpact(state),
		ExpectedAdverseSelection:   analyzer.forecastAdverse(state),
		Uncertainty:                resonanceOutcome.Uncertainty,
		Confidence: math.Min(
			causalOutcome.Reading.Confidence,
			math.Exp(-math.Abs(resonanceOutcome.Surprise)),
		),
	}

	thesis.Forecasts = append(thesis.Forecasts, forecast)
}

/*
forecastImpact is temporary impact as capacity utilization times the observed
spread. Candidate size is capped at BuyCapacity, so the ratio is the share of
two-sided visible depth consumed by a full-touch entry.

ponytail: spread-times-utilization is a coarse friction proxy; upgrade path is
manifold gas-derived impact from calibrated depth consumption models.
*/
func (analyzer *Analyzer) forecastImpact(state manifold.State) float64 {
	depth := state.BuyCapacity + state.SellCapacity

	if state.BuyCapacity <= 0 || depth <= 0 || state.Spread < 0 {
		return 0
	}

	return state.Spread * (state.BuyCapacity / depth)
}

/*
forecastAdverse is informed-flow drag from manifold stress scaled by the
observed spread so adverse selection stays in return units.

ponytail: stress-anisotropy times spread is an interim adverse-selection
heuristic; upgrade path is causal-head informed-flow probability scaled by
observed touch cost.
*/
func (analyzer *Analyzer) forecastAdverse(state manifold.State) float64 {
	stress := math.Abs(state.StressAnisotropy)

	if stress <= 0 || state.Spread < 0 {
		return 0
	}

	return stress * state.Spread
}
