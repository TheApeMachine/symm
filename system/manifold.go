package system

import "github.com/spf13/viper"

type Manifold struct {
	RelaxationSteps int
	MinSteps        int
	MaxSteps        int
}

func NewManifold() *Manifold {
	viper.SetDefault("manifold.relaxation_steps", 20)
	viper.SetDefault("manifold.min_steps", 10)
	viper.SetDefault("manifold.max_steps", 100)

	return &Manifold{
		RelaxationSteps: viper.GetInt("manifold.relaxation_steps"),
		MinSteps:        viper.GetInt("manifold.min_steps"),
		MaxSteps:        viper.GetInt("manifold.max_steps"),
	}
}
