package market

import (
	"fmt"
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/telemetry"
)

/*
DynamicsEnvelope carries regime-derived windows and anomaly scales shared by
microstructure gates, signal buffers, and sizing envelopes.
*/
type DynamicsEnvelope struct {
	WindowCapacity int
	MinSamples     int
	AnomalySigma   float64
}

func LoadDynamicsEnvelope() (DynamicsEnvelope, error) {
	window := viper.GetInt("regime.window")
	minObs := viper.GetInt("regime.baseline.min_obs")
	trendSigma := viper.GetFloat64("regime.baseline.trend_sigma")

	if window <= 0 {
		return DynamicsEnvelope{}, fmt.Errorf("market dynamics: regime.window must be positive")
	}

	if minObs <= 0 {
		return DynamicsEnvelope{}, fmt.Errorf("market dynamics: regime.baseline.min_obs must be positive")
	}

	if trendSigma <= 0 || math.IsNaN(trendSigma) {
		return DynamicsEnvelope{}, fmt.Errorf("market dynamics: regime.baseline.trend_sigma must be positive")
	}

	sigma := trendSigma * telemetry.MarketSurpriseIndex()

	if sigma <= 0 || math.IsNaN(sigma) || math.IsInf(sigma, 0) {
		return DynamicsEnvelope{}, fmt.Errorf("market dynamics: anomaly sigma is invalid")
	}

	return DynamicsEnvelope{
		WindowCapacity: window,
		MinSamples:     minObs,
		AnomalySigma:   sigma,
	}, nil
}

/*
AnomalyCeiling returns median + sigma*MAD for one sample window.
*/
func AnomalyCeiling(samples []float64, envelope DynamicsEnvelope) (float64, bool) {
	if len(samples) < envelope.MinSamples {
		return 0, false
	}

	median := float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(samples...)...))
	mad := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(samples...)...))

	return median + envelope.AnomalySigma*mad, true
}

/*
AppendRingSample appends one observation into a bounded ring buffer.
*/
func AppendRingSample(values []float64, value float64, capacity int) []float64 {
	if value <= 0 || capacity <= 0 {
		return values
	}

	if len(values) >= capacity {
		copy(values, values[1:])
		values[len(values)-1] = value

		return values
	}

	return append(values, value)
}
