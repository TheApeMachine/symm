package system

import (
	"github.com/spf13/viper"
	"time"
)

/*
Learning configures resource use, persistence cadence, and opt-in rehearsal
augmentation, not market beliefs.

RehearsalDropout additionally teaches every rehearsed context once with one
of its region frames missing and once with one frame swapped for another
region of the same fragment tape (deliberate noise, as dropout does). It is
off unless enabled: the perturbed contexts never occurred in the market.
*/
type Learning struct {
	Traders            int
	CheckpointInterval time.Duration
	RehearsalDropout   bool
}

func NewLearning() *Learning {
	// Eight independent workers is a resource allocation; the first evaluates the shared model.
	viper.SetDefault("learning.traders", 8)
	// Operational persistence cadence, never a decision or outcome horizon.
	viper.SetDefault("learning.checkpoint_interval", "1m")
	viper.SetDefault("learning.rehearsal_dropout", false)
	return &Learning{
		Traders:            viper.GetInt("learning.traders"),
		CheckpointInterval: viper.GetDuration("learning.checkpoint_interval"),
		RehearsalDropout:   viper.GetBool("learning.rehearsal_dropout"),
	}
}
