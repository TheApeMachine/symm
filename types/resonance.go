package types

import (
	"errors"
	"math"
	"slices"
	"time"

	"github.com/theapemachine/nomagique/learning"
)

const basisPointsPerUnit = 10_000

/*
ResonanceForecast is the one return forecast supported by a resonance reading.

Curve contains one-step log-return predictions over SupportedHorizon. Retention
states how much of the first forecast state's magnitude survives at each step.
ExpectedReturn is the simple return obtained by accumulating every supported
step after applying that retention. Confidence is the posterior probability that
the first resolved return has the point forecast's direction. PredictiveScale and
DegreesOfFreedom identify the Student-t distribution behind that probability.
*/
type ResonanceForecast struct {
	Curve                      []float64 `json:"forwardCurve"`
	Retention                  []float64 `json:"forwardRetention"`
	SupportedHorizon           int       `json:"supportedHorizon"`
	ExpectedReturn             float64   `json:"expectedReturn"`
	ExpectedBasisPoints        float64   `json:"expectedBasisPoints"`
	Confidence                 float64   `json:"confidence"`
	ConfidenceReady            bool      `json:"confidenceReady"`
	PredictiveScale            float64   `json:"predictiveScale,omitempty"`
	PredictiveScaleBasisPoints float64   `json:"predictiveScaleBasisPoints,omitempty"`
	DegreesOfFreedom           float64   `json:"degreesOfFreedom,omitempty"`
}

/*
ResonanceVerdict is the plain-language reading of a resonance frame.

The panel answers three questions before any number is read: is a forecast
available, which estimator learns its return head, and which way does the curve
point. Each is decided here rather than recomputed in the browser.
*/
type ResonanceVerdict struct {
	/* "observing" | "predicting". */
	Learning string `json:"learning"`
	/* The online estimator used by the supervised return head. */
	Tuning string `json:"tuning"`
	/*
		Each label's health as nominal (1), attention (0), or broken (-1), so the
		panel tones a verdict without keeping a second copy of the label set.
	*/
	LearningHealth float64 `json:"learningHealth"`
	TuningHealth   float64 `json:"tuningHealth"`
	/*
		Sign of the expected return and the posterior probability of that direction.
		Direction turns the arrow; Conviction dims it when poorly supported.
	*/
	Direction  float64 `json:"direction"`
	Conviction float64 `json:"conviction"`
}

/*
ResonanceReading is one predictive-coding result for a symbol. Forecast remains
nil only when the settled latent state cannot support a forward path; an
informative state publishes the zero-coefficient prior before any target resolves.

	SkillEvidence is historical prequential evidence that the model beats a zero-return
	baseline more than half the time; it is deliberately separate from the current
	forecast's Confidence. Alpha is the configured generative-model base pace, not

the return learner's gain and not a dynamically tuned value.
*/
type ResonanceReading struct {
	EvidenceRevision uint64                        `json:"evidenceRevision"`
	Stage            string                        `json:"stage"`
	Source           SourceType                    `json:"source"`
	Symbol           string                        `json:"symbol"`
	TargetSymbol     string                        `json:"targetSymbol"`
	At               time.Time                     `json:"at"`
	Surprise         float64                       `json:"surprise"`
	Energy           float64                       `json:"energy"`
	Latent           []float64                     `json:"latent,omitempty"`
	Embedding        []float64                     `json:"embedding,omitempty"`
	Layers           []learning.ResonanceLayerWire `json:"layers,omitempty"`
	Forecast         *ResonanceForecast            `json:"forecast,omitempty"`
	Verdict          ResonanceVerdict              `json:"verdict"`
	Alpha            float64                       `json:"alpha"`
	Samples          uint64                        `json:"samples"`
	SkillEvidence    float64                       `json:"skillEvidence"`
}

/*
NewResonanceForecast validates and owns a confidence-supported forecast curve.
It refuses incomplete curve/retention pairs rather than assigning unsupported
steps unit retention.
*/
func NewResonanceForecast(
	curve, retention []float64,
	supportedHorizon int,
	confidence float64,
) (*ResonanceForecast, error) {
	forecast := &ResonanceForecast{
		Curve:            slices.Clone(curve),
		Retention:        slices.Clone(retention),
		SupportedHorizon: supportedHorizon,
		Confidence:       confidence,
	}

	expectedReturn, err := forecast.calculateExpectedReturn()

	if err != nil {
		return nil, err
	}

	forecast.ExpectedReturn = expectedReturn
	forecast.ExpectedBasisPoints = expectedReturn * basisPointsPerUnit

	return forecast, nil
}

/*
SetPredictiveDistribution attaches the uncertainty distribution used to derive
Confidence. An unavailable distribution must remain structurally absent instead
of acquiring an invented scale.
*/
func (forecast *ResonanceForecast) SetPredictiveDistribution(
	scale, degreesOfFreedom float64,
	ready bool,
) error {
	if forecast == nil {
		return errors.New("resonance forecast required")
	}

	if !ready {
		if scale != 0 || degreesOfFreedom != 0 {
			return errors.New("unready resonance forecast cannot carry a distribution")
		}

		forecast.ConfidenceReady = false
		forecast.PredictiveScale = 0
		forecast.PredictiveScaleBasisPoints = 0
		forecast.DegreesOfFreedom = 0

		return nil
	}

	if !(scale > 0) || math.IsNaN(scale) || math.IsInf(scale, 0) ||
		!(degreesOfFreedom > 0) || math.IsNaN(degreesOfFreedom) ||
		math.IsInf(degreesOfFreedom, 0) {
		return errors.New("resonance forecast distribution must be finite and positive")
	}

	forecast.ConfidenceReady = true
	forecast.PredictiveScale = scale
	forecast.PredictiveScaleBasisPoints = scale * basisPointsPerUnit
	forecast.DegreesOfFreedom = degreesOfFreedom

	return nil
}

/*
Validate confirms that a published forecast still carries the exact horizon
and return implied by its curve. Consumers use this before decision math so a
malformed external value remains unavailable instead of acquiring defaults.
*/
func (forecast *ResonanceForecast) Validate() error {
	expectedReturn, err := forecast.calculateExpectedReturn()

	if err != nil {
		return err
	}

	if forecast.ExpectedReturn != expectedReturn {
		return errors.New("resonance forecast expected return does not match curve")
	}

	if forecast.ExpectedBasisPoints != expectedReturn*basisPointsPerUnit {
		return errors.New("resonance forecast basis points do not match expected return")
	}

	if forecast.ConfidenceReady {
		if !(forecast.PredictiveScale > 0) ||
			math.IsNaN(forecast.PredictiveScale) ||
			math.IsInf(forecast.PredictiveScale, 0) ||
			!(forecast.DegreesOfFreedom > 0) ||
			math.IsNaN(forecast.DegreesOfFreedom) ||
			math.IsInf(forecast.DegreesOfFreedom, 0) {
			return errors.New("resonance forecast distribution is invalid")
		}

		if forecast.PredictiveScaleBasisPoints !=
			forecast.PredictiveScale*basisPointsPerUnit {
			return errors.New("resonance forecast scale basis points do not match scale")
		}

		return nil
	}

	if forecast.PredictiveScale != 0 || forecast.PredictiveScaleBasisPoints != 0 ||
		forecast.DegreesOfFreedom != 0 {
		return errors.New("unready resonance forecast cannot carry a distribution")
	}

	return nil
}

/*
Step returns one retention-supported log-return prediction. A valid numerical
zero remains present; false means the requested step has no valid forecast.
*/
func (forecast *ResonanceForecast) Step(index int) (float64, bool) {
	if forecast == nil || index < 0 || index >= forecast.SupportedHorizon ||
		len(forecast.Curve) != forecast.SupportedHorizon ||
		len(forecast.Retention) != forecast.SupportedHorizon {
		return 0, false
	}

	reference := forecast.Retention[0]
	surviving := forecast.Retention[index]

	if reference <= 0 || surviving <= 0 ||
		math.IsNaN(reference) || math.IsInf(reference, 0) ||
		math.IsNaN(surviving) || math.IsInf(surviving, 0) {
		return 0, false
	}

	step := forecast.Curve[index]

	if math.IsNaN(step) || math.IsInf(step, 0) {
		return 0, false
	}

	return step * math.Min(1, surviving/reference), true
}

/*
WorstIntermediateDrawdown returns the deepest predicted loss from the current
price anywhere on the supported path, as a positive simple-return fraction.

Each Step is already a retention-adjusted log return. Adding the steps produces
the cumulative log-price path; converting its lowest point with Expm1 preserves
the return units used by ExpectedReturn. A path that never falls below its
starting price has no expected drawdown and returns zero.
*/
func (forecast *ResonanceForecast) WorstIntermediateDrawdown() (float64, error) {
	if err := forecast.Validate(); err != nil {
		return 0, err
	}

	cumulative := 0.0
	worst := 0.0

	for index := range forecast.SupportedHorizon {
		step, present := forecast.Step(index)

		if !present {
			return 0, errors.New("resonance forecast path contains an unsupported step")
		}

		cumulative += step

		if cumulative < worst {
			worst = cumulative
		}
	}

	return -math.Expm1(worst), nil
}

func (forecast *ResonanceForecast) calculateExpectedReturn() (float64, error) {
	if forecast == nil {
		return 0, errors.New("resonance forecast required")
	}

	if forecast.SupportedHorizon <= 0 ||
		len(forecast.Curve) != forecast.SupportedHorizon ||
		len(forecast.Retention) != forecast.SupportedHorizon {
		return 0, errors.New("resonance forecast horizon does not match curve and retention")
	}

	if math.IsNaN(forecast.Confidence) || math.IsInf(forecast.Confidence, 0) ||
		forecast.Confidence < 0 || forecast.Confidence > 1 {
		return 0, errors.New("resonance forecast confidence must be finite on [0,1]")
	}

	accumulated := 0.0

	for index := range forecast.SupportedHorizon {
		step, ok := forecast.Step(index)

		if !ok {
			return 0, errors.New("resonance forecast contains an invalid supported step")
		}

		accumulated += step
	}

	expectedReturn := math.Expm1(accumulated)

	if math.IsNaN(expectedReturn) || math.IsInf(expectedReturn, 0) {
		return 0, errors.New("resonance forecast expected return is non-finite")
	}

	return expectedReturn, nil
}
