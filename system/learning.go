package system

import (
	"github.com/spf13/viper"
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
	PolicyPrefix          string
	EnterSuffix           string
	ExitSuffix            string
	MinimumPathConfidence int
	ThresholdStart        float64
	ThresholdStep         float64
	ThresholdFloor        float64
	ThresholdCeil         float64
	VirtualPromote        int
	VocabSize             int
	DefaultWindowSize     int
	ShannonCapacity       float64
}

func NewLearning() *Learning {
	viper.SetDefault("learning.policy_prefix", "model/current/")
	viper.SetDefault("learning.enter_suffix", "/enter.bin")
	viper.SetDefault("learning.exit_suffix", "/exit.bin")
	viper.SetDefault("learning.minimum_path_confidence", 2)
	viper.SetDefault("learning.threshold_start", 0.5)
	viper.SetDefault("learning.threshold_step", 0.05)
	viper.SetDefault("learning.threshold_floor", 0.05)
	viper.SetDefault("learning.threshold_ceil", 0.95)
	viper.SetDefault("learning.virtual_promote", 3)
	viper.SetDefault("learning.vocab_size", 15)
	viper.SetDefault("learning.default_window_size", 15)
	viper.SetDefault("learning.shannon_capacity", 0.45)

	return &Learning{
		PolicyPrefix:          viper.GetString("learning.policy_prefix"),
		EnterSuffix:           viper.GetString("learning.enter_suffix"),
		ExitSuffix:            viper.GetString("learning.exit_suffix"),
		MinimumPathConfidence: viper.GetInt("learning.minimum_path_confidence"),
		ThresholdStart:        viper.GetFloat64("learning.threshold_start"),
		ThresholdStep:         viper.GetFloat64("learning.threshold_step"),
		ThresholdFloor:        viper.GetFloat64("learning.threshold_floor"),
		ThresholdCeil:         viper.GetFloat64("learning.threshold_ceil"),
		VirtualPromote:        viper.GetInt("learning.virtual_promote"),
		VocabSize:             viper.GetInt("learning.vocab_size"),
		DefaultWindowSize:     viper.GetInt("learning.default_window_size"),
		ShannonCapacity:       viper.GetFloat64("learning.shannon_capacity"),
	}
}
