package fluid

import (
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
)

// Cold-start structural floors. These are NOT scoring thresholds — they only
// seed the volume clock before live market data has been observed. The live
// grid re-derives tick size and lattice width from the book.
const (
	// integrationIntervalFloor is only used when neither integration_interval
	// nor the idle/max-step budget can define a cadence.
	integrationIntervalFloor = time.Minute

	// maxIntegrationStepsFloor bounds catch-up work for stale or sparse books.
	maxIntegrationStepsFloor = 50

	// secondsPerDay couples the volume clock to the integration interval below,
	// so bars/day is derived from one cadence assumption rather than a second
	// independent magic number.
	secondsPerDay = 24 * 60 * 60
)

/*
symbolConfig holds instrument-scale configuration so each fluid grid follows
exchange metadata.
*/
type symbolConfig struct {
	tickSizeFallback    float64
	gridHalfWidth       int
	bookDepthLevels     int
	integrationInterval time.Duration
	idleThreshold       time.Duration
	maxIntegrationSteps int
	volumeBarsPerDay    float64
}

var symbolConfigValue atomic.Pointer[symbolConfig]

/*
resetSymbolConfig clears the last-loaded config snapshot. Tests that adjust
viper call this so observers of symbolConfigValue see a fresh load.
*/
func resetSymbolConfig() {
	symbolConfigValue.Store(nil)
}

/*
loadSymbolConfig reads fluid cadence and lattice floors from viper on every
call. Construction is once per symbol; caching the first writer poisoned Session
proofs when unit tests mutated viper earlier in the same package run.
*/
func loadSymbolConfig() (symbolConfig, error) {
	halfWidth := viper.GetInt("signals.fluid.grid_half_width")
	idleThreshold := viper.GetDuration("signals.fluid.idle_threshold")
	maxIntegrationSteps := viper.GetInt("signals.fluid.max_integration_steps")

	if maxIntegrationSteps <= 0 {
		maxIntegrationSteps = maxIntegrationStepsFloor
	}

	integrationInterval := viper.GetDuration("signals.fluid.integration_interval")

	if integrationInterval <= 0 && idleThreshold > 0 {
		integrationInterval = idleThreshold / time.Duration(maxIntegrationSteps)
	}

	if integrationInterval <= 0 {
		integrationInterval = integrationIntervalFloor
	}

	if idleThreshold <= 0 {
		idleThreshold = integrationInterval * time.Duration(maxIntegrationSteps)
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
		bookDepthLevels:     configuredBookDepthLevels(),
		integrationInterval: integrationInterval,
		idleThreshold:       idleThreshold,
		maxIntegrationSteps: maxIntegrationSteps,
		volumeBarsPerDay:    volumeBarsPerDay,
	}

	symbolConfigValue.Store(&built)

	return built, nil
}
