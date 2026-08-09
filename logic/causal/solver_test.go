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
	convey.Convey("Given a predictive-coding reading without a forecast", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Resonance.Store("BTC/USD", types.ResonanceReading{
			Source: types.SourceResonance,
			Symbol: "BTC/USD",
		})
		thesis.Readiness.Stamp(types.SourceResonance)
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		err := solver.Update(thesis)

		convey.Convey("Then causal should complete without inventing an estimate", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(thesis.Readiness.Causal, convey.ShouldBeTrue)
			_, found := thesis.Causal.Load("BTC/USD")
			convey.So(found, convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given the first aligned causal observation", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Resonance.Store("BTC/USD", testResonanceReading(
			0.5,
			0.25,
			[]float64{0.1},
		))
		thesis.Readiness.Stamp(types.SourceResonance)
		thesis.Measurements.Store(types.SourceSentiment, []*types.Measurement{{
			Source: types.SourceSentiment,
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricChange, types.SideNone): {Raw: 0.2},
			},
		}})
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		err := solver.Update(thesis)

		convey.Convey("Then it should publish the estimate with zero precision", func() {
			convey.So(err, convey.ShouldBeNil)
			stored, found := thesis.Causal.Load("BTC/USD")
			convey.So(found, convey.ShouldBeTrue)
			output := stored.(map[string]any)
			convey.So(output["samples"], convey.ShouldEqual, 1)
			convey.So(output["precision"], convey.ShouldEqual, 0.0)
			convey.So(output["treatmentLevel"], convey.ShouldEqual, 0.1)
			convey.So(output["historyRows"], convey.ShouldHaveLength, 1)
			_, hasAssociation := output["association"]
			convey.So(hasAssociation, convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given a causal evidence stream for one symbol", t, func() {
		solver := NewSolver(nil, nil)
		thesis := types.NewThesis(t.Context(), nil)
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		convey.Convey("It should retain rows and report finite-sample precision", func() {
			for index := range 12 {
				energy := float64(index % 3)
				surprise := float64((index * 2) % 5)
				prediction := float64(index + 1)
				realizedReturn := 0.5*energy + 0.25*surprise + 2*prediction
				thesis.Resonance.Store("BTC/USD", testResonanceReading(
					energy, surprise, []float64{prediction},
				))
				thesis.Readiness.Stamp(types.SourceResonance)
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

			convey.So(output["association"], convey.ShouldNotBeNil)
			convey.So(output["samples"], convey.ShouldEqual, 12)
			convey.So(output["precision"], convey.ShouldBeGreaterThan, 0.0)
			convey.So(output["precision"], convey.ShouldBeLessThan, 1.0)
			rows, rowsOK := output["historyRows"].([][]float64)
			convey.So(rowsOK, convey.ShouldBeTrue)
			convey.So(rows, convey.ShouldHaveLength, 12)
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
	thesis := types.NewThesis(b.Context(), nil)
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
	thesis.Readiness.Stamp(types.SourceResonance)
	b.ResetTimer()

	for b.Loop() {
		err := solver.Update(thesis)

		if err != nil {
			b.Fatal(err)
		}
	}
}
