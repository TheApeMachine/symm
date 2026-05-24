package cmd

import (
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

func calibrationParamsFromConfig(cfg *config.Config) engine.CalibrationParams {
	params := engine.DefaultCalibrationParams()

	if cfg.MinCalibrationSamples > 0 {
		params.MinCalibrationSamples = cfg.MinCalibrationSamples
	}

	if cfg.MinConfidenceHistory > 0 {
		params.MinConfidenceHistory = cfg.MinConfidenceHistory
	}

	if cfg.CalibrationHalfLifeFloor > 0 {
		params.HalfLifeFloor = cfg.CalibrationHalfLifeFloor
	}

	if cfg.CalibrationHalfLifeCeiling > 0 {
		params.HalfLifeCeiling = cfg.CalibrationHalfLifeCeiling
	}

	if cfg.CalibrationRunwayFactor > 0 {
		params.RunwayFactor = cfg.CalibrationRunwayFactor
	}

	return params
}
