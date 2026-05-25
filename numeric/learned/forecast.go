package learned

import (
	"errors"

	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	forecastAlphaFloor   = 0.05
	forecastAlphaCeiling = 1.0
)

/*
Forecast learns a multiplicative scale from settled predicted-vs-actual outcomes.
Weight tracks how wrong recent forecasts have been and modulates the learning rate:
large surprise moves the scale faster, stable accuracy barely moves it.
Forecast implements numeric.Dynamic for pipeline composition and exposes Scale for
parameter feedback into signal internals.
*/
type Forecast struct {
	ratio  adaptive.AlphaEMA
	weight *Weight
}

/*
NewForecast creates a learner with trust-modulated updates.
*/
func NewForecast(rate float64) *Forecast {
	if rate <= 0 {
		rate = 0.35
	}

	return &Forecast{
		weight: NewWeight(rate),
	}
}

/*
Next ingests predicted and actual values and returns the live scale.
When fewer than two values are supplied it returns the current scale unchanged.
*/
func (forecast *Forecast) Next(out float64, values ...float64) (float64, error) {
	if forecast == nil {
		return 0, errors.New("learned: Forecast.Next nil receiver")
	}

	if len(values) < 2 {
		return forecast.Scale(), nil
	}

	predicted := values[0]
	actual := values[1]

	sample, ok := SampleRatio(predicted, actual)

	if !ok {
		return forecast.Scale(), nil
	}

	trust, err := forecast.weight.Next(out, predicted, actual)

	if err != nil {
		return 0, err
	}

	alpha := forecastAlphaFloor + (1-trust)*(forecastAlphaCeiling-forecastAlphaFloor)

	if err := forecast.absorbSample(sample, alpha); err != nil {
		return 0, err
	}

	return forecast.Scale(), nil
}

/*
Absorb folds one calibration sample using an explicit smoothing alpha in (0, 1].
Use this when an outer scheduler already derives alpha from runway or half-life.
*/
func (forecast *Forecast) Absorb(predicted, actual, alpha float64) error {
	if forecast == nil {
		return errors.New("learned: Forecast.Absorb nil receiver")
	}

	sample, ok := SampleRatio(predicted, actual)

	if !ok {
		return nil
	}

	if _, err := forecast.weight.Next(0, predicted, actual); err != nil {
		return err
	}

	return forecast.absorbSample(sample, alpha)
}

/*
Scale returns the live parameter multiplier. Before the first sample it is one.
*/
func (forecast *Forecast) Scale() float64 {
	if forecast == nil || forecast.ratio.Updates() == 0 {
		return 1
	}

	value := forecast.ratio.Value()

	if value <= 0 {
		return 0
	}

	return value
}

/*
Trust returns the current forecast-trust level in [0, 1].
*/
func (forecast *Forecast) Trust() float64 {
	if forecast == nil || forecast.weight == nil {
		return 0
	}

	trust, _ := forecast.weight.Next(0)

	return trust
}

/*
Updates counts settled forecasts absorbed into the scale learner.
*/
func (forecast *Forecast) Updates() int {
	if forecast == nil {
		return 0
	}

	return forecast.ratio.Updates()
}

/*
Reset clears ratio and trust state.
*/
func (forecast *Forecast) Reset() error {
	if forecast == nil {
		return errors.New("learned: Forecast.Reset nil receiver")
	}

	forecast.ratio.Reset()

	return forecast.weight.Reset()
}

func (forecast *Forecast) absorbSample(sample, alpha float64) error {
	if alpha <= 0 || alpha > 1 {
		return errors.New("learned: Forecast alpha must be in (0,1]")
	}

	return forecast.ratio.Update(sample, alpha)
}
