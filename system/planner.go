package system

import (
	"math"

	"github.com/spf13/viper"
)

type PlannerConfig struct {
	MaxAllocationFraction float64
	MinimumSkill          float64
	MinimumConfidence     float64
	CausalAlpha           float64
	MCTSIterations        int
	ExplorationConstant   float64
}

func NewPlannerConfig() *PlannerConfig {
	viper.SetDefault("trading.allocation.max_fraction", 0.1)
	viper.SetDefault("trading.resonance.minimum_skill", 0.5)
	viper.SetDefault("trading.resonance.minimum_confidence", 0.8)
	viper.SetDefault("trading.mcts.causal_alpha", 1.0)
	viper.SetDefault("trading.mcts.iterations", 50)
	viper.SetDefault("trading.mcts.exploration_constant", math.Sqrt2)

	return &PlannerConfig{
		MaxAllocationFraction: viper.GetFloat64("trading.allocation.max_fraction"),
		MinimumSkill:          viper.GetFloat64("trading.resonance.minimum_skill"),
		MinimumConfidence:     viper.GetFloat64("trading.resonance.minimum_confidence"),
		CausalAlpha:           viper.GetFloat64("trading.mcts.causal_alpha"),
		MCTSIterations:        viper.GetInt("trading.mcts.iterations"),
		ExplorationConstant:   viper.GetFloat64("trading.mcts.exploration_constant"),
	}
}
