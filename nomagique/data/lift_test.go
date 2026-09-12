package data

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
)

func liftedMeasurements() []*Measurement[float64] {
	currentTime := time.Now()

	measurementOne := NewMeasurement[float64]("hawkes", map[string]Metric[float64]{})
	measurementOne.Label, measurementOne.At, measurementOne.From = "test", currentTime, currentTime
	measurementOne.Metrics["arrival_rate"] = Metric[float64]{
		Label: "arrival_rate", Raw: 100.0,
	}
	measurementOne.Metadata = map[string]float64{
		MetadataSupport: 10,
	}

	finalizer := NewFinalizer[float64]()

	for range finalizer.Next(transport.NewValues(measurementOne).Next(nil)) {
	}

	measurementTwo := NewMeasurement[float64]("depthflow", map[string]Metric[float64]{})
	measurementTwo.Label, measurementTwo.At, measurementTwo.From = "test", currentTime, currentTime
	measurementTwo.Metrics["imbalance"] = Metric[float64]{
		Label: "imbalance", Raw: 50.0,
	}

	for range finalizer.Next(transport.NewValues(measurementTwo).Next(nil)) {
	}

	measurementFailing := NewMeasurement[float64]("broken", map[string]Metric[float64]{})
	measurementFailing.Label, measurementFailing.At, measurementFailing.From = "test", currentTime, currentTime
	measurementFailing.Err = errors.New("sensor failure")

	return []*Measurement[float64]{measurementOne, measurementTwo, measurementFailing, nil}
}

func TestLiftNext(t *testing.T) {
	Convey("Lift resolves values through authority and ignores failed measurements", t, func() {
		node := NewLift()
		var reading *LiftReading

		for out := range node.Next(transport.NewValues(liftedMeasurements()...).Next(nil)) {
			reading = (*LiftReading)(out)
		}

		So(node.Error(), ShouldNotBeNil)
		So(reading, ShouldNotBeNil)
		// hawkes arrival_rate maturity is 1 - 1/10 = 0.9, estimated with undefined SNR discounts to 0.5, authority = 0.45, raw = 100 -> value = 45
		So(reading.Values["hawkes/arrival_rate"], ShouldAlmostEqual, 45.0, 1e-6)
		// depthflow imbalance is a stateless direct observation: authority = 1.0, raw = 50 -> value = 50
		So(reading.Values["depthflow/imbalance"], ShouldEqual, 50.0)
		_, hasBroken := reading.Values["broken/something"]
		So(hasBroken, ShouldBeFalse)
	})
}

func TestLiftReadoutsNext(t *testing.T) {
	Convey("LiftReadouts returns high-fidelity Readouts", t, func() {
		node := NewLiftReadouts()
		var reading *ReadoutLift

		for out := range node.Next(transport.NewValues(liftedMeasurements()...).Next(nil)) {
			reading = (*ReadoutLift)(out)
		}

		So(node.Error(), ShouldNotBeNil)
		So(reading, ShouldNotBeNil)

		hawkes, ok := reading.Readouts["hawkes/arrival_rate"]
		So(ok, ShouldBeTrue)
		So(hawkes.Authority, ShouldAlmostEqual, 0.45, 1e-6)

		depth, ok := reading.Readouts["depthflow/imbalance"]
		So(ok, ShouldBeTrue)
		So(depth.Authority, ShouldEqual, 1.0)
	})
}

func BenchmarkLiftNext(b *testing.B) {
	measurements := liftedMeasurements()[:2]
	node := NewLift()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for range node.Next(transport.NewValues(measurements...).Next(nil)) {
		}
	}
}
