package system

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/types"
)

/*
UninformativeDirectionConfidence is the probability boundary at which a
one-sided forecast provides no directional evidence. The regulator may raise
the confidence required to extend the adaptive forecast horizon above this
boundary; a lower value would call an unsupported direction informative.
*/
const UninformativeDirectionConfidence = 0.5

type PlannerConfig struct {
	Admission             types.AdmissionPolicy
	MaxAllocationFraction float64
	MinimumConfidence     float64
	MinimumGraphScore     float64
	MinimumUtility        float64
	CausalAlpha           float64
	MCTSIterations        int
	ExplorationConstant   float64
}

func NewPlannerConfig() *PlannerConfig {
	viper.SetDefault("trading.allocation.max_fraction", 0.1)
	viper.SetDefault("trading.admission.required_direction", 1.0)
	viper.SetDefault("trading.admission.minimum_thesis_score", 0.5)
	viper.SetDefault("trading.admission.minimum_confidence", 0.5)
	viper.SetDefault("trading.admission.minimum_support", 0.5)
	viper.SetDefault("trading.admission.maximum_contradiction", 0.3)
	viper.SetDefault(
		"trading.resonance.minimum_confidence",
		UninformativeDirectionConfidence,
	)
	viper.SetDefault("trading.evidence.minimum_score", -1.0)
	viper.SetDefault("trading.utility.minimum", 0.0)
	viper.SetDefault("trading.mcts.causal_alpha", 1.0)
	viper.SetDefault("trading.mcts.iterations", 16)
	viper.SetDefault("trading.mcts.exploration_constant", math.Sqrt2)

	return &PlannerConfig{
		Admission: types.AdmissionPolicy{
			RequiredDirection:    viper.GetFloat64("trading.admission.required_direction"),
			MinimumThesisScore:   viper.GetFloat64("trading.admission.minimum_thesis_score"),
			MinimumConfidence:    viper.GetFloat64("trading.admission.minimum_confidence"),
			MinimumSupport:       viper.GetFloat64("trading.admission.minimum_support"),
			MaximumContradiction: viper.GetFloat64("trading.admission.maximum_contradiction"),
		},
		MaxAllocationFraction: viper.GetFloat64("trading.allocation.max_fraction"),
		MinimumConfidence:     viper.GetFloat64("trading.resonance.minimum_confidence"),
		MinimumGraphScore:     viper.GetFloat64("trading.evidence.minimum_score"),
		MinimumUtility:        viper.GetFloat64("trading.utility.minimum"),
		CausalAlpha:           viper.GetFloat64("trading.mcts.causal_alpha"),
		MCTSIterations:        viper.GetInt("trading.mcts.iterations"),
		ExplorationConstant:   viper.GetFloat64("trading.mcts.exploration_constant"),
	}
}
