package system

import "github.com/spf13/viper"

/*
Parallel configures the physics+coder three-book experiment. It does not
share policy, model, or desk slots with the incumbent strategy.
*/
type Parallel struct {
	Enabled bool
	Pace    float64
}

func NewParallel() *Parallel {
	viper.SetDefault("parallel.enabled", false)
	viper.SetDefault("parallel.pace", 0.05)

	return &Parallel{
		Enabled: viper.GetBool("parallel.enabled"),
		Pace:    viper.GetFloat64("parallel.pace"),
	}
}
