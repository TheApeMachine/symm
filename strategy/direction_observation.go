package strategy

import (
	"fmt"
	"time"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Observe consumes every populated analytical surface on one envelope. Each fact
keeps its own identity and is learned independently; no hand-authored score
decides in advance which market fact deserves to matter.
*/
func (predictor *directionalPredictor) observe(envelope *types.Envelope) error {
	if envelope == nil {
		return nil
	}

	if envelope.TypeID == types.EnvelopeTicker {
		if envelope.Resonance == nil {
			return fmt.Errorf(
				"strategy: ticker %s requires its resonance artifact",
				envelope.TickerData.Symbol,
			)
		}

		if envelope.Resonance.Symbol != envelope.TickerData.Symbol ||
			!envelope.Resonance.At.Equal(envelope.TickerData.Timestamp) {
			return fmt.Errorf(
				"strategy: ticker %s requires matching resonance identity and event time",
				envelope.TickerData.Symbol,
			)
		}
	}

	for _, measurement := range envelope.SignalMeasurements() {
		if err := predictor.observeMeasurement(measurement); err != nil {
			return err
		}
	}

	for index := range envelope.Categories {
		if err := predictor.observeCategory(&envelope.Categories[index]); err != nil {
			return err
		}
	}

	for _, candidate := range envelope.Opportunities {
		if err := predictor.observeOpportunity(candidate); err != nil {
			return err
		}
	}

	if err := predictor.observeCognition(envelope.Cognition); err != nil {
		return err
	}

	if err := predictor.observeManifold(
		envelopeSymbol(envelope),
		envelopeTime(envelope),
		envelope.Manifold,
	); err != nil {
		return err
	}

	return predictor.observeResonance(envelope.Resonance)
}

func (predictor *directionalPredictor) observeCategory(category *types.Category) error {
	if category == nil || category.Symbol == "" || category.Type == types.CategoryTypeNone {
		return nil
	}

	state, err := predictor.state(category.Symbol)

	if err != nil {
		return err
	}

	quality := category.Maturity * category.Freshness
	source := string(category.Type)

	if err := state.observe(
		featureKey{family: "category", source: source, metric: "confidence"},
		category.Confidence, quality, category.At, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "category", source: source, metric: "surprisal"},
		category.Surprisal, quality, category.At, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "category", source: source, metric: "strength"},
		category.Strength, quality, category.At, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "category", source: source, metric: "uncertainty"},
		category.Uncertainty, quality, category.At, featureEstimability,
	); err != nil {
		return err
	}

	return nil
}

func (predictor *directionalPredictor) observeOpportunity(candidate *types.OpportunityCandidate) error {
	if candidate == nil || candidate.Symbol == "" {
		return nil
	}

	state, err := predictor.state(candidate.Symbol)

	if err != nil {
		return err
	}

	source := string(candidate.Archetype)
	quality := candidate.Maturity

	if err := state.observe(
		featureKey{family: "opportunity", source: source, metric: "direction"},
		float64(candidate.Direction), quality, candidate.Updated, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "opportunity", source: source, metric: "provenance"},
		float64(candidate.Provenance), quality, candidate.Updated, featureEstimability,
	); err != nil {
		return err
	}

	if candidate.Economics == nil || !candidate.Economics.Calibrated {
		state.opportunity = *candidate

		return nil
	}

	if err := state.observe(
		featureKey{family: "opportunity", source: source, metric: "transition_probability"},
		candidate.Economics.TransitionProbability,
		quality, candidate.Updated, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "opportunity", source: source, metric: "profit_first"},
		candidate.Economics.ProfitFirst,
		quality, candidate.Updated, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "opportunity", source: source, metric: "uncertainty"},
		candidate.Economics.Uncertainty,
		quality, candidate.Updated, featureEstimability,
	); err != nil {
		return err
	}

	state.opportunity = *candidate

	return nil
}

func (predictor *directionalPredictor) observeCognition(cognition *types.Cognition) error {
	if cognition == nil || cognition.Symbol == "" {
		return nil
	}

	if cognition.Error != "" {
		return fmt.Errorf("strategy: cognition %s failed: %s", cognition.Symbol, cognition.Error)
	}

	state, err := predictor.state(cognition.Symbol)

	if err != nil {
		return err
	}

	quality := cognition.Confidence

	if err := state.observe(
		featureKey{family: "cognition", source: "state", metric: "class_confidence"},
		cognition.ClassConfidence, quality, cognition.At, featureEstimability,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "cognition", source: "state", metric: "contrast"},
		cognition.Contrast, quality, cognition.At, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "cognition", source: "state", metric: "contrast_evidence"},
		cognition.ContrastEvidence, quality, cognition.At, featureEstimability,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "cognition", source: "state", metric: "lookahead_score"},
		cognition.LookaheadScore, quality, cognition.At, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "cognition", source: "state", metric: "interpolated_surprisal"},
		cognition.InterpolatedSurprisal,
		quality, cognition.At, featureEstimability,
	); err != nil {
		return err
	}

	for _, class := range cognition.Classes {
		if err := state.observe(
			featureKey{family: "cognition", source: "class", metric: class.Name},
			class.Probability, quality, cognition.At, featureContext,
		); err != nil {
			return err
		}
	}

	for _, contribution := range cognition.Contributions {
		if err := state.observe(
			featureKey{
				family: "cognition", source: "contribution", metric: contribution.Token,
			},
			contribution.Bits, quality, cognition.At, featureContext,
		); err != nil {
			return err
		}
	}

	return nil
}

func (predictor *directionalPredictor) observeManifold(
	symbol string,
	at time.Time,
	manifold *types.ManifoldState,
) error {
	if manifold == nil || symbol == "" {
		return nil
	}

	state, err := predictor.state(symbol)

	if err != nil {
		return err
	}

	readings := []struct {
		key   featureKey
		value float64
	}{
		{featureKey{family: "manifold", source: "reading", metric: "divergence"}, manifold.Divergence},
		{featureKey{family: "manifold", source: "reading", metric: "guidance_speed"}, manifold.GuidanceSpeed},
		{featureKey{family: "manifold", source: "reading", metric: "coherence_magnitude_squared"}, manifold.CoherenceMag2},
		{featureKey{family: "manifold", source: "reading", metric: "pressure_gradient_norm"}, manifold.PressureGradNorm},
		{featureKey{family: "manifold", source: "reading", metric: "viscosity_proxy"}, manifold.ViscosityProxy},
		{featureKey{family: "manifold", source: "reading", metric: "kuramoto_order"}, manifold.KuramotoR},
		{featureKey{family: "manifold", source: "scale", metric: "density"}, float64(manifold.DensityScale)},
		{featureKey{family: "manifold", source: "scale", metric: "momentum"}, float64(manifold.MomentumScale)},
		{featureKey{family: "manifold", source: "scale", metric: "energy"}, float64(manifold.EnergyScale)},
		{featureKey{family: "manifold", source: "scale", metric: "wave"}, float64(manifold.WaveScale)},
	}

	for _, reading := range readings {
		if err := state.observe(reading.key, reading.value, 1, at, featureContext); err != nil {
			return err
		}
	}

	return nil
}

func (predictor *directionalPredictor) observeResonance(artifact *types.ResonanceArtifact) error {
	if artifact == nil || artifact.Symbol == "" {
		return nil
	}

	if artifact.Dynamics.Err != nil {
		return fmt.Errorf("strategy: resonance %s failed: %w", artifact.Symbol, artifact.Dynamics.Err)
	}

	if artifact.At.IsZero() {
		return fmt.Errorf("strategy: resonance %s requires event time", artifact.Symbol)
	}

	state, err := predictor.state(artifact.Symbol)

	if err != nil {
		return err
	}

	state.horizonSteps = 0

	if artifact.Calibrated {
		if artifact.SupportedHorizon <= 0 {
			return fmt.Errorf(
				"strategy: calibrated resonance %s requires a positive supported horizon",
				artifact.Symbol,
			)
		}

		state.horizonSteps = artifact.SupportedHorizon
	}

	quality := artifact.Confidence

	for symbol, value := range artifact.Dynamics.All() {
		metric, found := nmtypes.SymbolName(symbol)

		if !found {
			return fmt.Errorf("strategy: resonance metric %d has no interned name", symbol)
		}

		if err := state.observe(
			featureKey{family: "resonance", source: "dynamics", metric: metric},
			value, quality, artifact.At, featureContext,
		); err != nil {
			return err
		}
	}

	if err := state.observe(
		featureKey{family: "resonance", source: "calibration", metric: "confidence"},
		artifact.Confidence, quality, artifact.At, featureEstimability,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{
			family: "resonance", source: "calibration", metric: "resolution_target",
		},
		artifact.LastResolutionTarget, quality, artifact.At, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{
			family: "resonance", source: "calibration", metric: "resolution_error",
		},
		artifact.LastResolutionError, quality, artifact.At, featureEstimability,
	); err != nil {
		return err
	}

	if artifact.Forecast == nil {
		return nil
	}

	if err := state.observe(
		featureKey{family: "resonance", source: "forecast", metric: "direction"},
		artifact.Forecast.Call, quality, artifact.At, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "resonance", source: "forecast", metric: "location"},
		artifact.Forecast.Distribution.Value,
		quality, artifact.At, featureContext,
	); err != nil {
		return err
	}

	if err := state.observe(
		featureKey{family: "resonance", source: "forecast", metric: "scale"},
		artifact.Forecast.Distribution.Scale,
		quality, artifact.At, featureEstimability,
	); err != nil {
		return err
	}

	return nil
}

func envelopeSymbol(envelope *types.Envelope) string {
	switch envelope.TypeID {
	case types.EnvelopeTicker:
		return envelope.TickerData.Symbol
	case types.EnvelopeTrade:
		return envelope.TradeData.Symbol
	case types.EnvelopeLevel3:
		return envelope.Level3Data.Symbol
	case types.EnvelopeExecution:
		return envelope.ExecutionData.Symbol
	case types.EnvelopeFuturesTicker:
		return envelope.FuturesTickerData.Symbol
	case types.EnvelopeFuturesTrade:
		return envelope.FuturesTradeData.Symbol
	default:
		return ""
	}
}

func envelopeTime(envelope *types.Envelope) time.Time {
	switch envelope.TypeID {
	case types.EnvelopeTicker:
		return envelope.TickerData.Timestamp
	case types.EnvelopeTrade:
		return envelope.TradeData.Timestamp
	case types.EnvelopeLevel3:
		return envelope.Level3Data.Timestamp
	case types.EnvelopeExecution:
		return envelope.ExecutionData.Timestamp
	case types.EnvelopeFuturesTicker:
		return envelope.FuturesTickerData.Timestamp
	case types.EnvelopeFuturesTrade:
		return envelope.FuturesTradeData.Timestamp
	default:
		return time.Time{}
	}
}
