package config

import (
	"errors"
	"math"

	"github.com/spf13/viper"
)

const (
	defaultEntryConfidenceBaseline   = 0.55
	defaultEntrySurpriseBaseline     = 1.0
	defaultExitConfidenceFloor       = 0.35
	defaultTurbulenceConfidenceScale = 0.30
	defaultEntryTemperatureScale     = 0.35
	defaultEntryConfidenceCeiling    = 0.90
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
	// EntryTemperatureScale raises the entry confidence bar as the macro market
	// temperature rises: required = baseline + scale*temperature, capped at the
	// ceiling. This is the ontological hierarchy gate — hot/frenzied macro state
	// makes micro triggers prove themselves more before an entry fires.
	EntryTemperatureScale  float64
	EntryConfidenceCeiling float64
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

	temperatureScale := viper.GetFloat64("trading.entry.temperature_confidence_scale")

	if temperatureScale <= 0 || math.IsNaN(temperatureScale) {
		temperatureScale = defaultEntryTemperatureScale
	}

	ceiling := viper.GetFloat64("trading.entry.confidence_ceiling")

	if ceiling <= 0 || math.IsNaN(ceiling) {
		ceiling = defaultEntryConfidenceCeiling
	}

	config := ThresholdConfig{
		EntryConfidenceBaseline:   entryBaseline,
		ExitConfidenceBaseline:    exitBaseline,
		EntrySurpriseBaseline:     surpriseBaseline,
		TurbulenceConfidenceScale: turbulenceScale,
		ExitConfidenceFloor:       floor,
		EntryTemperatureScale:     temperatureScale,
		EntryConfidenceCeiling:    ceiling,
	}

	return NewSafeConfig(config)
}

func (config ThresholdConfig) Validate() error {
	if err := requireUnitInterval(
		"trading.entry.confidence_baseline",
		config.EntryConfidenceBaseline,
	); err != nil {
		return err
	}

	if err := requireUnitInterval(
		"trading.exit.confidence_baseline",
		config.ExitConfidenceBaseline,
	); err != nil {
		return err
	}

	if err := requireUnitInterval(
		"trading.exit.confidence_floor",
		config.ExitConfidenceFloor,
	); err != nil {
		return err
	}

	if config.ExitConfidenceFloor > config.ExitConfidenceBaseline {
		return errors.New(
			"trading.exit.confidence_floor must not exceed confidence_baseline",
		)
	}

	if err := requirePositiveFinite(
		"trading.entry.surprise_baseline",
		config.EntrySurpriseBaseline,
	); err != nil {
		return err
	}

	if err := requireUnitInterval(
		"trading.entry.turbulence_confidence_scale",
		config.TurbulenceConfidenceScale,
	); err != nil {
		return err
	}

	if err := requirePositiveFinite(
		"trading.entry.temperature_confidence_scale",
		config.EntryTemperatureScale,
	); err != nil {
		return err
	}

	if err := requireUnitInterval(
		"trading.entry.confidence_ceiling",
		config.EntryConfidenceCeiling,
	); err != nil {
		return err
	}

	if config.EntryConfidenceBaseline > config.EntryConfidenceCeiling {
		return errors.New(
			"trading.entry.confidence_baseline must not exceed confidence_ceiling",
		)
	}

	return nil
}
