package fluid

import (
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

/*
withFluidConfig decorates a Convey branch with isolated Viper settings. The
parent branch is re-executed for every child, and Reset restores global state
at the end of that branch as prescribed by GoConvey's execution model.
*/
func withFluidConfig(settings map[string]any, run func()) func() {
	return func() {
		previous := make(map[string]any, len(settings))

		for key, value := range settings {
			previous[key] = viper.Get(key)
			viper.Set(key, value)
		}

		resetSymbolConfig()
		Reset(func() {
			for key, value := range previous {
				viper.Set(key, value)
			}

			resetSymbolConfig()
		})
		run()
	}
}

/*
withFluidGrid decorates a Convey branch with a valid fixed lattice. Overrides
exist only for tests whose stated condition changes one of those facts.
*/
func withFluidGrid(overrides map[string]any, run func()) func() {
	return withFluidConfig(fluidGridSettings(overrides), run)
}

/* fluidGridSettings returns the fixed lattice facts shared by grid proofs. */
func fluidGridSettings(overrides map[string]any) map[string]any {
	settings := map[string]any{
		"market.book_depth_levels":            25,
		"signals.fluid.tick_size":             0.01,
		"signals.fluid.grid_half_width":       10,
		"signals.fluid.integration_interval":  100 * time.Millisecond,
		"signals.fluid.idle_threshold":        5 * time.Second,
		"signals.fluid.max_integration_steps": 50,
	}

	for key, value := range overrides {
		settings[key] = value
	}

	return settings
}

/* configureFluidBenchmark applies process-local settings for Go benchmarks. */
func configureFluidBenchmark(settings map[string]any) {
	for key, value := range settings {
		viper.Set(key, value)
	}

	resetSymbolConfig()
}
