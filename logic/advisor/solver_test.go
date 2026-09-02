package advisor

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

func TestNewSolver(t *testing.T) {
	Convey("Given two class-bearing Features", t, func() {
		solver := NewSolver(t.Context(), midpointFeatures())

		Convey("the Solver becomes ready", func() {
			So(solver.Error(), ShouldBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.READY)
		})
	})

	Convey("Given a Feature without its one Class", t, func() {
		features := midpointFeatures()
		features[0].Class = nil
		solver := NewSolver(t.Context(), features)

		Convey("construction fails instead of inferring a Class", func() {
			So(solver.Error(), ShouldNotBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.FATAL)
		})
	})

	Convey("Given an ambiguous metric key", t, func() {
		features := midpointFeatures()
		features[0].Keys[0] = "midpoint_return_zscore"
		solver := NewSolver(t.Context(), features)

		Convey("construction fails instead of binding an arbitrary producer", func() {
			So(solver.Error(), ShouldNotBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.FATAL)
		})
	})

	Convey("Given Features declared on different market clocks", t, func() {
		features := midpointFeatures()
		features[1].Clock = "trades/event_ordinal"
		solver := NewSolver(t.Context(), features)

		Convey("construction fails instead of mixing incompatible observations", func() {
			So(solver.Error(), ShouldNotBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.FATAL)
		})
	})
}

func TestSolverStep(t *testing.T) {
	Convey("Given existing PumpDump midpoint-return observations", t, func() {
		solver := NewSolver(t.Context(), midpointFeatures())

		Convey("positive response favors the recovery Feature after conditioning", func() {
			conditionMidpointSolver(solver)
			envelope := advisorMeasurementEnvelope(2, 3, true, nil)
			So(solver.Step(envelope), ShouldEqual, envelope)

			distribution, sharpness, found, err := solver.Distribution("BTC/USD")
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(distribution, ShouldHaveLength, 2)
			So(distribution[0].State, ShouldEqual, types.PerspectiveState("recovery"))
			So(distribution[0].Probability, ShouldBeGreaterThan, distribution[1].Probability)
			So(distribution[0].Probability+distribution[1].Probability, ShouldAlmostEqual, 1)
			So(sharpness, ShouldBeGreaterThan, 0.0)
		})

		Convey("negative response moves mass to breakdown", func() {
			conditionMidpointSolver(solver)
			So(solver.Step(advisorMeasurementEnvelope(-2, 3, true, nil)), ShouldNotBeNil)

			distribution, _, found, err := solver.Distribution("BTC/USD")
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(distribution[1].State, ShouldEqual, types.PerspectiveState("breakdown"))
			So(distribution[1].Probability, ShouldBeGreaterThan, distribution[0].Probability)
		})

		Convey("an unrelated sparse envelope does not fabricate a distribution", func() {
			So(solver.Step(types.NewEnvelope(types.EnvelopeTicker)), ShouldNotBeNil)

			_, _, found, err := solver.Distribution("BTC/USD")
			So(err, ShouldBeNil)
			So(found, ShouldBeFalse)
		})

		Convey("an incomplete initial volume bar does not invoke classification", func() {
			So(solver.Step(advisorMeasurementEnvelope(1, 0, false, nil)), ShouldNotBeNil)
			So(solver.Error(), ShouldBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.READY)

			_, _, found, err := solver.Distribution("BTC/USD")
			So(err, ShouldBeNil)
			So(found, ShouldBeFalse)
		})

		Convey("an estimator-defined partial vector remains visibly waiting", func() {
			So(solver.Step(advisorMeasurementEnvelope(1, 1, false, nil)), ShouldNotBeNil)
			So(solver.Error(), ShouldBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.WAITING)

			So(solver.Step(advisorMeasurementEnvelope(1, 2, true, nil)), ShouldNotBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.READY)
		})

		Convey("a signal measurement error halts the Solver", func() {
			failure := errors.New("pumpdump failed")
			So(solver.Step(advisorMeasurementEnvelope(0, 1, true, failure)), ShouldBeNil)
			So(solver.Error(), ShouldNotBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.FATAL)
		})

		Convey("a feature observation without its declared clock halts the Solver", func() {
			envelope := advisorMeasurementEnvelope(1, 1, true, nil)
			delete(envelope.PumpDump.Metrics, "completed_volume_bar_ordinal")

			So(solver.Step(envelope), ShouldBeNil)
			So(solver.Error(), ShouldNotBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.FATAL)
		})

		Convey("a market clock moving backwards halts the Solver", func() {
			So(solver.Step(advisorMeasurementEnvelope(1, 2, true, nil)), ShouldNotBeNil)
			So(solver.Step(advisorMeasurementEnvelope(1, 1, true, nil)), ShouldBeNil)
			So(solver.Error(), ShouldNotBeNil)
			So(solver.Status().Current(), ShouldEqual, runtime.FATAL)
		})
	})
}

func TestSolverDistribution(t *testing.T) {
	Convey("Given an unconditioned equal class distribution", t, func() {
		solver := NewSolver(t.Context(), midpointFeatures())
		So(solver.Step(advisorMeasurementEnvelope(0, 1, true, nil)), ShouldNotBeNil)

		Convey("Distribution preserves both classes with zero sharpness", func() {
			distribution, sharpness, found, err := solver.Distribution("BTC/USD")
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(distribution, ShouldResemble, []types.PerspectiveClass{
				{State: "recovery", Probability: 0.5},
				{State: "breakdown", Probability: 0.5},
			})
			So(sharpness, ShouldAlmostEqual, 0.0)
		})
	})
}

func midpointFeatures() []*Feature {
	return []*Feature{
		NewFeature(
			"pumpdump/completed_volume_bar_ordinal",
			[]string{"pumpdump/positive_midpoint_return"},
			&Class{Label: "recovery"},
		),
		NewFeature(
			"pumpdump/completed_volume_bar_ordinal",
			[]string{"pumpdump/negative_midpoint_return"},
			&Class{Label: "breakdown"},
		),
	}
}

func conditionMidpointSolver(solver *Solver) {
	if solver.Step(advisorMeasurementEnvelope(1, 1, true, nil)) == nil {
		panic(solver.Error())
	}

	if solver.Step(advisorMeasurementEnvelope(-1, 2, true, nil)) == nil {
		panic(solver.Error())
	}
}

func advisorMeasurementEnvelope(
	value float64,
	ordinal uint64,
	complete bool,
	err error,
) *types.Envelope {
	at := time.Unix(1_700_000_000, 0)
	measurement := data.NewMeasurement[float64](
		"pumpdump:BTC/USD:1",
		"BTC/USD",
		"pumpdump",
		at,
		at,
	)
	measurement.Err = err
	positive := value
	negative := 0.0

	if value < 0 {
		positive = 0
		negative = -value
	}

	measurement.PutMetric(data.NewMetric(
		"completed_volume_bar_ordinal",
		float64(ordinal),
		nil,
		nil,
		data.UnitCount,
		data.TimescaleInstantaneous,
	))
	measurement.PutMetric(data.NewMetric(
		"positive_midpoint_return",
		positive,
		nil,
		nil,
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
	))

	if complete {
		measurement.PutMetric(data.NewMetric(
			"negative_midpoint_return",
			negative,
			nil,
			nil,
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
		))
	}

	envelope := types.NewEnvelope(types.EnvelopeTrade)
	envelope.TradeData.Symbol = "BTC/USD"
	envelope.PumpDump = measurement

	return envelope
}

func BenchmarkSolverStep(b *testing.B) {
	solver := NewSolver(b.Context(), midpointFeatures())
	conditionMidpointSolver(solver)
	envelope := advisorMeasurementEnvelope(2, 3, true, nil)
	clock := envelope.PumpDump.Metrics["completed_volume_bar_ordinal"]
	ordinal := uint64(3)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ordinal++
		clock.Raw = float64(ordinal)
		envelope.PumpDump.Metrics["completed_volume_bar_ordinal"] = clock

		if solver.Step(envelope) == nil {
			b.Fatal(solver.Error())
		}
	}
}
