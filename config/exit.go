package config

import "github.com/spf13/viper"

/*
ExitConfig carries trailing-stop tuning loaded once at boot.
*/
const defaultSpreadScale = 0.5

type ExitConfig struct {
	TrailDefault float64
	StopFloor    float64
	SpreadScale  float64
}

func LoadExitConfig() (ExitConfig, error) {
	return ExitConfig{
		TrailDefault: exitFloat("trail_default", 0.015),
		StopFloor:    exitFloat("stop_floor", 0.012),
		SpreadScale:  exitFloat("spread_scale", defaultSpreadScale),
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
