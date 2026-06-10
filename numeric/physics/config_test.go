package physics

import (
	"math"
	"testing"
)

func TestCellVolumeRhoMin(t *testing.T) {
	config := productionTestConfig()
	cellVolume := config.CellVolume()

	if cellVolume <= 0 {
		t.Fatalf("cell volume must be positive, got %g", cellVolume)
	}

	expectedRhoMin := 1.0 / cellVolume

	if math.Abs(config.RhoMin-expectedRhoMin) > 1e-12 {
		t.Fatalf("rhoMin = %g, want %g", config.RhoMin, expectedRhoMin)
	}

	expectedPMin := (config.Gamma - 1.0) * config.RhoMin * cellVolume

	if math.Abs(config.PMin-expectedPMin) > 1e-12 {
		t.Fatalf("pMin = %g, want %g", config.PMin, expectedPMin)
	}
}
