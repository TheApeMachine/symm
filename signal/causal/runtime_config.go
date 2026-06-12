package causal

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/correlation"
	ckernel "github.com/theapemachine/nomagique/kernel/causal"
)

/*
RuntimeConfig carries causal signal tuning loaded once at boot.
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

var (
	runtimeConfigOnce sync.Once
	runtimeConfig     RuntimeConfig
)

func loadRuntimeConfig() RuntimeConfig {
	runtimeConfigOnce.Do(func() {
		viperInstance := viper.GetViper()
		contagionWindow := contagionWindowFromViper(viperInstance, "signals.causal.contagion_window", 128)
		contagionWindowFast := contagionWindowFromViper(viperInstance, "signals.causal.contagion_window_fast", 0)
		contagionWindowMedium := contagionWindowFromViper(viperInstance, "signals.causal.contagion_window_medium", 0)
		contagionWindowSlow := contagionWindowFromViper(viperInstance, "signals.causal.contagion_window_slow", 0)

		if contagionWindowFast <= 0 {
			contagionWindowFast = contagionWindow / 8

			if contagionWindowFast < 1 {
				contagionWindowFast = 1
			}
		}

		if contagionWindowMedium <= 0 {
			contagionWindowMedium = contagionWindow / 2

			if contagionWindowMedium < 1 {
				contagionWindowMedium = 1
			}
		}

		if contagionWindowSlow <= 0 {
			contagionWindowSlow = contagionWindow
		}

		runtimeConfig = RuntimeConfig{
			PublishInterval:               viperInstance.GetDuration("signals.causal.publish_interval"),
			ContagionMinSamples:           contagionMinSamplesFromViper(viperInstance),
			ContagionAdaptiveSigma:        contagionSigmaFromViper(viperInstance, "signals.causal.contagion_adaptive_sigma", 2),
			ContagionVolatilityResetSigma: contagionSigmaFromViper(viperInstance, "signals.causal.contagion_volatility_reset_sigma", 5),
			ContagionBreak:                viperInstance.GetFloat64("signals.causal.contagion_break"),
			ConditionSwitch:               viperInstance.GetFloat64("signals.causal.condition_switch"),
			ContagionWindow:               contagionWindow,
			ContagionWindowFast:           contagionWindowFast,
			ContagionWindowMedium:         contagionWindowMedium,
			ContagionWindowSlow:           contagionWindowSlow,
			ContagionSlowMax:              contagionWindowFromViper(viperInstance, "signals.causal.contagion_window_slow_max", 0),
		}
	})

	return runtimeConfig
}

func contagionMinSamplesFromViper(viperInstance *viper.Viper) int {
	minSamples := viperInstance.GetInt("signals.causal.contagion_min_samples")

	if minSamples > 0 {
		return minSamples
	}

	return 16
}

func contagionWindowFromViper(viperInstance *viper.Viper, key string, fallback int) int {
	window := viperInstance.GetInt(key)

	if window > 0 {
		return window
	}

	if fallback > 0 {
		return fallback
	}

	return 1
}

func contagionSigmaFromViper(viperInstance *viper.Viper, key string, fallback float64) float64 {
	sigma := viperInstance.GetFloat64(key)

	if sigma > 0 {
		return sigma
	}

	return fallback
}

func (config RuntimeConfig) ladderConfig() ckernel.LadderConfig {
	return ckernel.LadderConfig{
		TreatmentNormal:   localFlowNode,
		ControlsNormal:    []int{macroMomentumNode, liquidityNode},
		TreatmentInverted: liquidityNode,
		ControlsInverted:  []int{macroMomentumNode},
		ConditionLeft:     liquidityNode,
		ConditionRight:    localFlowNode,
		ContagionBreak:    config.ContagionBreak,
		ConditionSwitch:   config.ConditionSwitch,
		KernelBandwidth:   0.35,
		ConfoundFraction:  0.25,
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
