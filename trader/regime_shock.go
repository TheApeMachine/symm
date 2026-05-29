package trader

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/ring"
)

/*
regimeShockBreaker detects discontinuous market shocks before feedback EMAs can
decay stale model trust.
*/
type regimeShockBreaker struct {
	mu        sync.RWMutex
	bySource  map[string]*regimeShockSeries
	active    bool
	recovery  int
	reason    string
	lastScore float64
}

type regimeShockSeries struct {
	samples ring.FloatRing
}

func newRegimeShockBreaker() *regimeShockBreaker {
	return &regimeShockBreaker{
		bySource: make(map[string]*regimeShockSeries),
	}
}

func (breaker *regimeShockBreaker) Observe(measurement engine.Measurement) {
	value, ok := shockFeature(measurement)

	if !ok {
		return
	}

	breaker.mu.Lock()
	defer breaker.mu.Unlock()

	series := breaker.seriesLocked(measurement.Source)
	breached, score := series.Breached(value)
	series.samples.Push(value)

	if breached {
		breaker.active = true
		breaker.recovery = 0
		breaker.reason = measurement.Source
		breaker.lastScore = score
		return
	}

	if !breaker.active {
		return
	}

	breaker.recovery++

	if breaker.recovery < configuredRegimeShockRecoverySamples() {
		return
	}

	breaker.active = false
	breaker.recovery = 0
	breaker.reason = ""
	breaker.lastScore = 0
}

func (breaker *regimeShockBreaker) Active() bool {
	if breaker == nil {
		return false
	}

	breaker.mu.RLock()
	defer breaker.mu.RUnlock()

	return breaker.active
}

func (breaker *regimeShockBreaker) Mutes(source string) bool {
	if breaker == nil || robustShockSource(source) {
		return false
	}

	breaker.mu.RLock()
	defer breaker.mu.RUnlock()

	return breaker.active
}

func (breaker *regimeShockBreaker) TrustFloor() float64 {
	floor := config.System.RegimeShockTrustFloor

	if floor < 0 {
		return 0
	}

	if floor > 1 {
		return 1
	}

	return floor
}

func (breaker *regimeShockBreaker) Snapshot() map[string]any {
	if breaker == nil {
		return nil
	}

	breaker.mu.RLock()
	defer breaker.mu.RUnlock()

	return map[string]any{
		"active":      breaker.active,
		"reason":      breaker.reason,
		"score":       breaker.lastScore,
		"recovery":    breaker.recovery,
		"trust_floor": breaker.TrustFloor(),
	}
}

func (breaker *regimeShockBreaker) seriesLocked(source string) *regimeShockSeries {
	series := breaker.bySource[source]

	if series != nil {
		return series
	}

	series = &regimeShockSeries{
		samples: ring.NewFloatRing(configuredRegimeShockWindow()),
	}
	breaker.bySource[source] = series

	return series
}

func (series *regimeShockSeries) Breached(value float64) (bool, float64) {
	samples := series.samples.Ordered()

	if len(samples) < configuredRegimeShockMinSamples() {
		return false, 0
	}

	sorted := numeric.CopySorted(samples)
	median := numeric.PercentileSorted(sorted, 0.5)
	mad := numeric.MedianAbsoluteDeviation(sorted, median)
	spread := math.Max(mad, math.Abs(median)/float64(len(samples)))

	if spread <= 0 {
		if value <= median {
			return false, 0
		}

		return true, math.Inf(1)
	}

	score := (value - median) / spread

	return score >= config.System.RegimeShockZScore, score
}

func shockFeature(measurement engine.Measurement) (float64, bool) {
	switch measurement.Source {
	case "correlation", "fluid":
		if measurement.Confidence <= 0 {
			return 0, false
		}

		return measurement.Confidence, true
	default:
		return 0, false
	}
}

func robustShockSource(source string) bool {
	switch source {
	case "cvd", "depthflow":
		return true
	default:
		return false
	}
}

func configuredRegimeShockWindow() int {
	if config.System.RegimeShockWindow > 0 {
		return config.System.RegimeShockWindow
	}

	return config.System.ForwardReturnMinSamples
}

func configuredRegimeShockMinSamples() int {
	if config.System.RegimeShockMinSamples > 0 {
		return config.System.RegimeShockMinSamples
	}

	return config.System.ForwardReturnMinSamples
}

func configuredRegimeShockRecoverySamples() int {
	if config.System.RegimeShockRecoverySamples > 0 {
		return config.System.RegimeShockRecoverySamples
	}

	return configuredRegimeShockMinSamples()
}
