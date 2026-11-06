package system

import "github.com/spf13/viper"

/*
Resonance configures the predictive coder pace and layer depth for the
per-symbol resonance solver.
*/
type Resonance struct {
	LearningRate float64
	Layers       int
}

func NewResonance() *Resonance {
	viper.SetDefault("resonance.layers", 3)
	viper.SetDefault("resonance.learning_rate", 0.01)

	return &Resonance{
		LearningRate: viper.GetFloat64("resonance.learning_rate"),
		Layers:       viper.GetInt("resonance.layers"),
	}
}
