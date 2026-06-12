package market

import "fmt"

/*
EWMoments tracks an exponentially weighted mean and variance (of the residual)
for scalar telemetry such as token surprisal in bits. Variance uses the same
alpha as the mean update so both stay on comparable time scales.
*/
type EWMoments struct {
	observations int
	mean         float64
	varianceEWMA float64
}

/*
Update ingests one sample with blending alpha in (0, 1]. The first call only
seeds mean and leaves variance at zero.
*/
func (moments *EWMoments) Update(observation float64, alpha float64) error {
	if alpha <= 0 || alpha > 1 {
		return fmt.Errorf(
			"market: EWMoments.Update alpha must be in (0,1], got %g",
			alpha,
		)
	}

	moments.observations++

	if moments.observations == 1 {
		moments.mean = observation
		moments.varianceEWMA = 0

		return nil
	}

	delta := observation - moments.mean
	moments.mean += alpha * delta
	delta2 := observation - moments.mean
	moments.varianceEWMA = (1-alpha)*moments.varianceEWMA + alpha*delta*delta2

	return nil
}

func (moments *EWMoments) Observations() int {
	return moments.observations
}

func (moments *EWMoments) Mean() float64 {
	return moments.mean
}

func (moments *EWMoments) VarianceEWMA() float64 {
	return moments.varianceEWMA
}

func (moments *EWMoments) Reset() {
	moments.observations = 0
	moments.mean = 0
	moments.varianceEWMA = 0
}
