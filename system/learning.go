package system

import (
	"github.com/spf13/viper"
	"time"
)

// Learning configures resource use and persistence cadence, not market beliefs.
type Learning struct {
	Traders            int
	CheckpointInterval time.Duration
}

func NewLearning() *Learning {
	// Eight independent workers is a resource allocation; the first evaluates the shared model.
	viper.SetDefault("learning.traders", 8)
	// Operational persistence cadence, never a decision or outcome horizon.
	viper.SetDefault("learning.checkpoint_interval", "1m")
	return &Learning{Traders: viper.GetInt("learning.traders"), CheckpointInterval: viper.GetDuration("learning.checkpoint_interval")}
}
