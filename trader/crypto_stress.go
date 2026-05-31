package trader

import (
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market/perspectives"
)

func (crypto *Crypto) maxStressRegime() broker.StressRegime {
	now := time.Now()
	regime := broker.StressRegime{}

	crypto.mu.RLock()
	defer crypto.mu.RUnlock()

	for _, set := range crypto.readings {
		measurements := make([]perspectives.Measurement, 0, len(set))

		for _, reading := range set {
			if reading.Stale(now) {
				continue
			}

			measurements = append(measurements, reading.Measurement)
		}

		sample := broker.StressRegimeFrom(measurements)

		if sample.Turbulence > regime.Turbulence {
			regime.Turbulence = sample.Turbulence
		}

		if sample.Vorticity > regime.Vorticity {
			regime.Vorticity = sample.Vorticity
		}
	}

	return regime
}

func (crypto *Crypto) advanceStressMachine() {
	if !crypto.scopedRuntime().Execution.ExecutionStressEnabled {
		return
	}

	broker.GlobalStressMachine().Advance(crypto.maxStressRegime(), time.Now())
}
