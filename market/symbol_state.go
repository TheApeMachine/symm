package market

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic"
)

/*
SymbolState keeps the latest measurement per signal source for playbook eval.
*/
type SymbolState struct {
	latest map[logic.SourceType]logic.Measurement
}

func NewSymbolState() *SymbolState {
	return &SymbolState{
		latest: make(map[logic.SourceType]logic.Measurement),
	}
}

/*
Observe records one measurement and returns the current fresh snapshot.
*/
func (state *SymbolState) Observe(measurement logic.Measurement) []logic.Measurement {
	if state == nil {
		return nil
	}

	if measurement.Source == "" {
		return state.Snapshot()
	}

	if measurement.ObservedAt.IsZero() {
		measurement.ObservedAt = time.Now().UTC()
	}

	state.latest[measurement.Source] = measurement

	return state.Snapshot()
}

/*
Snapshot returns the latest measurement per source inside the configured TTL.
*/
func (state *SymbolState) Snapshot() []logic.Measurement {
	if state == nil {
		return nil
	}

	maxAge := viper.GetDuration("market.story.measurement_max_age")

	if maxAge <= 0 {
		maxAge = 30 * time.Second
	}

	now := time.Now().UTC()
	measurements := make([]logic.Measurement, 0, len(state.latest))

	for _, measurement := range state.latest {
		if measurement.ObservedAt.IsZero() {
			continue
		}

		if now.Sub(measurement.ObservedAt) > maxAge {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements
}
