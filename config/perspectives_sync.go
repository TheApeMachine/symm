package config

import "github.com/theapemachine/symm/market/perspectives"

func syncPerspectives(cfg *Config) {
	if cfg == nil || cfg.NoiseFloorSNR <= 0 {
		return
	}

	perspectives.SetNoiseFloorSNR(cfg.NoiseFloorSNR)
}
