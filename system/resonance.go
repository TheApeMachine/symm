package system

import "github.com/spf13/viper"

type Resonance struct {
	LearningRate float64
	Layers       int
	MaxHorizon   int
}

func NewResonance() *Resonance {
	viper.SetDefault("resonance.learning_rate", 0.01)
	viper.SetDefault("resonance.layers", 3)
	viper.SetDefault("resonance.max_horizon", 10)

	return &Resonance{
		LearningRate: viper.GetFloat64("resonance.learning_rate"),
		Layers:       viper.GetInt("resonance.layers"),
		MaxHorizon:   viper.GetInt("resonance.max_horizon"),
	}
}
