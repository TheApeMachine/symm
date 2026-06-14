package causal

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/config"
	"gonum.org/v1/gonum/stat"
)

const spreadBPSHistoryCap = 64
const minSpreadBPSHistory = 3

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

var runtimeConfigValue sync.Map

func loadRuntimeConfig() RuntimeConfig {
	if loaded, ok := runtimeConfigValue.Load("config"); ok {
		return loaded.(RuntimeConfig)
	}

	built := buildRuntimeConfig()
	runtimeConfigValue.Store("config", built)

	return built
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

func (runtimeConfig RuntimeConfig) ladderConfig() causal.LadderConfig {
	ladderConfig := causal.LadderConfig{
		TreatmentNormal:   localFlowNode,
		ControlsNormal:    []int{macroMomentumNode, liquidityNode},
		TreatmentInverted: liquidityNode,
		ControlsInverted:  []int{macroMomentumNode},
		ConditionLeft:     liquidityNode,
		ConditionRight:    localFlowNode,
		ContagionBreak:    runtimeConfig.ContagionBreak,
		ConditionSwitch:   runtimeConfig.ConditionSwitch,
		MinHistory:        minCausalHistory,
	}

	if ladderConfig.KernelBandwidth <= 0 {
		ladderConfig.KernelBandwidth = viper.GetFloat64("signals.causal.kernel_bandwidth")
	}

	if ladderConfig.ConfoundFraction <= 0 {
		ladderConfig.ConfoundFraction = viper.GetFloat64("signals.causal.confound_fraction")
	}

	return ladderConfig
}

func (runtimeConfig RuntimeConfig) contagionConfig() correlation.ContagionConfig {
	return correlation.ContagionConfig{
		MinSamples:     runtimeConfig.ContagionMinSamples,
		SymbolCap:      contagionSymbolCap,
		AdaptiveSigma:  runtimeConfig.ContagionAdaptiveSigma,
		SpreadCapacity: 64,
	}
}

func (runtimeConfig RuntimeConfig) newHYWindowSet() *correlation.WindowSet {
	if runtimeConfig.ContagionSlowMax > 0 {
		return correlation.NewWindowSet(runtimeConfig.ContagionSlowMax)
	}

	return correlation.NewWindowSet(runtimeConfig.ContagionWindow)
}

func (runtimeConfig RuntimeConfig) contagionTierWindows() correlation.TierWindows {
	return correlation.TierWindows{
		Fast:   runtimeConfig.ContagionWindowFast,
		Medium: runtimeConfig.ContagionWindowMedium,
		Slow:   runtimeConfig.ContagionWindowSlow,
	}
}

func contagionWindowsFromAdaptation() (fastWindow, mediumWindow, slowWindow int) {
	config := loadRuntimeConfig()

	return config.ContagionWindowFast, config.ContagionWindowMedium, config.ContagionWindowSlow
}

func (state *CausalSymbol) spreadBPSFloor() float64 {
	if len(state.spreadBPSHistory) < minSpreadBPSHistory {
		return 0
	}

	return float64(
		statistic.NewQuantile(0.1, stat.LinInterp, nil).Observe(
			nomagique.Numbers(state.spreadBPSHistory...)...,
		),
	)
}

func (state *CausalSymbol) recordSpreadBPS(spreadBPS float64) {
	if spreadBPS <= 0 {
		return
	}

	state.spreadBPSHistory = appendRingFloat(state.spreadBPSHistory, spreadBPS, spreadBPSHistoryCap)
}

func appendRingFloat(values []float64, value float64, capacity int) []float64 {
	values = append(values, value)

	if len(values) <= capacity {
		return values
	}

	return values[len(values)-capacity:]
}
