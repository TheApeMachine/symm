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
		!causalOutcome.Ready || causalOutcome.CalibrationSamples == 0 {
		return
	}

	// Candidate notional is capped at the best ask, so the forecast does not
	// claim depth-crossing impact beyond the directly observed touch spread.
	thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
		Source:           "resonance+causal",
		Symbol:           state.Symbol,
		At:               state.At,
		ObservedInterval: state.Duration,
		SourceEpoch:      state.Epoch,
		HorizonEvents:    1,
		ExpiresEpoch:     state.Epoch + 1,
		Target:           resonanceOutcome.Target,
		ModelVersion:     "resonance_return_head_v1",
		Ready:            true,
		Calibrated:       true,
		CalibrationSamples: min(
			resonanceOutcome.CalibrationSamples,
			causalOutcome.CalibrationSamples,
		),
		IncrementalMSE:           resonanceOutcome.IncrementalMSE,
		IncrementalMSELowerBound: 0,
		ExpectedReturn:           resonanceOutcome.ExpectedReturn,
		ReferencePrice:           state.ReferencePrice,
		BuyCapacity:              state.BuyCapacity,
		SellCapacity:             state.SellCapacity,
		ExpectedSpread:           state.Spread,
		Uncertainty:              resonanceOutcome.Uncertainty,
		Confidence: math.Min(
			causalOutcome.Reading.Confidence,
			1/(1+resonanceOutcome.Surprise),
		),
	})
}
