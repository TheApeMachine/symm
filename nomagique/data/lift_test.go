package data

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLift(t *testing.T) {
	Convey("Given a set of measurements", t, func() {
		currentTime := time.Now()

		measurementOne := NewMeasurement[float64]("id-1", "test", "hawkes", currentTime, currentTime)
		measurementOne.PutMetric(Metric[float64]{
			Label: "arrival_rate",
			Raw:   100.0,
		})
		measurementOne.Metadata = map[string]float64{
			MetadataSupport: 10,
		}
		measurementOne.Finalize()

		measurementTwo := NewMeasurement[float64]("id-2", "test", "depthflow", currentTime, currentTime)
		measurementTwo.PutMetric(Metric[float64]{
			Label: "imbalance",
			Raw:   50.0,
		})
		measurementTwo.Finalize()

		measurementFailing := NewMeasurement[float64]("id-3", "test", "broken", currentTime, currentTime)
		measurementFailing.Err = errors.New("sensor failure")

		measurements := []*Measurement[float64]{measurementOne, measurementTwo, measurementFailing, nil}

		Convey("Lift should resolve values through authority and ignore failed measurements", func() {
			observation, err := Lift(measurements)

			So(err, ShouldNotBeNil)
			// hawkes arrival_rate maturity is 1 - 1/10 = 0.9, estimated with undefined SNR discounts to 0.5, authority = 0.45, raw = 100 -> value = 45
			So(observation["hawkes/arrival_rate"], ShouldAlmostEqual, 45.0, 1e-6)
			// depthflow imbalance is a stateless direct observation: authority = 1.0, raw = 50 -> value = 50
			So(observation["depthflow/imbalance"], ShouldEqual, 50.0)
			_, hasBroken := observation["broken/something"]
			So(hasBroken, ShouldBeFalse)
		})

		Convey("LiftReadouts should return high-fidelity Readouts", func() {
			readouts, err := LiftReadouts(measurements)

			So(err, ShouldNotBeNil)
			So(readouts["hawkes/arrival_rate"], ShouldNotBeNil)
			So(readouts["hawkes/arrival_rate"].Authority(), ShouldAlmostEqual, 0.45, 1e-6)
			So(readouts["depthflow/imbalance"], ShouldNotBeNil)
			So(readouts["depthflow/imbalance"].Authority(), ShouldEqual, 1.0)
		})
	})
}

func BenchmarkLift(b *testing.B) {
	currentTime := time.Now()

	measurementOne := NewMeasurement[float64]("id-1", "test", "hawkes", currentTime, currentTime)
	measurementOne.PutMetric(Metric[float64]{
		Label: "arrival_rate",
		Raw:   100.0,
	})
	measurementOne.Metadata = map[string]float64{
		MetadataSupport: 10,
	}
	measurementOne.Finalize()

	measurementTwo := NewMeasurement[float64]("id-2", "test", "depthflow", currentTime, currentTime)
	measurementTwo.PutMetric(Metric[float64]{
		Label: "imbalance",
		Raw:   50.0,
	})
	measurementTwo.Finalize()

	measurements := []*Measurement[float64]{measurementOne, measurementTwo}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = Lift(measurements)
	}
}
