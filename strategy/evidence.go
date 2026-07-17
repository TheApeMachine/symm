package strategy

import (
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
Evidence is the thin numeric projection Stoploss consumes from one Thesis.
It collapses logic-layer outputs onto the scalars the stop actually needs so
exit regulation stays independent of signal vocabulary and named regimes.
*/
type Evidence struct {
	Symbol               string
	Mark                 float64
	Entry                float64
	ExpectedReturn       float64
	Uncertainty          float64
	IncrementalMSE       float64
	ReturnReady          bool
	CausalReady          bool
	CausalExpectedReturn float64
	CognitionReady       bool
	CognitionConfidence  float64
	CognitionWinner      string
	CognitionAmbiguous   bool
	Spread               float64
	SellCapacity         float64
	Present              bool
}

/*
Project builds Evidence for one open holding from the current Thesis cut.
Missing mark or entry leaves Present false so Stoploss freezes last state
instead of inventing prices or zeroing floors through a nil frame.
*/
func Project(thesis *types.Thesis, holding types.Holding) Evidence {
	evidence := Evidence{Symbol: holding.Symbol}

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

	for _, forecast := range thesis.Forecasts {
		if forecast.Symbol != holding.Symbol {
			continue
		}

		evidence.ExpectedReturn = forecast.ExpectedReturn
		evidence.Uncertainty = forecast.Uncertainty
		evidence.IncrementalMSE = forecast.IncrementalMSE
		evidence.ReturnReady = forecast.Ready && forecast.Calibrated
		evidence.Spread = forecast.ExpectedSpread
		evidence.SellCapacity = forecast.SellCapacity
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
		break
	}

	for index := len(thesis.Causal) - 1; index >= 0; index-- {
		outcome, ok := asCausal(thesis.Causal[index])

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
	}

	return evidence
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
