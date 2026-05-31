package broker

import (
	"math"
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
StressRegime captures live market microstructure intensity used to scale paper
execution stress. Rejection probability and quote latency swell during turbulent
micro-regimes instead of applying a flat Bernoulli draw.
*/
type StressRegime struct {
	Turbulence float64
	Vorticity  float64
}

/*
StressRegimeFrom derives execution stress intensity from the fluid gauge factors
present in the current measurement snapshot.
*/
func StressRegimeFrom(measurements []perspectives.Measurement) StressRegime {
	regime := StressRegime{}

	for _, measurement := range measurements {
		if measurement.Source != perspectives.SourceFluid {
			continue
		}

		for _, factor := range measurement.Factors {
			switch factor.Name {
			case "turb_fd":
				regime.Turbulence = math.Max(regime.Turbulence, factor.Value)
			case "vort":
				regime.Vorticity = math.Max(regime.Vorticity, math.Abs(factor.Value))
			}
		}
	}

	return regime
}

/*
Multiplier scales baseline stress knobs by live turbulence and vorticity.
*/
func (regime StressRegime) Multiplier() float64 {
	intensity := math.Max(regime.Turbulence, regime.Vorticity)

	if intensity <= 0 {
		return 1
	}

	return 1 + intensity
}

/*
EffectiveRejectRate scales a configured reject rate by the current micro-regime.
*/
func EffectiveRejectRate(baseRate float64, regime StressRegime) float64 {
	if baseRate <= 0 {
		return 0
	}

	return math.Min(1, baseRate*regime.Multiplier())
}

/*
EffectiveStressLatency scales configured quote-age stress by the current micro-regime.
*/
func EffectiveStressLatency(baseLatency time.Duration, regime StressRegime) time.Duration {
	if baseLatency <= 0 {
		return 0
	}

	multiplier := regime.Multiplier()

	if multiplier <= 1 {
		return baseLatency
	}

	return time.Duration(float64(baseLatency) * multiplier)
}
