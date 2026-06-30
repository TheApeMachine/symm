package fluid

import (
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
)

// Cold-start structural floors. These are NOT scoring thresholds — they only
// seed the lattice geometry and the volume clock before live market data has
// been observed. Operator config (viper) overrides each; the live grid then
// re-derives tick size from the book (resolveBookTickSize) and the integration
// interval / volume cadence from observed event spacing once frames arrive.
const (
	// gridHalfWidthFloor is the lattice radius in tick units around the touch.
	// ponytail: a fixed radius is the ceiling — a venue with very fine ticks
	// wants a wider window than one with coarse ticks. Upgrade path: derive the
	// radius from observed touch-spread / tick_size (instrument metadata in the
	// tree) so the lattice spans the real near-touch band per instrument.
	gridHalfWidthFloor = 10

	// integrationIntervalFloor seeds the field integration step before any event
	// cadence is observed.
	// ponytail: a fixed step is the ceiling. Upgrade path: derive the step from
	// statutil.MedianCadence over observed event timestamps so the solver steps
	// at the instrument's real arrival rate, not a one-minute guess.
	integrationIntervalFloor = time.Minute

	// maxIntegrationStepsFloor bounds catch-up work for stale or sparse books.
	maxIntegrationStepsFloor = 50

	// secondsPerDay couples the volume clock to the integration interval below,
	// so bars/day is derived from one cadence assumption rather than a second
	// independent magic number.
	secondsPerDay = 24 * 60 * 60
)

type symbolConfig struct {
	tickSizeFallback    float64
	gridHalfWidth       int
	integrationInterval time.Duration
	idleThreshold       time.Duration
	maxIntegrationSteps int
	volumeBarsPerDay    float64
}

var symbolConfigValue atomic.Pointer[symbolConfig]

func loadSymbolConfig() (symbolConfig, error) {
	if loaded := symbolConfigValue.Load(); loaded != nil {
		return *loaded, nil
	}

	halfWidth := viper.GetInt("signals.fluid.grid_half_width")

	if halfWidth <= 0 {
		halfWidth = gridHalfWidthFloor
	}

	integrationInterval := viper.GetDuration("signals.fluid.integration_interval")

	if integrationInterval <= 0 {
		integrationInterval = integrationIntervalFloor
	}

	idleThreshold := viper.GetDuration("signals.fluid.idle_threshold")

	if idleThreshold <= 0 {
		idleThreshold = integrationInterval * maxIntegrationStepsFloor
	}

	maxIntegrationSteps := viper.GetInt("signals.fluid.max_integration_steps")

	if maxIntegrationSteps <= 0 {
		maxIntegrationSteps = maxIntegrationStepsFloor
	}

	// Bars per day is the number of integration steps that fit in a day. Derive
	// it from the interval so the volume clock and the field solver share one
	// cadence instead of two unrelated constants (was a bare 288).
	volumeBarsPerDay := viper.GetFloat64("signals.volume_clock_bars_per_day")

	if volumeBarsPerDay <= 0 {
		volumeBarsPerDay = float64(secondsPerDay) / integrationInterval.Seconds()
	}

	built := symbolConfig{
		tickSizeFallback:    viper.GetFloat64("signals.fluid.tick_size"),
		gridHalfWidth:       halfWidth,
		integrationInterval: integrationInterval,
		idleThreshold:       idleThreshold,
		maxIntegrationSteps: maxIntegrationSteps,
		volumeBarsPerDay:    volumeBarsPerDay,
	}

	if symbolConfigValue.CompareAndSwap(nil, &built) {
		return built, nil
	}

	return *symbolConfigValue.Load(), nil
}
