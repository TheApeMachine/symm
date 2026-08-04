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
	convey.Convey("Given one failed symbol and one resolved symbol", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis()
		output := map[string]any{"effect": 0.5}
		results := []causalResult{
			{symbol: "BAD/USD", err: errors.New("bad symbol")},
			{symbol: "GOOD/USD", output: output},
		}

		convey.Convey("It should skip the failure without dropping the resolved symbol", func() {
			resolved := solver.store(thesis, results)
			stored, found := thesis.Causal.Load("GOOD/USD")
			_, failedStored := thesis.Causal.Load("BAD/USD")

			convey.So(resolved, convey.ShouldBeTrue)
			convey.So(found, convey.ShouldBeTrue)
			convey.So(stored, convey.ShouldResemble, output)
			convey.So(failedStored, convey.ShouldBeFalse)
		})
	})
}

func BenchmarkUpdate(b *testing.B) {
	solver := NewSolver(nil, nil)
	thesis := types.NewThesis()
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
		thesis.Resonance.Store(symbol, map[string]any{
			"energy":       float64(index),
			"surprise":     float64(index) / float64(index+1),
			"forwardCurve": []float64{float64(index) / float64(index+1)},
		})
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
