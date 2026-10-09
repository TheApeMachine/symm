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
	PrecursorHorizon   int
	// MinimumPathConfidence is the matched token path length (one per token)
	// Step requires before acting on a unanimous match. It has no default:
	// the configuration must state it.
	MinimumPathConfidence int
}

func NewLearning() *Learning {
	// Eight independent workers is a resource allocation; the first evaluates the shared model.
	viper.SetDefault("learning.traders", 8)
	// Operational persistence cadence, never a decision or outcome horizon.
	viper.SetDefault("learning.checkpoint_interval", "1m")
	viper.SetDefault("learning.rehearsal_dropout", false)
	viper.SetDefault("learning.precursor_horizon", 8)
	return &Learning{
		Traders:               viper.GetInt("learning.traders"),
		CheckpointInterval:    viper.GetDuration("learning.checkpoint_interval"),
		RehearsalDropout:      viper.GetBool("learning.rehearsal_dropout"),
		PrecursorHorizon:      viper.GetInt("learning.precursor_horizon"),
		MinimumPathConfidence: viper.GetInt("learning.minimum_path_confidence"),
	}
}
