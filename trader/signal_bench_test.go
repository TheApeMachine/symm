package trader

import (
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

func BenchmarkSignalMeasure(benchmark *testing.B) {
	errnie.Apply(&errnie.Config{Level: "panic"})
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal, tree := newTraderSignal(benchmark)
		warmupTraderPumpDump(tree)
		insertTraderTicker(tree, "ETH/USD", 11000, 41000, 40990, 41010)

		measurements := signal.Measure()

		if len(measurements) == 0 {
			benchmark.Fatal("Measure returned no measurements")
		}

		_ = signal.Close()
	}
}

func BenchmarkSignalMeasureEach(benchmark *testing.B) {
	errnie.Apply(&errnie.Config{Level: "panic"})
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal, tree := newTraderSignal(benchmark)
		warmupTraderPumpDump(tree)
		insertTraderTicker(tree, "ETH/USD", 11000, 41000, 40990, 41010)

		count := 0
		signal.MeasureEach(func(*datura.Artifact) {
			count++
		})

		if count == 0 {
			benchmark.Fatal("MeasureEach emitted no measurements")
		}

		_ = signal.Close()
	}
}
