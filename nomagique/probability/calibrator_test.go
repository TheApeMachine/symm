package probability_test

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestCalibratorRetention(t *testing.T) {
	tests.CheckCalibrator(t, probability.NewCalibrator(collection.NewTail[float64](transport.NewIO(core.From(4)))), 4)
	tests.CheckCalibrator(t, probability.NewCalibrator(transport.NewPipe()), 0)
	tests.CheckCalibratorPoison(t, func() core.Primitive { return probability.NewCalibrator(transport.NewPipe()) })
}

// This benchmark measures the current graph, not the retired zero-allocation API.
func BenchmarkCalibrator(b *testing.B) {
	node := probability.NewCalibrator(collection.NewTail[float64](transport.NewIO(core.From(4))))
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		input := transport.NewIO(core.From(1.234))
		if node.Next(input) == nil || node.Next(input) != nil {
			b.Fatal("expected one calibrator record")
		}
	}
	if err := node.Error(); err != nil {
		b.Fatal(err)
	}
}
