package fluid

import (
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/config"
	signalsupport "github.com/theapemachine/symm/signal"
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

	halfWidth, _ := signalsupport.DerivedGridHalfWidth(10)
	integrationInterval, _ := signalsupport.DerivedIntegrationInterval(1)
	depth, _ := config.DerivedBookDepthLevels()

	built := symbolConfig{
		tickSizeFallback:    config.DerivedSolverTickSize(depth),
		gridHalfWidth:       halfWidth,
		integrationInterval: integrationInterval,
		volumeBarsPerDay:    signalsupport.VolumeClockBarsPerDay(),
	}

	if symbolConfigValue.CompareAndSwap(nil, &built) {
		return built
	}

	return *symbolConfigValue.Load()
}
