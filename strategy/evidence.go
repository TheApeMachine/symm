package strategy

import (
	"slices"
	"math"

	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
Project builds StopEvidence for one open holding from the current Thesis cut.
Missing mark or entry leaves Present false so Stoploss freezes last state
instead of inventing prices or zeroing floors through a nil frame.
*/
func Project(thesis *types.Thesis, holding types.Holding) types.StopEvidence {
	evidence := types.StopEvidence{Symbol: holding.Symbol}

	if holding.Mark == nil || holding.EntryPrice == nil {
		return evidence
	}

	mark := holding.Mark.Float64()
	entry := holding.EntryPrice.Float64()

	if mark <= 0 || entry <= 0 {
		return evidence
	}

	evidence.Mark = mark
	evidence.Entry = entry
	evidence.Present = true

	if thesis == nil {
		return evidence
	}

	for _, forecast := range thesis.Forecasts {
		if forecast.Symbol != holding.Symbol {
			continue
		}

		evidence.ForecastEpoch = forecast.SourceEpoch
		evidence.ExpectedReturn = forecast.ExpectedReturn
		evidence.Uncertainty = forecast.Uncertainty
		evidence.IncrementalMSE = forecast.IncrementalMSE
		evidence.ReturnReady = forecast.Ready && forecast.Calibrated
		evidence.Spread = forecast.ExpectedSpread
		evidence.SellCapacity = forecast.SellCapacity
		evidence.NormalizedResidual = normalizedResidual(
			evidence.IncrementalMSE, evidence.Uncertainty,
		)
	}

	for index := len(thesis.Resonance) - 1; index >= 0; index-- {
		outcome, ok := asResonance(thesis.Resonance[index])

		if !ok || outcome.Symbol != holding.Symbol {
			continue
		}

		evidence.ExpectedReturn = outcome.ExpectedReturn
		evidence.Uncertainty = outcome.Uncertainty
		evidence.IncrementalMSE = outcome.IncrementalMSE
		evidence.ReturnReady = outcome.ReturnReady
		evidence.NormalizedResidual = normalizedResidual(
			evidence.IncrementalMSE, evidence.Uncertainty,
		)
		break
	}

	for _, v := range slices.Backward(thesis.Causal) {
		outcome, ok := asCausal(v)

		if !ok || outcome.Symbol != holding.Symbol {
			continue
		}

		evidence.CausalReady = outcome.Ready
		evidence.CausalExpectedReturn = outcome.ExpectedReturn
		break
	}

	if value, found := thesis.Cognition.Load(holding.Symbol); found {
		cognition, ok := value.(types.Cognition)

		if ok {
			evidence.CognitionReady = cognition.Ready
			evidence.CognitionConfidence = cognition.Confidence
			evidence.CognitionWinner = cognition.Winner
			evidence.CognitionAmbiguous = cognition.Ambiguous
		}
	}

	if value, found := thesis.Manifold.Load(holding.Symbol); found {
		state, ok := value.(manifold.State)

		if ok && state.GasReady() {
			evidence.Spread = state.Spread / state.ReferencePrice
			evidence.SellCapacity = state.SellCapacity
		}

		if ok && state.Epoch > 0 {
			evidence.ForecastEpoch = state.Epoch
		}
	}

	return evidence
}

/*
normalizedResidual is sqrt(MSE) / σ so Stoploss sees a dimensionless residual
in return space rather than return-squared over return.
*/
func normalizedResidual(incrementalMSE, uncertainty float64) float64 {
	if uncertainty <= 0 || incrementalMSE <= 0 {
		return 0
	}

	return math.Sqrt(incrementalMSE) / uncertainty
}

/*
asResonance normalizes pointer or value ResonanceOutcome payloads from Thesis.
*/
func asResonance(value any) (logic.ResonanceOutcome, bool) {
	switch outcome := value.(type) {
	case *logic.ResonanceOutcome:
		if outcome == nil {
			return logic.ResonanceOutcome{}, false
		}

		return *outcome, true
	case logic.ResonanceOutcome:
		return outcome, true
	default:
		return logic.ResonanceOutcome{}, false
	}
}

/*
asCausal normalizes pointer or value CausalOutcome payloads from Thesis.
*/
func asCausal(value any) (logic.CausalOutcome, bool) {
	switch outcome := value.(type) {
	case *logic.CausalOutcome:
		if outcome == nil {
			return logic.CausalOutcome{}, false
		}

		return *outcome, true
	case logic.CausalOutcome:
		return outcome, true
	default:
		return logic.CausalOutcome{}, false
	}
}
