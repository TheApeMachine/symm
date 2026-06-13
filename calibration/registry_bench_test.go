package calibration

import (
	"testing"

	"github.com/theapemachine/symm/logic"
)

func BenchmarkRegistryRecord(b *testing.B) {
	registry := NewRegistry()
	target := CalibrationTarget{
		Source:           logic.SourcePumpDump,
		Category:         logic.CategoryVerticalIgnition,
		PredictedMoveBps: 40,
		RealizedMoveBps:  20,
		CostBps:          10,
	}

	b.ReportAllocs()

	for b.Loop() {
		registry.Record(target, 0.7)
	}
}
