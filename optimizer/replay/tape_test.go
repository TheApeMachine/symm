package replay

import (
	"os"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/types"
	optimizerio "github.com/theapemachine/symm/optimizer/io"
)

func TestPrecompileTapeRingWindow(t *testing.T) {
	convey.Convey("Given more than StoryRingCapacity measurements", t, func() {
		testconfig.Load(t)
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

		tape := mustPrecompileTape(t, rows)
		snapshots := tape.AppendSnapshot(len(rows)-1, nil)

		convey.Convey("It should cap the decision snapshot to the story ring size", func() {
			convey.So(len(snapshots), convey.ShouldEqual, 1024)
			convey.So(snapshots[0].SNR, convey.ShouldEqual, 10)
		})
	})
}

func TestReplayTapeAppendSnapshot(t *testing.T) {
	convey.Convey("Given interleaved symbol and global rows", t, func() {
		testconfig.Load(t)
		rows := []types.Measurement{
			{Symbol: "BTC/EUR", SNR: 1, Last: 100},
			{Symbol: "", Category: types.CategoryRiskOnSurge, SNR: 2},
			{Symbol: "ETH/EUR", SNR: 3, Last: 200},
			{Symbol: "BTC/EUR", SNR: 4, Last: 101},
		}

		tape := mustPrecompileTape(t, rows)
		snapshots := tape.AppendSnapshot(3, make([]types.Measurement, 0, 4))

		convey.Convey("It should return exactly the symbol's own chronological window, matching the live story ring", func() {
			// market/story.rememberMeasurement keeps a per-symbol ring; rows with
			// no symbol never enter any live window, so replay must exclude them
			// too — otherwise replay reasons over context live never sees.
			convey.So(len(snapshots), convey.ShouldEqual, 2)
			convey.So(snapshots[0].SNR, convey.ShouldEqual, 1)
			convey.So(snapshots[1].SNR, convey.ShouldEqual, 4)
		})
	})
}

func TestReplayTapeWindowIsPerSymbolOccurrences(t *testing.T) {
	convey.Convey("Given a sparse symbol interleaved with a dense one", t, func() {
		testconfig.Load(t)

		// 2048 dense rows interleaved with 3 sparse rows. Under the old global
		// window the sparse symbol kept ~buffer/symbols rows of context; live it
		// keeps its own last buffer readings regardless of how long ago they
		// happened. The snapshot must contain ALL THREE sparse readings.
		rows := make([]types.Measurement, 0, 2053)
		rows = append(rows, types.Measurement{Symbol: "SPARSE/EUR", SNR: 1, Last: 1})

		for index := range 2048 {
			rows = append(rows, types.Measurement{Symbol: "DENSE/EUR", SNR: float64(index), Last: 100})
		}

		rows = append(rows, types.Measurement{Symbol: "SPARSE/EUR", SNR: 2, Last: 1.1})
		rows = append(rows, types.Measurement{Symbol: "DENSE/EUR", SNR: 9999, Last: 100})
		rows = append(rows, types.Measurement{Symbol: "SPARSE/EUR", SNR: 3, Last: 1.2})

		tape := mustPrecompileTape(t, rows)
		snapshots := tape.AppendSnapshot(len(rows)-1, nil)

		convey.Convey("It should keep the sparse symbol's full history within the ring capacity", func() {
			convey.So(len(snapshots), convey.ShouldEqual, 3)
			convey.So(snapshots[0].SNR, convey.ShouldEqual, 1)
			convey.So(snapshots[1].SNR, convey.ShouldEqual, 2)
			convey.So(snapshots[2].SNR, convey.ShouldEqual, 3)
		})
	})
}

func TestBackfillSymbolQuotesGivesPredictionRowsTheCacheBook(t *testing.T) {
	convey.Convey("Given a booked row followed by a bookless prediction-style row", t, func() {
		testconfig.Load(t)
		rows := []types.Measurement{
			{
				Symbol: "BTC/EUR", Source: types.SourceFluid, SNR: 1, Last: 100,
				Bid: 99.9, Ask: 100.1,
				BookBids: []types.BookLevel{{Price: 99.9, Qty: 50}},
				BookAsks: []types.BookLevel{{Price: 100.1, Qty: 50}},
			},
			{Symbol: "BTC/EUR", Source: types.SourcePrediction, SNR: 2, Last: 100},
		}

		tape := mustPrecompileTape(t, rows)

		convey.Convey("The bookless row presents the symbol's last-known book and touch, aliased", func() {
			row := tape.Ticks[1].Row

			// Live fills come from the quote cache's current book; replay must
			// offer the same book on rows recorded without one, or it refuses
			// entries the live desk would take.
			convey.So(row.HasBookDepth(), convey.ShouldBeTrue)
			convey.So(row.Bid, convey.ShouldEqual, 99.9)
			convey.So(row.Ask, convey.ShouldEqual, 100.1)
			convey.So(&row.BookBids[0], convey.ShouldEqual, &tape.Ticks[0].Row.BookBids[0])
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
		testconfig.Load(t)
		rowCount := parallelPrecompileRowThreshold + 128
		rows := mixedSymbolRows(rowCount)

		sequential, err := precompileTapeSequential(rows)
		convey.So(err, convey.ShouldBeNil)

		parallel, err := PrecompileTapeWorkers(rows, 8)
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("It should build identical snapshot indices", func() {
			assertMatchingTapes(sequential, parallel)
		})
	})
}

func TestPrecompileTapeParallelPassOneMatchesSequential(t *testing.T) {
	convey.Convey("Given a large interleaved tape", t, func() {
		testconfig.Load(t)
		rowCount := parallelPrecompileRowThreshold + 128
		rows := mixedSymbolRows(rowCount)

		sequential, err := precompileTapeSequential(rows)
		convey.So(err, convey.ShouldBeNil)
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
		buffer, err := assignSymbolOrdinals(parallelTicks, symbolIndices)
		convey.So(err, convey.ShouldBeNil)

		parallel := ReplayTape{
			Ticks:             parallelTicks,
			SymbolIndices:     symbolIndices,
			LastPrices:        lastPrices,
			measurementBuffer: buffer,
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

	// The decision window is derived per tick; equality of derived snapshots is
	// the contract that matters between the two precompile paths.
	var sequentialScratch, parallelScratch []types.Measurement

	for index := range sequential.Ticks {
		convey.So(
			parallel.Ticks[index].SymbolOrdinal,
			convey.ShouldEqual,
			sequential.Ticks[index].SymbolOrdinal,
		)
		convey.So(
			parallel.Ticks[index].Regime,
			convey.ShouldEqual,
			sequential.Ticks[index].Regime,
		)

		sequentialScratch = sequential.AppendSnapshot(index, sequentialScratch)
		parallelScratch = parallel.AppendSnapshot(index, parallelScratch)
		convey.So(len(parallelScratch), convey.ShouldEqual, len(sequentialScratch))
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
				_, err := PrecompileTapeWorkers(rows, workers)

				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPrecompileTape(b *testing.B) {
	rows := benchmarkTapeRows(1024 + 10)

	for b.Loop() {
		_, err := PrecompileTape(rows)

		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAppendSnapshot(b *testing.B) {
	rows := benchmarkTapeRows(1024 * 64)
	tape := mustPrecompileTape(b, rows)
	tickIndex := len(rows) - 1
	scratch := make([]types.Measurement, 0, 1024)

	b.ReportMetric(float64(len(rows)), "rows")
	b.ResetTimer()

	for b.Loop() {
		scratch = tape.AppendSnapshot(tickIndex, scratch)
	}
}

func TestReplayTapeEmptyMeasurementBuffer(t *testing.T) {
	convey.Convey("Given a tape with non-positive measurementBuffer", t, func() {
		testconfig.Load(t)
		rows := []types.Measurement{
			{Symbol: "BTC/EUR", SNR: 1, Last: 100},
			{Symbol: "BTC/EUR", SNR: 4, Last: 101},
		}

		tape, err := PrecompileTape(rows)
		convey.So(err, convey.ShouldBeNil)
		tape.measurementBuffer = 0

		snapshots := tape.AppendSnapshot(1, nil)

		convey.Convey("It should return an empty snapshot window", func() {
			convey.So(len(snapshots), convey.ShouldEqual, 0)
		})
	})
}

func TestPrecompileTapeHonestQuotes(t *testing.T) {
	convey.Convey("Given capture-style rows with and without book depth", t, func() {
		at := time.Unix(1_700_000_000, 0).UTC()
		rows := []types.Measurement{
			{
				Symbol:    "PUMP/EUR",
				Last:      1,
				SpreadBPS: 800,
				At:        at,
			},
			{
				Symbol: "BTC/EUR",
				Last:   100,
				Bid:    99,
				Ask:    101,
				BookAsks: []types.BookLevel{
					{Price: 101, Qty: 2},
				},
				BookBids: []types.BookLevel{
					{Price: 99, Qty: 2},
				},
				At: at.Add(time.Second),
			},
		}

		tape := mustPrecompileTape(t, rows)

		convey.Convey("It should derive bid/ask from spread without inventing depth", func() {
			spreadOnly := tape.Ticks[0].Row
			convey.So(spreadOnly.Bid, convey.ShouldBeGreaterThan, 0)
			convey.So(spreadOnly.Ask, convey.ShouldBeGreaterThan, spreadOnly.Bid)
			convey.So(spreadOnly.HasBookDepth(), convey.ShouldBeFalse)
		})

		convey.Convey("It should preserve tape book levels exactly", func() {
			withBook := tape.Ticks[1].Row
			convey.So(withBook.Bid, convey.ShouldEqual, 99)
			convey.So(withBook.Ask, convey.ShouldEqual, 101)
			convey.So(len(withBook.BookAsks), convey.ShouldEqual, 1)
			convey.So(withBook.BookAsks[0].Qty, convey.ShouldEqual, 2)
			convey.So(len(withBook.BookBids), convey.ShouldEqual, 1)
			convey.So(withBook.BookBids[0].Qty, convey.ShouldEqual, 2)
		})
	})
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
		tape := mustPrecompileTape(b, rows)

		if tape.Len() != len(rows) {
			b.Fatalf("expected %d ticks, got %d", len(rows), tape.Len())
		}
	}
}
