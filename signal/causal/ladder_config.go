package causal

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/statistic"
	"gonum.org/v1/gonum/stat"
)

const spreadBPSHistoryCap = 64
const minSpreadBPSHistory = 3

func (state *CausalSymbol) recordSpreadBPS(spreadBPS float64) {
	if spreadBPS <= 0 {
		return
	}

	state.spreadBPSHistory = appendRingFloat(state.spreadBPSHistory, spreadBPS, spreadBPSHistoryCap)
}

func (state *CausalSymbol) spreadBPSFloor() float64 {
	if len(state.spreadBPSHistory) < minSpreadBPSHistory {
		return 0
	}

	return float64(
		statistic.NewQuantile(0.1, stat.LinInterp, nil).Observe(nomagique.Numbers(state.spreadBPSHistory...)...),
	)
}

func (state *CausalSymbol) ladderConfig() causal.LadderConfig {
	config := loadRuntimeConfig().ladderConfig()
	localFlows := state.localFlowSamples()

	if len(localFlows) >= minCausalHistory {
		flowMean := meanOf(localFlows)
		flowVariance := varianceOf(localFlows)

		if flowVariance > 0 {
			flowSigma := math.Sqrt(flowVariance)
			sampleCount := float64(len(localFlows))

			if config.KernelBandwidth <= 0 {
				config.KernelBandwidth = 1.06 * flowSigma * math.Pow(sampleCount, -0.2)
			}

			if config.ConfoundFraction <= 0 && flowMean > 0 {
				config.ConfoundFraction = math.Min(0.5, flowSigma/flowMean)
			}
		}
	}

	if config.KernelBandwidth <= 0 {
		config.KernelBandwidth = viper.GetFloat64("signals.causal.kernel_bandwidth")
	}

	if config.ConfoundFraction <= 0 {
		config.ConfoundFraction = viper.GetFloat64("signals.causal.confound_fraction")
	}

	return config
}

func (state *CausalSymbol) localFlowSamples() []float64 {
	if len(state.samples) == 0 {
		return nil
	}

	flows := make([]float64, len(state.samples))

	for index := range state.samples {
		flows[index] = state.samples[index].nodes[localFlowNode]
	}

	return flows
}

func appendRingFloat(values []float64, value float64, capacity int) []float64 {
	values = append(values, value)

	if len(values) <= capacity {
		return values
	}

	return values[len(values)-capacity:]
}

func meanOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0

	for _, value := range values {
		sum += value
	}

	return sum / float64(len(values))
}

func varianceOf(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	mean := meanOf(values)
	sumSquares := 0.0

	for _, value := range values {
		delta := value - mean
		sumSquares += delta * delta
	}

	return sumSquares / float64(len(values)-1)
}
