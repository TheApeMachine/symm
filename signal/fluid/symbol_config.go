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

func loadSymbolConfig() (symbolConfig, error) {
	if loaded := symbolConfigValue.Load(); loaded != nil {
		return *loaded, nil
	}

	halfWidth := viper.GetInt("signals.fluid.grid_half_width")

	if halfWidth <= 0 {
		halfWidth = 10
	}

	integrationInterval := viper.GetDuration("signals.fluid.integration_interval")

	if integrationInterval <= 0 {
		integrationInterval = 1 * time.Minute
	}

	volumeBarsPerDay := viper.GetFloat64("signals.volume_clock_bars_per_day")

	if volumeBarsPerDay <= 0 {
		volumeBarsPerDay = 288
	}

	tickSizeFallback := viper.GetFloat64("signals.fluid.tick_size")

	built := symbolConfig{
		tickSizeFallback:    tickSizeFallback,
		gridHalfWidth:       halfWidth,
		integrationInterval: integrationInterval,
		volumeBarsPerDay:    volumeBarsPerDay,
	}

	if symbolConfigValue.CompareAndSwap(nil, &built) {
		return built, nil
	}

	return *symbolConfigValue.Load(), nil
}
