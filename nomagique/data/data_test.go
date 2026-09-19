package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestData(t *testing.T) {
	Convey("Given Extract atom", t, func() {
		extract := NewExtract(types.Const("price"))
		So(extract(map[string]any{"price": 42.5}), ShouldEqual, 42.5)
		So(extract(map[string]any{"other": 10.0}), ShouldEqual, 0.0)
	})

	Convey("Given Quality and Finalizer Value closures", t, func() {
		quality := NewQuality()

		facts := QualityFacts{
			Support:       10,
			HasSupport:    true,
			Divergence:    2.0,
			HasDivergence: true,
			NoiseVariance: 1.0,
			HasNoise:      true,
		}
		reading := quality(facts)

		So(reading.Estimated, ShouldBeTrue)
		So(reading.SNRDefined, ShouldBeTrue)
		So(reading.SNR, ShouldEqual, 4.0) // 2^2 / 1 = 4.0
		So(reading.Maturity, ShouldAlmostEqual, 0.9, 1e-6)

		measurement := NewMeasurement[float64]("test", nil)
		measurement.Metadata = map[string]string{
			MetadataSupport:       "10",
			MetadataDivergence:    "2.0",
			MetadataNoiseVariance: "1.0",
		}
		measurement.Finalize()

		So(measurement.Estimated, ShouldBeTrue)
		So(measurement.SNRDefined, ShouldBeTrue)
		So(measurement.SNR, ShouldEqual, 4.0)
		So(measurement.Maturity, ShouldAlmostEqual, 0.9, 1e-6)
	})

	Convey("Given Series storage closure", t, func() {
		series := NewSeries[float64](types.Const(3))

		// Observe key "btc"
		r1 := series(SeriesInput[float64]{
			Key:   "btc",
			Sec:   100,
			Nsec:  0,
			Value: 50000.0,
		})
		So(r1.Found, ShouldBeTrue)

		r2 := series(SeriesInput[float64]{
			Key:   "btc",
			Sec:   200,
			Nsec:  0,
			Value: 51000.0,
		})
		So(r2.Found, ShouldBeTrue)

		// Query as-of 150 -> should return 50000.0
		q1 := series(SeriesInput[float64]{
			Key:   "btc",
			Sec:   150,
			Nsec:  0,
			Query: true,
		})
		So(q1.Found, ShouldBeTrue)
		So(q1.Value, ShouldEqual, 50000.0)

		// Query as-of 250 -> should return 51000.0
		q2 := series(SeriesInput[float64]{
			Key:   "btc",
			Sec:   250,
			Nsec:  0,
			Query: true,
		})
		So(q2.Found, ShouldBeTrue)
		So(q2.Value, ShouldEqual, 51000.0)
	})

	Convey("Given Equations Value closure", t, func() {
		eq := NewEquations(
			NewUnaryEquation(types.Const("double_bid"), types.Const("bid"), func(in float64) float64 {
				return in * 2.0
			}),
			NewBinaryEquation(types.Const("spread"), types.Const("ask"), types.Const("bid"), types.Value[[2]float64, float64](arithmetic.NewSubtract())),
		)

		m := NewMeasurement[float64]("quote", nil)
		m.Metrics["bid"] = Metric[float64]{Label: "bid", Raw: 100.0}
		m.Metrics["ask"] = Metric[float64]{Label: "ask", Raw: 105.0}
		m.Metrics["double_bid"] = Metric[float64]{Label: "double_bid"}
		m.Metrics["spread"] = Metric[float64]{Label: "spread"}

		m = eq(m)

		So(m.Metrics["double_bid"].Raw, ShouldEqual, 200.0)
		So(m.Metrics["spread"].Raw, ShouldEqual, 5.0)
	})
}
