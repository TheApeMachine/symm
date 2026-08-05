package causal

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestStore(t *testing.T) {
	convey.Convey("Given one failed symbol, one warming symbol, and one resolved symbol", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis(nil)
		output := map[string]any{"effect": 0.5}
		solver.appendHistory("GOOD/USD", []float64{0.1, 0.2, 0.3, 0.4})
		solver.appendHistory("WARM/USD", []float64{0.5, 0.6, 0.7, 0.8})
		results := []causalResult{
			{symbol: "BAD/USD", err: errors.New("bad symbol")},
			{symbol: "GOOD/USD", output: output},
			{symbol: "WARM/USD", output: nil},
		}

		convey.Convey("It should skip the failure and persist history rows for all valid symbols", func() {
			resolved := solver.store(thesis, results)
			stored, found := thesis.Causal.Load("GOOD/USD")
			warmStored, warmFound := thesis.Causal.Load("WARM/USD")
			_, failedStored := thesis.Causal.Load("BAD/USD")

			convey.So(resolved, convey.ShouldBeTrue)
			convey.So(found, convey.ShouldBeTrue)
			convey.So(stored, convey.ShouldResemble, output)
			convey.So(warmFound, convey.ShouldBeTrue)
			convey.So(warmStored.(map[string]any)["historyRows"], convey.ShouldResemble, [][]float64{{0.5, 0.6, 0.7, 0.8}})
			convey.So(failedStored, convey.ShouldBeFalse)
			convey.So(output["historyRows"], convey.ShouldResemble, [][]float64{{0.1, 0.2, 0.3, 0.4}})
		})
	})
}

func TestAppendHistory(t *testing.T) {
	convey.Convey("Given causal rows for two symbols", t, func() {
		solver := NewSolver(nil, nil)
		rowWidth := 4
		capacity := 1 + rowWidth + rowWidth*(rowWidth+1)/2

		for index := range capacity + 3 {
			solver.appendHistory("BTC/USD", []float64{float64(index), 0.2, 0.3, 0.4})
		}

		solver.appendHistory("ETH/USD", []float64{7, 0.6, 0.5, 0.4})

		convey.Convey("It should retain a symbol-local moment-complete history", func() {
			bitcoinRows := solver.historyRows("BTC/USD")
			ethereumRows := solver.historyRows("ETH/USD")

			convey.So(bitcoinRows, convey.ShouldHaveLength, capacity)
			convey.So(bitcoinRows[0][0], convey.ShouldEqual, 3.0)
			convey.So(bitcoinRows[len(bitcoinRows)-1][0], convey.ShouldEqual, float64(capacity+2))
			convey.So(ethereumRows, convey.ShouldResemble, [][]float64{{7, 0.6, 0.5, 0.4}})
		})
	})
}

func TestHistoryRows(t *testing.T) {
	convey.Convey("Given a stored causal row", t, func() {
		solver := NewSolver(nil, nil)
		original := []float64{0.1, 0.2, 0.3, 0.4}
		solver.appendHistory("BTC/USD", original)
		snapshot := solver.historyRows("BTC/USD")

		convey.Convey("It should isolate stored and returned rows from mutation", func() {
			original[0] = 9
			snapshot[0][1] = 8
			stored := solver.historyRows("BTC/USD")

			convey.So(stored, convey.ShouldResemble, [][]float64{{0.1, 0.2, 0.3, 0.4}})
		})
	})
}

func BenchmarkUpdate(b *testing.B) {
	solver := NewSolver(nil, nil)
	thesis := types.NewThesis(nil)
	measurements := make([]*types.Measurement, 0, 640)

	for index := range 640 {
		symbol := fmt.Sprintf("SYMBOL-%03d/USD", index)
		measurements = append(measurements, &types.Measurement{
			Source: types.SourceLiquidity,
			Symbol: symbol,
			At:     time.Unix(1, 0),
			Metrics: map[string]types.MetricSample{
				"return": {Raw: float64(index) / float64(len(measurements)+1)},
			},
		})
		thesis.Resonance.Store(symbol, testResonanceReading(
			&testing.T{},
			float64(index),
			float64(index)/float64(index+1),
			[]float64{float64(index) / float64(index+1)},
		))
	}

	thesis.Measurements.Store(types.SourceLiquidity, measurements)
	b.ResetTimer()

	for b.Loop() {
		err := solver.Update(thesis)

		if err != nil {
			b.Fatal(err)
		}
	}
}
