package fluid

import (
	"sync"
	"time"

	"github.com/spf13/viper"
)

type symbolConfig struct {
	tickSizeFallback    float64
	gridHalfWidth       int
	integrationInterval time.Duration
	volumeBarsPerDay    float64
}

var (
	symbolConfigOnce sync.Once
	symbolConfigLoad symbolConfig
)

func loadSymbolConfig() symbolConfig {
	symbolConfigOnce.Do(func() {
		symbolConfigLoad = symbolConfig{
			tickSizeFallback:    viper.GetFloat64("signals.fluid.tick_size"),
			gridHalfWidth:       viper.GetInt("signals.fluid.grid_half_width"),
			integrationInterval: viper.GetDuration("signals.fluid.integration_interval"),
			volumeBarsPerDay:    viper.GetFloat64("signals.volume_clock_bars_per_day"),
		}
	})

	return symbolConfigLoad
}
