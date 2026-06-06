package replay

import (
	"os"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
	optimizerio "github.com/theapemachine/symm/optimizer/io"
)

func TestPrecompileTapeRingWindow(t *testing.T) {
	convey.Convey("Given more than StoryRingCapacity measurements", t, func() {
		rows := make([]types.Measurement, 0, 1024+10)

		for index := range 1024 + 10 {
			rows = append(rows, types.Measurement{
				Symbol:   "BTC/EUR",
				Source:   types.SourceFluid,
				Category: types.CategoryLaminar,
				SNR:      float64(index),
				Last:     100 + float64(index),
			})
		}

		tape := PrecompileTape(rows)
		snapshots := tape.AppendSnapshot(len(rows)-1, nil)

		convey.Convey("It should cap the decision snapshot to the story ring size", func() {
			convey.So(len(snapshots), convey.ShouldEqual, 1024)
			convey.So(snapshots[0].SNR, convey.ShouldEqual, 10)
		})
	})
}

func TestReplayTapeAppendSnapshot(t *testing.T) {
	convey.Convey("Given interleaved symbol and global rows", t, func() {
		rows := []types.Measurement{
			{Symbol: "BTC/EUR", SNR: 1, Last: 100},
			{Symbol: "", Category: types.CategoryRiskOnSurge, SNR: 2},
			{Symbol: "ETH/EUR", SNR: 3, Last: 200},
			{Symbol: "BTC/EUR", SNR: 4, Last: 101},
		}

		tape := PrecompileTape(rows)
		snapshots := tape.AppendSnapshot(3, make([]types.Measurement, 0, 4))

		convey.Convey("It should return chronological global and matching-symbol rows only", func() {
			convey.So(len(snapshots), convey.ShouldEqual, 3)
			convey.So(snapshots[0].SNR, convey.ShouldEqual, 1)
			convey.So(snapshots[1].SNR, convey.ShouldEqual, 2)
			convey.So(snapshots[2].SNR, convey.ShouldEqual, 4)
		})
	})
}

func benchmarkTapeRows(count int) []types.Measurement {
	rows := make([]types.Measurement, 0, count)

	for index := range count {
		rows = append(rows, types.Measurement{
			Symbol:   "BTC/EUR",
			Source:   types.SourceFluid,
			Category: types.CategoryLaminar,
			SNR:      float64(index),
			Last:     100 + float64(index),
		})
	}

	return rows
}

func TestPrecompileTapeParallelMatchesSequential(t *testing.T) {
	convey.Convey("Given a large interleaved tape", t, func() {
		rowCount := parallelPrecompileRowThreshold + 128
		rows := mixedSymbolRows(rowCount)

		sequential := precompileTapeSequential(rows)
		parallel := PrecompileTapeWorkers(rows, 8)

		convey.Convey("It should build identical snapshot indices", func() {
			assertMatchingTapes(sequential, parallel)
		})
	})
}

func TestPrecompileTapeParallelPassOneMatchesSequential(t *testing.T) {
	convey.Convey("Given a large interleaved tape", t, func() {
		rowCount := parallelPrecompileRowThreshold + 128
		rows := mixedSymbolRows(rowCount)

		sequential := precompileTapeSequential(rows)
		parallelTicks := make([]PrecompiledTick, rowCount)
		chunks := make([]precompileChunk, 8)
		chunkSize := (rowCount + 7) / 8

		for workerIndex := range 8 {
			startIndex := workerIndex * chunkSize
			endIndex := startIndex + chunkSize

			if endIndex > rowCount {
				endIndex = rowCount
			}

			chunks[workerIndex] = buildPrecompileChunk(rows, parallelTicks, startIndex, endIndex)
		}

		symbolIndices, globalIndices, categories, lastPrices := mergePrecompileChunks(chunks)
		precompileSnapshotIndices(parallelTicks, symbolIndices, globalIndices, 8)

		parallel := ReplayTape{
			Ticks:      parallelTicks,
			LastPrices: lastPrices,
		}

		convey.Convey("It should merge pass-one indices in chronological order", func() {
			convey.So(globalIndices, convey.ShouldResemble, collectGlobalIndices(sequential.Ticks))
			convey.So(len(categories), convey.ShouldBeGreaterThan, 0)
			convey.So(lastPrices, convey.ShouldResemble, sequential.LastPrices)
			assertMatchingTapes(sequential, parallel)
		})
	})
}

func mixedSymbolRows(rowCount int) []types.Measurement {
	rows := make([]types.Measurement, 0, rowCount)

	for index := range rowCount {
		row := types.Measurement{
			Source:   types.SourceFluid,
			Category: types.CategoryLaminar,
			SNR:      float64(index),
			Last:     100 + float64(index),
		}

		if index%16 == 0 {
			row.Symbol = ""
		} else if index%2 == 0 {
			row.Symbol = "BTC/EUR"
		} else {
			row.Symbol = "ETH/EUR"
		}

		rows = append(rows, row)
	}

	return rows
}

func collectGlobalIndices(ticks []PrecompiledTick) []int {
	indices := make([]int, 0)

	for index, tick := range ticks {
		if tick.Row.Symbol == "" {
			indices = append(indices, index)
		}
	}

	return indices
}

func assertMatchingTapes(sequential, parallel ReplayTape) {
	convey.So(len(parallel.Ticks), convey.ShouldEqual, len(sequential.Ticks))

	for index := range sequential.Ticks {
		convey.So(
			parallel.Ticks[index].SnapshotIndices,
			convey.ShouldResemble,
			sequential.Ticks[index].SnapshotIndices,
		)
	}
}

func BenchmarkPrecompileTapeParallel(b *testing.B) {
	rows := benchmarkTapeRows(parallelPrecompileRowThreshold * 4)

	b.ReportMetric(float64(len(rows)), "rows")

	for _, workers := range []int{1, 0} {
		label := "workers=1"

		if workers == 0 {
			label = "workers=auto"
		}

		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				_ = PrecompileTapeWorkers(rows, workers)
			}
		})
	}
}

func BenchmarkPrecompileTape(b *testing.B) {
	rows := benchmarkTapeRows(1024 + 10)

	for b.Loop() {
		_ = PrecompileTape(rows)
	}
}

func BenchmarkAppendSnapshot(b *testing.B) {
	rows := benchmarkTapeRows(1024 * 64)
	tape := PrecompileTape(rows)
	tickIndex := len(rows) - 1
	scratch := make([]types.Measurement, 0, 1024)

	b.ReportMetric(float64(len(rows)), "rows")
	b.ResetTimer()

	for b.Loop() {
		scratch = tape.AppendSnapshot(tickIndex, scratch)
	}
}

func BenchmarkPrecompileTapeCapture(b *testing.B) {
	path := os.Getenv("SYMM_CAPTURE_PATH")

	if path == "" {
		b.Skip("SYMM_CAPTURE_PATH is required")
	}

	rows, skipped, err := optimizerio.LoadMeasurements(path)

	if err != nil {
		b.Fatal(err)
	}

	b.ReportMetric(float64(len(rows)), "rows/op")
	b.ReportMetric(float64(skipped), "skipped/op")
	b.ResetTimer()

	for b.Loop() {
		tape := PrecompileTape(rows)

		if tape.Len() != len(rows) {
			b.Fatalf("expected %d ticks, got %d", len(rows), tape.Len())
		}
	}
}
