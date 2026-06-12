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

func causalTable(samples []causalSample) (ckernel.NodeTable, error) {
	rows := make([][]float64, len(samples))

	for index := range samples {
		rows[index] = samples[index].nodes[:]
	}

	return ckernel.NewNodeTable(rows, priceVelocityNode, minCausalHistory)
}

func outcomeFromKernel(outcome ckernel.Outcome) causalOutcome {
	return causalOutcome{
		raw:          outcome.Raw,
		reason:       outcome.Reason,
		intervention: outcome.Intervention,
		association:  outcome.Association,
		uplift:       outcome.Uplift,
		inverted:     outcome.Inverted,
		contagion:    outcome.Contagion,
		condition:    outcome.Condition,
	}
}
