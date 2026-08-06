package types

import (
	"errors"
	"math"
	"slices"
	"time"
)

/*
ResonanceForecast is the one return forecast supported by a resonance reading.

Curve contains one-step log-return predictions over SupportedHorizon. Retention
states how much of the first forecast state's magnitude survives at each step.
ExpectedReturn is the simple return obtained by accumulating every supported
step after applying that retention. Confidence has already capped the horizon;
it is not a second multiplier on ExpectedReturn.
*/
type ResonanceForecast struct {
	Curve            []float64 `json:"forwardCurve"`
	Retention        []float64 `json:"forwardRetention"`
	SupportedHorizon int       `json:"supportedHorizon"`
	ExpectedReturn   float64   `json:"expectedReturn"`
	Confidence       float64   `json:"confidence"`
}

/*
ResonanceReading is one predictive-coding result for a symbol. Forecast remains
nil until the task head has enough resolved market data to publish a curve, so
absence cannot be confused with a numerical forecast of zero.
*/
type ResonanceReading struct {
	Stage        string             `json:"stage"`
	Source       SourceType         `json:"source"`
	Symbol       string             `json:"symbol"`
	TargetSymbol string             `json:"targetSymbol"`
	At           time.Time          `json:"at"`
	Surprise     float64            `json:"surprise"`
	Energy       float64            `json:"energy"`
	Latent       []float64          `json:"latent,omitempty"`
	Forecast     *ResonanceForecast `json:"forecast,omitempty"`
	Alpha        float64            `json:"alpha"`
	Samples      uint64             `json:"samples"`
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

	return forecast, nil
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
