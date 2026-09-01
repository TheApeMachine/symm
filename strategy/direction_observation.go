package strategy

import (
	"fmt"

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

	for _, measurement := range envelope.SignalMeasurements() {
		if err := predictor.observeMeasurement(measurement); err != nil {
			return err
		}
	}

	for _, perspective := range envelope.Perspectives {
		if err := predictor.observePerspective(perspective); err != nil {
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

	if err := predictor.observeManifold(envelopeSymbol(envelope), envelope.Manifold); err != nil {
		return err
	}

	return predictor.observeResonance(envelope.Resonance)
}

func (predictor *directionalPredictor) observePerspective(perspective *types.Perspective) error {
	if perspective == nil {
		return nil
	}

	if perspective.Err != nil {
		return fmt.Errorf("strategy: perspective %s failed: %w", perspective.Kind, perspective.Err)
	}

	state, err := predictor.state(perspective.Symbol)

	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	for index := 0; index < perspective.Count; index++ {
		reading := perspective.Readings[index]

		if !reading.Defined {
			continue
		}

		metric, found := nmtypes.SymbolName(reading.Metric)

		if !found {
			return fmt.Errorf("strategy: advisor metric %d has no interned name", reading.Metric)
		}

		quality := reading.Maturity

		if reading.SNRDefined {
			quality *= reading.SNR / (1 + reading.SNR)
		}

		if err := state.observe(featureKey{
			family: "perspective",
			source: perspective.Kind.String(),
			metric: metric,
		}, reading.Value, quality); err != nil {
			return err
		}
	}

	return nil
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
	state.mu.Lock()
	defer state.mu.Unlock()

	source := string(category.Type)

	if err := state.observe(featureKey{family: "category", source: source, metric: "confidence"}, category.Confidence, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "category", source: source, metric: "surprisal"}, category.Surprisal, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "category", source: source, metric: "strength"}, category.Strength, quality); err != nil {
		return err
	}

	return state.observe(featureKey{family: "category", source: source, metric: "uncertainty"}, category.Uncertainty, quality)
}

func (predictor *directionalPredictor) observeOpportunity(candidate *types.OpportunityCandidate) error {
	if candidate == nil || candidate.Symbol == "" {
		return nil
	}

	state, err := predictor.state(candidate.Symbol)

	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	source := string(candidate.Archetype)
	quality := candidate.Maturity

	if err := state.observe(featureKey{family: "opportunity", source: source, metric: "direction"}, float64(candidate.Direction), quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "opportunity", source: source, metric: "provenance"}, float64(candidate.Provenance), quality); err != nil {
		return err
	}

	if candidate.Economics == nil || !candidate.Economics.Calibrated {
		return nil
	}

	if err := state.observe(featureKey{family: "opportunity", source: source, metric: "transition_probability"}, candidate.Economics.TransitionProbability, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "opportunity", source: source, metric: "profit_first"}, candidate.Economics.ProfitFirst, quality); err != nil {
		return err
	}

	return state.observe(featureKey{family: "opportunity", source: source, metric: "uncertainty"}, candidate.Economics.Uncertainty, quality)
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

	state.mu.Lock()
	defer state.mu.Unlock()

	quality := cognition.Confidence

	if err := state.observe(featureKey{family: "cognition", source: "state", metric: "class_confidence"}, cognition.ClassConfidence, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "cognition", source: "state", metric: "contrast"}, cognition.Contrast, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "cognition", source: "state", metric: "contrast_evidence"}, cognition.ContrastEvidence, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "cognition", source: "state", metric: "lookahead_score"}, cognition.LookaheadScore, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "cognition", source: "state", metric: "interpolated_surprisal"}, cognition.InterpolatedSurprisal, quality); err != nil {
		return err
	}

	for _, class := range cognition.Classes {
		if err := state.observe(featureKey{family: "cognition", source: "class", metric: class.Name}, class.Probability, quality); err != nil {
			return err
		}
	}

	for _, contribution := range cognition.Contributions {
		if err := state.observe(featureKey{family: "cognition", source: "contribution", metric: contribution.Token}, contribution.Bits, quality); err != nil {
			return err
		}
	}

	return nil
}

func (predictor *directionalPredictor) observeManifold(symbol string, manifold *types.ManifoldState) error {
	if manifold == nil || symbol == "" {
		return nil
	}

	state, err := predictor.state(symbol)

	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

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
		if err := state.observe(reading.key, reading.value, 1); err != nil {
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

	state, err := predictor.state(artifact.Symbol)

	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	quality := artifact.Confidence

	for symbol, value := range artifact.Dynamics.All() {
		metric, found := nmtypes.SymbolName(symbol)

		if !found {
			return fmt.Errorf("strategy: resonance metric %d has no interned name", symbol)
		}

		if err := state.observe(featureKey{family: "resonance", source: "dynamics", metric: metric}, value, quality); err != nil {
			return err
		}
	}

	if err := state.observe(featureKey{family: "resonance", source: "calibration", metric: "confidence"}, artifact.Confidence, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "resonance", source: "calibration", metric: "resolution_target"}, artifact.LastResolutionTarget, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "resonance", source: "calibration", metric: "resolution_error"}, artifact.LastResolutionError, quality); err != nil {
		return err
	}

	if artifact.Forecast == nil {
		return nil
	}

	if err := state.observe(featureKey{family: "resonance", source: "forecast", metric: "direction"}, artifact.Forecast.Call, quality); err != nil {
		return err
	}

	if err := state.observe(featureKey{family: "resonance", source: "forecast", metric: "location"}, artifact.Forecast.Distribution.Value, quality); err != nil {
		return err
	}

	return state.observe(featureKey{family: "resonance", source: "forecast", metric: "scale"}, artifact.Forecast.Distribution.Scale, quality)
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
