package market

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/config"
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

type dynamicsEnvelopeSlot struct {
	envelope DynamicsEnvelope
	surprise float64
}

var (
	dynamicsEnvelopeCache      atomic.Pointer[dynamicsEnvelopeSlot]
	dynamicsEnvelopeConfigOnce sync.Once
	dynamicsEnvelopeWindow     int
	dynamicsEnvelopeMinObs     int
	dynamicsEnvelopeTrendSigma float64
	dynamicsEnvelopeConfigErr  error
)

func LoadDynamicsEnvelope() (DynamicsEnvelope, error) {
	dynamicsEnvelopeConfigOnce.Do(func() {
		regime, err := config.DerivedRegimeSpec()

		if err != nil {
			dynamicsEnvelopeConfigErr = err
			return
		}

		baseline := config.DerivedBaselineSpec(regime)
		dynamicsEnvelopeWindow = regime.Window
		dynamicsEnvelopeMinObs = regime.MinSamples
		dynamicsEnvelopeTrendSigma = baseline.TrendSigma

		if dynamicsEnvelopeTrendSigma <= 0 || math.IsNaN(dynamicsEnvelopeTrendSigma) {
			dynamicsEnvelopeConfigErr = fmt.Errorf("market dynamics: derived trend sigma is invalid")
		}
	})

	if dynamicsEnvelopeConfigErr != nil {
		return DynamicsEnvelope{}, dynamicsEnvelopeConfigErr
	}

	surprise := telemetry.MarketSurpriseIndex()

	if cached := dynamicsEnvelopeCache.Load(); cached != nil && cached.surprise == surprise {
		return cached.envelope, nil
	}

	sigma := dynamicsEnvelopeTrendSigma * surprise

	if sigma <= 0 || math.IsNaN(sigma) || math.IsInf(sigma, 0) {
		return DynamicsEnvelope{}, fmt.Errorf("market dynamics: anomaly sigma is invalid")
	}

	envelope := DynamicsEnvelope{
		WindowCapacity: dynamicsEnvelopeWindow,
		MinSamples:     dynamicsEnvelopeMinObs,
		AnomalySigma:   sigma,
	}

	dynamicsEnvelopeCache.Store(&dynamicsEnvelopeSlot{
		envelope: envelope,
		surprise: surprise,
	})

	return envelope, nil
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
