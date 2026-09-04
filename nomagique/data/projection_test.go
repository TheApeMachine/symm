package data

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

/* fixed emits a declared value regardless of the carrier. */
type fixed struct{ value types.Scalar }

func (node fixed) Step(types.Scalar) types.Scalar { return node.value }

func TestProjection(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()

	Convey("Given a Projection declaring two readings", t, func() {
		projection := &Projection{
			Source: "test",
			Identity: func() (string, string, time.Time, time.Time) {
				return "BTC/USD:test:1", "BTC/USD", at, at
			},
			Readings: []Reading{
				{Label: "alpha", Read: fixed{2.5}, Unit: UnitRate},
				{Label: "beta", Read: fixed{-1}, Unit: UnitDimensionless},
			},
		}

		Convey("it publishes each reading under its label", func() {
			So(projection.Step(0), ShouldEqual, types.Scalar(0))

			measurement := projection.Measurement()
			So(measurement, ShouldNotBeNil)
			So(measurement.ID, ShouldEqual, "BTC/USD:test:1")
			So(measurement.Metrics["alpha"].Raw, ShouldEqual, 2.5)
			So(measurement.Metrics["beta"].Raw, ShouldEqual, -1.0)
			So(measurement.Metrics["alpha"].Unit, ShouldEqual, UnitRate)
		})

		Convey("it returns zero so it composes as a sink", func() {
			So(projection.Step(types.Scalar(99)), ShouldEqual, types.Scalar(0))
		})
	})

	Convey("Given a reading gated closed", t, func() {
		projection := &Projection{
			Readings: []Reading{
				{Label: "undefined", Read: fixed{5}, When: fixed{0}},
				{Label: "defined", Read: fixed{5}, When: fixed{1}},
			},
		}

		Convey("an undefined quantity is absent, not published as zero", func() {
			projection.Step(0)

			measurement := projection.Measurement()
			_, present := measurement.Metrics["undefined"]
			So(present, ShouldBeFalse)
			So(measurement.Metrics["defined"].Raw, ShouldEqual, 5.0)
		})
	})

	Convey("Given a Support slot", t, func() {
		projection := &Projection{Support: fixed{4}}

		Convey("Finalize derives maturity from the evidence count", func() {
			projection.Step(0)

			measurement := projection.Measurement()
			So(measurement.Metadata[MetadataSupport], ShouldEqual, 4.0)
			So(measurement.Maturity, ShouldEqual, 0.75)
		})
	})

	Convey("Given no Support slot", t, func() {
		projection := &Projection{
			Readings: []Reading{{Label: "alpha", Read: fixed{1}}},
		}

		Convey("the measurement is whole, as a stateless reading is", func() {
			projection.Step(0)
			So(projection.Measurement().Maturity, ShouldEqual, 1.0)
		})
	})

	Convey("Given a Projection with no readings", t, func() {
		projection := &Projection{}

		Convey("it emits an empty Measurement rather than failing", func() {
			So(projection.Step(0), ShouldEqual, types.Scalar(0))
			So(projection.Measurement(), ShouldNotBeNil)
			So(projection.Measurement().Metrics, ShouldBeEmpty)
		})
	})
}
