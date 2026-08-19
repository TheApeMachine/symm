package system

import (
	"math"

	"github.com/spf13/viper"
)

/*
UninformativeDirectionConfidence is the probability boundary at which a
one-sided forecast provides no directional evidence. The regulator may raise
the confidence required to extend the adaptive forecast horizon above this
boundary; a lower value would call an unsupported direction informative.
*/
const UninformativeDirectionConfidence = 0.5

type PlannerConfig struct {
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
		MaxAllocationFraction: viper.GetFloat64("trading.allocation.max_fraction"),
		MinimumConfidence:     viper.GetFloat64("trading.resonance.minimum_confidence"),
		MinimumGraphScore:     viper.GetFloat64("trading.evidence.minimum_score"),
		MinimumUtility:        viper.GetFloat64("trading.utility.minimum"),
		CausalAlpha:           viper.GetFloat64("trading.mcts.causal_alpha"),
		MCTSIterations:        viper.GetInt("trading.mcts.iterations"),
		ExplorationConstant:   viper.GetFloat64("trading.mcts.exploration_constant"),
	}
}
