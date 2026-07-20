package strategy

import (
	"math"
	"slices"

	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
Evidence projects Thesis cuts onto StopEvidence for Stoploss.Regulate. Missing
mark or entry leaves Present false so the regulator freezes instead of inventing
prices.
*/
type Evidence struct{}

/*
NewEvidence returns the stop-evidence projector.
*/
func NewEvidence() Evidence {
	return Evidence{}
}

/*
Project builds StopEvidence for one open holding from the current Thesis cut.
*/
func (evidence Evidence) Project(
	thesis *types.Thesis,
	holding types.Holding,
) types.StopEvidence {
	projected := types.StopEvidence{Symbol: holding.Symbol}

	if holding.EntryPrice == nil {
		return projected
	}

	// StopMark (mid/last) only — bid Mark is flatten-now PnL, not stop geometry.
	markDecimal := holding.StopMark

	if markDecimal == nil {
		return projected
	}

	mark := markDecimal.Float64()
	entry := holding.EntryPrice.Float64()

	if mark <= 0 || entry <= 0 {
		return projected
	}

	projected.Mark = mark
	projected.Entry = entry
	projected.ReferencePrice = markDecimal.Copy()
	projected.Present = true

	if thesis == nil {
		return projected
	}

	evidence.forecast(&projected, thesis, holding.Symbol)
	evidence.resonance(&projected, thesis, holding.Symbol)
	evidence.causal(&projected, thesis, holding.Symbol)
	evidence.cognition(&projected, thesis, holding.Symbol)
	evidence.manifold(&projected, thesis, holding.Symbol)
	evidence.retreat(&projected, thesis, holding.Symbol)

	return projected
}

func (evidence Evidence) forecast(
	projected *types.StopEvidence,
	thesis *types.Thesis,
	symbol string,
) {
	forecast, found := selectForecast(thesis.Forecasts, symbol)

	if !found {
		return
	}

	projected.ForecastEpoch = forecast.SourceEpoch
	projected.ExpectedReturn = forecast.ExpectedReturn
	projected.Uncertainty = forecast.Uncertainty
	projected.IncrementalMSE = forecast.IncrementalMSE
	projected.ReturnReady = forecast.Ready && forecast.Calibrated
	projected.Spread = forecast.ExpectedSpread
	projected.SellCapacity = forecast.SellCapacity
	projected.NormalizedResidual = evidence.residual(
		projected.IncrementalMSE, projected.Uncertainty,
	)
}

func (evidence Evidence) resonance(
	projected *types.StopEvidence,
	thesis *types.Thesis,
	symbol string,
) {
	for index := len(thesis.Resonance) - 1; index >= 0; index-- {
		outcome, ok := evidence.asResonance(thesis.Resonance[index])

		if !ok || outcome.Symbol != symbol {
			continue
		}

		projected.ExpectedReturn = outcome.ExpectedReturn
		projected.Uncertainty = outcome.Uncertainty
		projected.IncrementalMSE = outcome.IncrementalMSE
		projected.ReturnReady = outcome.ReturnReady
		projected.NormalizedResidual = evidence.residual(
			projected.IncrementalMSE, projected.Uncertainty,
		)
		return
	}
}

func (evidence Evidence) causal(
	projected *types.StopEvidence,
	thesis *types.Thesis,
	symbol string,
) {
	for _, value := range slices.Backward(thesis.Causal) {
		outcome, ok := evidence.asCausal(value)

		if !ok || outcome.Symbol != symbol {
			continue
		}

		projected.CausalReady = outcome.Ready
		projected.CausalExpectedReturn = outcome.ExpectedReturn
		return
	}
}

func (evidence Evidence) cognition(
	projected *types.StopEvidence,
	thesis *types.Thesis,
	symbol string,
) {
	value, found := thesis.Cognition.Load(symbol)

	if !found {
		return
	}

	cognition, ok := value.(types.Cognition)

	if !ok {
		return
	}

	projected.CognitionReady = cognition.Ready
	projected.CognitionConfidence = cognition.Confidence
	projected.CognitionWinner = cognition.Winner
	projected.CognitionAmbiguous = cognition.Ambiguous
}

func (evidence Evidence) manifold(
	projected *types.StopEvidence,
	thesis *types.Thesis,
	symbol string,
) {
	value, found := thesis.Manifold.Load(symbol)

	if !found {
		return
	}

	state, ok := value.(manifold.State)

	if !ok {
		return
	}

	if state.GasReady() {
		// Manifold spread is already return-space ((ask-bid)/mid); do not
		// divide by ReferencePrice again or LiveScale undercuts thin books.
		projected.Spread = state.Spread
		projected.SellCapacity = state.SellCapacity
	}

	if state.Epoch > 0 {
		projected.ForecastEpoch = state.Epoch
	}
}

func (evidence Evidence) retreat(
	projected *types.StopEvidence,
	thesis *types.Thesis,
	symbol string,
) {
	for _, measurement := range thesis.Measurements {
		if measurement == nil ||
			measurement.Symbol != symbol ||
			measurement.Metric != types.MetricRetreatingQuantity ||
			measurement.Normalized == nil {
			continue
		}

		projected.RetreatReady = true

		if *measurement.Normalized > projected.RetreatPressure {
			projected.RetreatPressure = *measurement.Normalized
		}
	}
}

func (evidence Evidence) residual(incrementalMSE, uncertainty float64) float64 {
	if uncertainty <= 0 || incrementalMSE <= 0 {
		return 0
	}

	return math.Sqrt(incrementalMSE) / uncertainty
}

func (evidence Evidence) asResonance(value any) (logic.ResonanceOutcome, bool) {
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

func (evidence Evidence) asCausal(value any) (logic.CausalOutcome, bool) {
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
