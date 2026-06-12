package config

import (
	"math"

	"github.com/spf13/viper"
)

const (
	defaultEntryConfidenceBaseline      = 0.55
	defaultEntrySurpriseBaseline        = 1.0
	defaultExitConfidenceFloor          = 0.35
	defaultTurbulenceConfidenceScale    = 0.30
)

/*
ThresholdConfig carries playbook confidence and surprise baselines loaded once at boot.
*/
type ThresholdConfig struct {
	EntryConfidenceBaseline   float64
	ExitConfidenceBaseline    float64
	EntrySurpriseBaseline     float64
	TurbulenceConfidenceScale float64
	ExitConfidenceFloor       float64
}

func LoadThresholdConfig() (ThresholdConfig, error) {
	entryBaseline := viper.GetFloat64("trading.entry.confidence_baseline")

	if entryBaseline <= 0 {
		entryBaseline = defaultEntryConfidenceBaseline
	}

	exitBaseline := viper.GetFloat64("trading.exit.confidence_baseline")

	if exitBaseline <= 0 {
		exitBaseline = entryBaseline - 0.05
	}

	surpriseBaseline := viper.GetFloat64("trading.entry.surprise_baseline")

	if surpriseBaseline <= 0 {
		surpriseBaseline = defaultEntrySurpriseBaseline
	}

	floor := viper.GetFloat64("trading.exit.confidence_floor")

	if floor <= 0 {
		floor = defaultExitConfidenceFloor
	}

	turbulenceScale := viper.GetFloat64("trading.entry.turbulence_confidence_scale")

	if turbulenceScale <= 0 || math.IsNaN(turbulenceScale) {
		turbulenceScale = defaultTurbulenceConfidenceScale
	}

	return ThresholdConfig{
		EntryConfidenceBaseline:   entryBaseline,
		ExitConfidenceBaseline:    exitBaseline,
		EntrySurpriseBaseline:     surpriseBaseline,
		TurbulenceConfidenceScale: turbulenceScale,
		ExitConfidenceFloor:       floor,
	}, nil
}
