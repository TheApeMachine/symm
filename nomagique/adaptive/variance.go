package adaptive

import (
	"github.com/theapemachine/errnie"
)

/*
Variance tracks an adaptive mean and variance from the observed sample stream.
*/
type Variance struct {
	mean  float64
	m2    float64
	count int
}

/*
VarianceOutput reports the adaptive mean and variance.
*/
type VarianceOutput struct {
	Value float64
	Mean  float64
	Ready bool
	Count int
}

/*
NewVariance returns a typed adaptive variance tracker.
*/
func NewVariance() *Variance {
	return &Variance{}
}

/*
Measure adds one sample and returns sample variance when it is identifiable.
Sample variance requires two observations; a single point cannot estimate
dispersion around its own mean.
*/
func (variance *Variance) Measure(sample float64) (VarianceOutput, error) {
	if err := finiteAdaptive("variance", sample); err != nil {
		return VarianceOutput{}, err
	}

	variance.count++

	if variance.count == 1 {
		variance.mean = sample
		variance.m2 = 0

		return VarianceOutput{
			Mean:  variance.mean,
			Ready: false,
			Count: variance.count,
		}, nil
	}

	delta := sample - variance.mean
	variance.mean += delta / float64(variance.count)
	delta2 := sample - variance.mean
	variance.m2 += delta * delta2
	value := variance.m2 / float64(variance.count-1)

	if err := finiteAdaptive("variance", value); err != nil {
		return VarianceOutput{}, err
	}

	if value < 0 {
		return VarianceOutput{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"variance: sample variance must be non-negative",
			nil,
		))
	}

	return VarianceOutput{
		Value: value,
		Mean:  variance.mean,
		Ready: true,
		Count: variance.count,
	}, nil
}

/*
Count returns the number of observed samples.
*/
func (variance *Variance) Count() int {
	return variance.count
}
