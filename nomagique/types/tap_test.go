package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTap(t *testing.T) {
	Convey("Given a reading exposed by an upstream node", t, func() {
		reading := Scalar(7)

		Convey("a Tap with no Into emits that reading as a slot", func() {
			tap := &Tap{Read: func() Scalar { return reading }}

			So(tap.Step(0), ShouldEqual, Scalar(7))
			So(tap.Step(999), ShouldEqual, Scalar(7))
		})

		Convey("a Tap with an Into feeds it and returns the additive identity", func() {
			downstream := &Probe{}
			tap := &Tap{Read: func() Scalar { return reading }, Into: downstream}

			So(tap.Step(0), ShouldEqual, Scalar(0))
			So(downstream.Value, ShouldEqual, Scalar(7))
		})

		Convey("an injecting Tap leaves a parallel sum uncorrupted", func() {
			// Law of Sinks: the recording branch contributes 0, so the Split
			// returns only what the computing branch produced.
			split := &Split{
				A: Identity{},
				B: &Tap{Read: func() Scalar { return reading }, Into: &Probe{}},
			}

			So(split.Step(42), ShouldEqual, Scalar(42))
		})

		Convey("an omitted Read has nothing to select and yields zero", func() {
			So((&Tap{}).Step(5), ShouldEqual, Scalar(0))
		})
	})
}

func TestProbe(t *testing.T) {
	Convey("Given a Probe placed inline in a Chain", t, func() {
		probe := &Probe{}

		chain := &Chain{
			A: Identity{},
			B: probe,
			C: Identity{},
		}

		Convey("it captures the carrier without changing what the Chain computes", func() {
			So(chain.Step(3.5), ShouldEqual, Scalar(3.5))
			So(probe.Value, ShouldEqual, Scalar(3.5))
			So(probe.Seen, ShouldBeTrue)
		})

		Convey("it retains only the most recent value", func() {
			chain.Step(1)
			chain.Step(2)

			So(probe.Value, ShouldEqual, Scalar(2))
		})

		Convey("an unstepped Probe reports that it has seen nothing", func() {
			So((&Probe{}).Seen, ShouldBeFalse)
		})
	})
}
