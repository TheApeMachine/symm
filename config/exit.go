package config

import "github.com/spf13/viper"

/*
ExitConfig carries trailing-stop tuning loaded once at boot.
*/
const defaultSpreadScale = 0.5

type ExitConfig struct {
	TrailDefault      float64
	StopFloor         float64
	SpreadScale       float64
	MaxInitialRiskPct float64
	MaxTrailPct       float64
	MinTrailPct       float64
}

func LoadExitConfig() (ExitConfig, error) {
	trailDefault := exitFloat("trail_default", 0.015)

	return ExitConfig{
		TrailDefault:      trailDefault,
		StopFloor:         exitFloat("stop_floor", 0.012),
		SpreadScale:       exitFloat("spread_scale", defaultSpreadScale),
		MaxInitialRiskPct: exitFloat("max_initial_risk_pct", trailDefault),
		MaxTrailPct:       exitFloat("max_trail_pct", 0.05),
		MinTrailPct:       exitFloat("min_trail_pct", 0.008),
	}, nil
}

func (config ExitConfig) Float(key string, fallback float64) float64 {
	switch key {
	case "trail_default":
		if config.TrailDefault > 0 {
			return config.TrailDefault
		}
	case "stop_floor":
		if config.StopFloor > 0 {
			return config.StopFloor
		}
	case "spread_scale":
		if config.SpreadScale > 0 {
			return config.SpreadScale
		}
	}

	return fallback
}

func exitFloat(key string, fallback float64) float64 {
	value := viper.GetFloat64("trading.exit." + key)

	if value <= 0 {
		return fallback
	}

	return value
}
