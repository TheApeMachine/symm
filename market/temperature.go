package market

import (
	"math"

	"github.com/theapemachine/symm/telemetry"
)

/*
MacroTemperature derives a 0..1 macro heat gauge from the live surprise index
and the dynamics envelope. Hot markets raise micro trigger thresholds instead
of voting alongside them in the playbook.
*/
func MacroTemperature() (float64, bool) {
	envelope, err := LoadDynamicsEnvelope()

	if err != nil {
		return 0, false
	}

	if envelope.AnomalySigma <= 0 {
		return 0, false
	}

	surprise := telemetry.MarketSurpriseIndex()

	if surprise <= 0 || math.IsNaN(surprise) || math.IsInf(surprise, 0) {
		return 0, false
	}

	temperature := surprise / envelope.AnomalySigma

	if temperature < 0 {
		return 0, false
	}

	if temperature > 1 {
		return 1, true
	}

	return temperature, true
}
