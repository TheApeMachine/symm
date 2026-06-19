package response

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"time"

	"github.com/spf13/viper"
)

/*
EffectiveNetworkLatency samples paper execution delay from the latency profile.
Falls back to trading.replay.execution_latency_ms when the profile is missing.
*/
func EffectiveNetworkLatency() time.Duration {
	path := viper.GetString("trading.paper.latency_profile")

	if path != "" {
		if latency := latencyFromProfile(path); latency > 0 {
			return latency
		}
	}

	ms := viper.GetInt("trading.replay.execution_latency_ms")

	if ms <= 0 {
		return 0
	}

	return time.Duration(ms) * time.Millisecond
}

func latencyFromProfile(path string) time.Duration {
	payload, readErr := os.ReadFile(path)

	if readErr != nil || len(payload) == 0 {
		return 0
	}

	var profile struct {
		Samples []float64 `json:"samples_ms"`
	}

	if json.Unmarshal(payload, &profile) != nil || len(profile.Samples) == 0 {
		return 0
	}

	index := rand.IntN(len(profile.Samples))
	sample := profile.Samples[index]

	if sample <= 0 {
		return 0
	}

	return time.Duration(sample * float64(time.Millisecond))
}
