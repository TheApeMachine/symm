package fluid

import (
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
)

type symbolConfig struct {
	tickSizeFallback    float64
	gridHalfWidth       int
	integrationInterval time.Duration
	volumeBarsPerDay    float64
}

var symbolConfigValue atomic.Pointer[symbolConfig]

func loadSymbolConfig() symbolConfig {
	if loaded := symbolConfigValue.Load(); loaded != nil {
		return *loaded
	}

	built := symbolConfig{
		tickSizeFallback:    viper.GetFloat64("signals.fluid.tick_size"),
		gridHalfWidth:       viper.GetInt("signals.fluid.grid_half_width"),
		integrationInterval: viper.GetDuration("signals.fluid.integration_interval"),
		volumeBarsPerDay:    viper.GetFloat64("signals.volume_clock_bars_per_day"),
	}

	if symbolConfigValue.CompareAndSwap(nil, &built) {
		return built
	}

	return *symbolConfigValue.Load()
}
