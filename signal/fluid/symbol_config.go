package fluid

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
)

// secondsPerDay is the stable civil-time conversion used to derive the volume
// clock from the explicitly configured integration cadence.
const secondsPerDay = 24 * 60 * 60

/*
symbolConfig holds instrument-scale configuration so each fluid grid follows
exchange metadata.
*/
type symbolConfig struct {
	tickSizeFallback    float64
	gridHalfWidth       int
	integrationInterval time.Duration
	idleThreshold       time.Duration
	maxIntegrationSteps int
	volumeBarsPerDay    float64
	historyCapacity     int
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
call. Construction is once per symbol; caching the first writer poisoned package
proofs when unit tests mutated viper earlier in the same package run.
*/
func loadSymbolConfig() (symbolConfig, error) {
	halfWidth := viper.GetInt("signals.fluid.grid_half_width")
	idleThreshold := viper.GetDuration("signals.fluid.idle_threshold")
	maxIntegrationSteps := viper.GetInt("signals.fluid.max_integration_steps")

	if maxIntegrationSteps <= 0 {
		return symbolConfig{}, fmt.Errorf(
			"fluid: signals.fluid.max_integration_steps must be positive",
		)
	}

	if idleThreshold <= 0 {
		return symbolConfig{}, fmt.Errorf(
			"fluid: signals.fluid.idle_threshold must be positive",
		)
	}

	integrationInterval := viper.GetDuration("signals.fluid.integration_interval")

	if integrationInterval <= 0 {
		integrationInterval = idleThreshold / time.Duration(maxIntegrationSteps)
	}

	if integrationInterval <= 0 {
		return symbolConfig{}, fmt.Errorf(
			"fluid: signals.fluid.integration_interval could not be derived",
		)
	}

	// Bars per day is the number of integration steps that fit in a day. Derive
	// it from the interval so the volume clock and the field solver share one
	// cadence instead of two unrelated constants (was a bare 288).
	volumeBarsPerDay := viper.GetFloat64("signals.volume_clock_bars_per_day")

	if volumeBarsPerDay <= 0 {
		volumeBarsPerDay = float64(secondsPerDay) / integrationInterval.Seconds()
	}

	if math.IsNaN(volumeBarsPerDay) || math.IsInf(volumeBarsPerDay, 0) ||
		volumeBarsPerDay <= 0 {
		return symbolConfig{}, fmt.Errorf(
			"fluid: signals.volume_clock_bars_per_day must be positive and finite",
		)
	}

	historyCapacity := viper.GetInt("signals.feed_track_capacity")

	if historyCapacity <= 0 {
		return symbolConfig{}, fmt.Errorf(
			"fluid: signals.feed_track_capacity must be positive",
		)
	}

	built := symbolConfig{
		tickSizeFallback:    viper.GetFloat64("signals.fluid.tick_size"),
		gridHalfWidth:       halfWidth,
		integrationInterval: integrationInterval,
		idleThreshold:       idleThreshold,
		maxIntegrationSteps: maxIntegrationSteps,
		volumeBarsPerDay:    volumeBarsPerDay,
		historyCapacity:     historyCapacity,
	}

	symbolConfigValue.Store(&built)

	return built, nil
}
