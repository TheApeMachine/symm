package fluid

import (
	"fmt"
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

func loadSymbolConfig() (symbolConfig, error) {
	if loaded := symbolConfigValue.Load(); loaded != nil {
		return *loaded, nil
	}

	halfWidth, halfWidthErr := signalsupport.DerivedGridHalfWidth(10)

	if halfWidthErr != nil {
		return symbolConfig{}, halfWidthErr
	}

	integrationInterval, intervalErr := signalsupport.DerivedIntegrationInterval(1)

	if intervalErr != nil {
		return symbolConfig{}, intervalErr
	}

	depth, depthErr := config.DerivedBookDepthLevels()

	if depthErr != nil {
		return symbolConfig{}, depthErr
	}

	built := symbolConfig{
		tickSizeFallback:    config.DerivedSolverTickSize(depth),
		gridHalfWidth:       halfWidth,
		integrationInterval: integrationInterval,
		volumeBarsPerDay:    signalsupport.VolumeClockBarsPerDay(),
	}

	if built.tickSizeFallback <= 0 {
		return symbolConfig{}, fmt.Errorf("fluid: derived solver tick size must be positive")
	}

	if symbolConfigValue.CompareAndSwap(nil, &built) {
		return built, nil
	}

	return *symbolConfigValue.Load(), nil
}
