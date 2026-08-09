package causal

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func testResonanceReading(
	thesis *types.Thesis,
	symbol string,
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
		Source:   types.SourceResonance,
		Symbol:   symbol,
		At:       thesis.At,
		Energy:   energy,
		Surprise: surprise,
		Forecast: forecast,
	}
}

func setCausalPrice(
	thesis *types.Thesis,
	symbol string,
	midpoint float64,
	at time.Time,
) {
	thesis.Measurements.Store(types.SourceCVD, []*types.Measurement{{
		Source: types.SourceCVD,
		Symbol: symbol,
		At:     at,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricMidpoint, types.SideNone): {
				Raw:  midpoint,
				Unit: types.UnitQuoteCurrency,
			},
		},
	}})
}

func TestUpdate(t *testing.T) {
	convey.Convey("Given a predictive-coding reading without a forecast", t, func() {
		solver := NewSolver(nil, nil, nil)
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Resonance.Store("BTC/USD", types.ResonanceReading{
			Source: types.SourceResonance,
			Symbol: "BTC/USD",
			At:     thesis.At,
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

	convey.Convey("Given a forecast followed by a later executable midpoint", t, func() {
		solver := NewSolver(nil, nil, nil)
		thesis := types.NewThesis(t.Context(), nil)
		symbol := "BTC/USD"
		firstAt := time.Unix(1, 0)
		thesis.Resonance.Store(symbol, testResonanceReading(
			thesis, symbol, 0.5, 0.25, []float64{0.1},
		))
		thesis.Readiness.Stamp(types.SourceResonance)
		setCausalPrice(thesis, symbol, 100, firstAt)
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		convey.So(solver.Update(thesis), convey.ShouldBeNil)
		_, found := thesis.Causal.Load(symbol)
		convey.So(found, convey.ShouldBeFalse)

		thesis.At = thesis.At.Add(time.Second)
		thesis.Resonance.Store(symbol, testResonanceReading(
			thesis, symbol, 0.75, 0.5, []float64{0.2},
		))
		setCausalPrice(thesis, symbol, 110, firstAt.Add(time.Second))
		err := solver.Update(thesis)

		convey.Convey("Then it should score only the strictly prior forecast", func() {
			convey.So(err, convey.ShouldBeNil)
			stored, found := thesis.Causal.Load(symbol)
			convey.So(found, convey.ShouldBeTrue)
			output := stored.(map[string]any)
			rows := output["historyRows"].([][]float64)
			convey.So(output["samples"], convey.ShouldEqual, 1)
			convey.So(output["precision"], convey.ShouldEqual, 0.0)
			convey.So(output["treatmentLevel"], convey.ShouldEqual, 0.2)
			convey.So(output["identification"], convey.ShouldEqual, "adjustedAssociation")
			convey.So(rows, convey.ShouldHaveLength, 1)
			convey.So(rows[0][0], convey.ShouldEqual, 0.5)
			convey.So(rows[0][1], convey.ShouldEqual, 0.25)
			convey.So(rows[0][2], convey.ShouldEqual, 0.1)
			convey.So(rows[0][3], convey.ShouldAlmostEqual, math.Log(110.0/100.0), 1e-12)
			_, hasAssociation := output["association"]
			convey.So(hasAssociation, convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given a causal evidence stream for one symbol", t, func() {
		solver := NewSolver(nil, nil, nil)
		thesis := types.NewThesis(t.Context(), nil)
		symbol := "BTC/USD"
		baseAt := time.Unix(1, 0)
		midpoint := 100.0
		previousEnergy := 0.0
		previousSurprise := 0.0
		previousPrediction := 0.0
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		convey.Convey("It should retain forward rows and report finite-sample precision", func() {
			for index := range 13 {
				if index > 0 {
					realizedReturn := 0.5*previousEnergy +
						0.25*previousSurprise + 2*previousPrediction
					midpoint *= math.Exp(realizedReturn)
				}

				energy := float64(index%3) / 1_000
				surprise := float64((index*2)%5) / 1_000
				prediction := float64(index+1) / 1_000
				thesis.At = baseAt.Add(time.Duration(index) * time.Second)
				thesis.Resonance.Store(symbol, testResonanceReading(
					thesis, symbol, energy, surprise, []float64{prediction},
				))
				thesis.Readiness.Stamp(types.SourceResonance)
			setCausalPrice(thesis, symbol, midpoint, thesis.At)

				err := solver.Update(thesis)
				convey.So(err, convey.ShouldBeNil)
				previousEnergy = energy
				previousSurprise = surprise
				previousPrediction = prediction
			}

			stored, found := thesis.Causal.Load(symbol)
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
	solver := NewSolver(nil, nil, nil)
	b.Cleanup(func() {
		if err := solver.Close(); err != nil {
			b.Fatal(err)
		}
	})
	thesis := types.NewThesis(b.Context(), nil)
	baseAt := time.Unix(1, 0)
	symbols := make([]string, 640)

	for index := range symbols {
		symbol := fmt.Sprintf("SYMBOL-%03d/USD", index)
		symbols[index] = symbol
		thesis.Resonance.Store(symbol, testResonanceReading(
			thesis,
			symbol,
			float64(index),
			float64(index)/float64(index+1),
			[]float64{float64(index) / float64(index+1)},
		))
		setCausalPrice(thesis, symbol, 100, baseAt)
	}

	thesis.Readiness.Stamp(types.SourceResonance)

	if err := solver.Update(thesis); err != nil {
		b.Fatal(err)
	}

	tick := 1
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()
		thesis.At = thesis.At.Add(time.Second)
		at := baseAt.Add(time.Duration(tick) * time.Second)

		for index, symbol := range symbols {
			thesis.Resonance.Store(symbol, testResonanceReading(
				thesis,
				symbol,
				float64(index),
				float64(index)/float64(index+1),
				[]float64{float64(index) / float64(index+1)},
			))
			setCausalPrice(thesis, symbol, 100+float64(tick)/1_000, at)
		}

		b.StartTimer()

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}

		tick++
	}
}
