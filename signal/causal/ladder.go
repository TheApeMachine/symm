package causal

import (
	"github.com/spf13/viper"
	ckernel "github.com/theapemachine/nomagique/kernel/causal"
)

func ladderConfigFromViper() ckernel.LadderConfig {
	viperInstance := viper.GetViper()

	return ckernel.LadderConfig{
		TreatmentNormal:   localFlowNode,
		ControlsNormal:    []int{macroMomentumNode, liquidityNode},
		TreatmentInverted: liquidityNode,
		ControlsInverted:  []int{macroMomentumNode},
		ConditionLeft:     liquidityNode,
		ConditionRight:    localFlowNode,
		ContagionBreak:    viperInstance.GetFloat64("signals.causal.contagion_break"),
		ConditionSwitch:   viperInstance.GetFloat64("signals.causal.condition_switch"),
		KernelBandwidth:   0.35,
		ConfoundFraction:  0.25,
		MinHistory:        minCausalHistory,
	}
}
