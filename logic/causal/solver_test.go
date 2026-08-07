package causal

import (
	"fmt"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func testResonanceReading(
	energy, surprise float64,
	curve []float64,
) types.ResonanceReading {
	retention := make([]float64, len(curve))

	for index := range retention {
		retention[index] = 1
	}

	forecast, err := types.NewResonanceForecast(
		curve, retention, len(curve), 0.75,
	)

	if err != nil {
		panic(err)
	}

	return types.ResonanceReading{
		Energy: energy, Surprise: surprise, Forecast: forecast,
	}
}

func TestUpdate(t *testing.T) {
	convey.Convey("Given a causal evidence stream for one symbol", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis(nil)
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		convey.Convey("It should evaluate on its pool and store only Pearl output on the thesis", func() {
			for index := range 12 {
				energy := float64(index % 3)
				surprise := float64((index * 2) % 5)
				prediction := float64(index + 1)
				realizedReturn := 0.5*energy + 0.25*surprise + 2*prediction
				thesis.Resonance.Store("BTC/USD", testResonanceReading(
					energy, surprise, []float64{prediction},
				))
				thesis.Measurements.Store(types.SourceSentiment, []*types.Measurement{{
					Source: types.SourceSentiment,
					Symbol: "BTC/USD",
					At:     time.Unix(int64(index+1), 0),
					Metrics: map[string]types.MetricSample{
						types.MetricKey(types.MetricChange, types.SideNone): {Raw: realizedReturn},
					},
				}})

				err := solver.Update(thesis)
				convey.So(err, convey.ShouldBeNil)
			}

			stored, found := thesis.Causal.Load("BTC/USD")
			convey.So(found, convey.ShouldBeTrue)
			output := stored.(map[string]any)

			convey.So(solver.pool.SubmittedTasks(), convey.ShouldEqual, uint64(12))
			convey.So(output["ready"], convey.ShouldEqual, true)
			convey.So(output["association"], convey.ShouldNotBeNil)
			convey.So(output["historyRows"], convey.ShouldBeNil)
		})
	})
}

func BenchmarkUpdate(b *testing.B) {
	solver := NewSolver(nil, nil)
	b.Cleanup(func() {
		if err := solver.Close(); err != nil {
			b.Fatal(err)
		}
	})
	thesis := types.NewThesis(nil)
	measurements := make([]*types.Measurement, 0, 640)

	for index := range 640 {
		symbol := fmt.Sprintf("SYMBOL-%03d/USD", index)
		measurements = append(measurements, &types.Measurement{
			Source: types.SourceSentiment,
			Symbol: symbol,
			At:     time.Unix(1, 0),
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricChange, types.SideNone): {
					Raw: float64(index) / float64(len(measurements)+1),
				},
			},
		})
		thesis.Resonance.Store(symbol, testResonanceReading(
			float64(index),
			float64(index)/float64(index+1),
			[]float64{float64(index) / float64(index+1)},
		))
	}

	thesis.Measurements.Store(types.SourceSentiment, measurements)
	b.ResetTimer()

	for b.Loop() {
		err := solver.Update(thesis)

		if err != nil {
			b.Fatal(err)
		}
	}
}
