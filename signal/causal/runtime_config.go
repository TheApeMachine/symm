package causal

import (
	"sync/atomic"
	"time"

	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/config"
)

/*
RuntimeConfig carries causal signal tuning derived from regime sizing.
*/
type RuntimeConfig struct {
	PublishInterval               time.Duration
	ContagionMinSamples           int
	ContagionAdaptiveSigma        float64
	ContagionVolatilityResetSigma float64
	ContagionBreak                float64
	ConditionSwitch               float64
	ContagionWindow               int
	ContagionWindowFast           int
	ContagionWindowMedium         int
	ContagionWindowSlow           int
	ContagionSlowMax              int
}

var runtimeConfigValue atomic.Pointer[RuntimeConfig]

func loadRuntimeConfig() RuntimeConfig {
	if loaded := runtimeConfigValue.Load(); loaded != nil {
		return *loaded
	}

	built := buildRuntimeConfig()

	if runtimeConfigValue.CompareAndSwap(nil, &built) {
		return built
	}

	return *runtimeConfigValue.Load()
}

func buildRuntimeConfig() RuntimeConfig {
	regime, err := config.DerivedRegimeSpec()
	publishInterval := config.DerivedPublishInterval()

	slowWindowMax := 128
	slowWindowMin := 16
	regimeSpec := config.RegimeSpec{Window: slowWindowMax * 2, MinSamples: slowWindowMin}

	if err == nil {
		slowWindowMax = regime.Window / 2
		slowWindowMin = slowWindowMax / 8

		if slowWindowMin < regime.MinSamples {
			slowWindowMin = regime.MinSamples
		}

		regimeSpec = regime
	}

	contagionWindow := slowWindowMax
	contagionWindowFast := contagionWindow / 8

	if contagionWindowFast < 1 {
		contagionWindowFast = 1
	}

	contagionWindowMedium := contagionWindow / 2

	if contagionWindowMedium < 1 {
		contagionWindowMedium = 1
	}

	baseline := config.DerivedBaselineSpec(regimeSpec)

	return RuntimeConfig{
		PublishInterval:               publishInterval,
		ContagionMinSamples:           slowWindowMin,
		ContagionAdaptiveSigma:        baseline.TrendSigma,
		ContagionVolatilityResetSigma: baseline.VolFloorSigma,
		ContagionBreak:                1.0 - 1.0/baseline.StrongTrendSigma,
		ConditionSwitch:               float64(contagionWindow * contagionWindow),
		ContagionWindow:               contagionWindow,
		ContagionWindowFast:           contagionWindowFast,
		ContagionWindowMedium:         contagionWindowMedium,
		ContagionWindowSlow:           contagionWindow,
		ContagionSlowMax:              slowWindowMax,
	}
}

func (config RuntimeConfig) ladderConfig() causal.LadderConfig {
	return causal.LadderConfig{
		TreatmentNormal:   localFlowNode,
		ControlsNormal:    []int{macroMomentumNode, liquidityNode},
		TreatmentInverted: liquidityNode,
		ControlsInverted:  []int{macroMomentumNode},
		ConditionLeft:     liquidityNode,
		ConditionRight:    localFlowNode,
		ContagionBreak:    config.ContagionBreak,
		ConditionSwitch:   config.ConditionSwitch,
		MinHistory:        minCausalHistory,
	}
}

func (config RuntimeConfig) contagionConfig() correlation.ContagionConfig {
	return correlation.ContagionConfig{
		MinSamples:     config.ContagionMinSamples,
		SymbolCap:      contagionSymbolCap,
		AdaptiveSigma:  config.ContagionAdaptiveSigma,
		SpreadCapacity: 64,
	}
}
