package fluid

import (
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
)

/*
fluidViperMu serializes viper Set/Cleanup across fluid package tests.
Viper is process-global, so these tests must not mutate fluid keys concurrently.
*/
var fluidViperMu sync.Mutex

/*
withFluidInterval pins the integration cadence for market proofs.
*/
func withFluidInterval(t testing.TB) {
	t.Helper()
	pinFluidViper(t, map[string]any{
		"signals.fluid.integration_interval": 100 * time.Millisecond,
	})
}

/*
pinFluidViper applies keys under the package lock and restores them on cleanup.
The lock is not held for the test body, so nested Convey helpers may pin again.
*/
func pinFluidViper(t testing.TB, sets map[string]any) {
	t.Helper()

	fluidViperMu.Lock()
	previous := make(map[string]any, len(sets))

	for key, value := range sets {
		previous[key] = viper.Get(key)
		viper.Set(key, value)
	}

	resetSymbolConfig()
	fluidViperMu.Unlock()

	t.Cleanup(func() {
		fluidViperMu.Lock()
		defer fluidViperMu.Unlock()

		for key, value := range previous {
			viper.Set(key, value)
		}

		resetSymbolConfig()
	})
}

/*
setFluidGridConfig pins lattice defaults for grid unit tests.
*/
func setFluidGridConfig(t testing.TB) {
	t.Helper()
	pinFluidViper(t, map[string]any{
		"market.book_depth_levels":           25,
		"signals.fluid.tick_size":            0.01,
		"signals.fluid.grid_half_width":      10,
		"signals.fluid.integration_interval": 100 * time.Millisecond,
	})
}

/*
resetFluidConfig pins book-derived tick resolution defaults for tick-size tests.
*/
func resetFluidConfig(t testing.TB) {
	t.Helper()
	pinFluidViper(t, map[string]any{
		"market.book_depth_levels":            25,
		"signals.fluid.tick_size":             0,
		"signals.fluid.grid_half_width":       0,
		"signals.fluid.integration_interval":  100 * time.Millisecond,
		"signals.fluid.idle_threshold":        5 * time.Second,
		"signals.fluid.max_integration_steps": 50,
		"signals.volume_clock_bars_per_day":   288,
	})
}
