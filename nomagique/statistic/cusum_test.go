package statistic_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestCUSUM(t *testing.T) {
	Convey("CUSUM primitive detects directional departures beyond noise hurdles", t, func() {
		filter := statistic.NewCUSUM()

		Convey("Initial observation establishes baseline without triggering", func() {
			out := tests.CollectSeq[statistic.CUSUMReading](filter.Next(transport.NewValues(
				statistic.CUSUMObservation{Sequence: 1, Value: 100.0, Hurdle: 0.5, Threshold: 5.0},
			).Next(nil)))

			So(filter.Error(), ShouldBeNil)
			So(out[0].Signal, ShouldEqual, statistic.CUSUMNone)
			So(out[0].UpperSum, ShouldEqual, 0)
			So(out[0].LowerSum, ShouldEqual, 0)
			So(out[0].UpperStart, ShouldEqual, 1)
			So(out[0].LowerStart, ShouldEqual, 1)
		})

		Convey("Sub-hurdle micro fluctuations absorb into zero without triggering", func() {
			out := tests.CollectSeq[statistic.CUSUMReading](filter.Next(transport.NewValues(
				statistic.CUSUMObservation{Sequence: 1, Value: 100.0, Hurdle: 1.0, Threshold: 5.0},
				statistic.CUSUMObservation{Sequence: 2, Value: 100.5, Hurdle: 1.0, Threshold: 5.0},
				statistic.CUSUMObservation{Sequence: 3, Value: 100.0, Hurdle: 1.0, Threshold: 5.0},
			).Next(nil)))

			So(out[1].Signal, ShouldEqual, statistic.CUSUMNone)
			So(out[1].UpperSum, ShouldEqual, 0)
			So(out[2].Signal, ShouldEqual, statistic.CUSUMNone)
			So(out[2].LowerSum, ShouldEqual, 0)
		})

		Convey("Persistent positive moves accumulate and trigger CUSUMUpper with accurate Point A", func() {
			out := tests.CollectSeq[statistic.CUSUMReading](filter.Next(transport.NewValues(
				statistic.CUSUMObservation{Sequence: 1, Value: 100.0, Hurdle: 0.5, Threshold: 3.0},
				statistic.CUSUMObservation{Sequence: 2, Value: 102.0, Hurdle: 0.5, Threshold: 3.0},
				statistic.CUSUMObservation{Sequence: 3, Value: 104.5, Hurdle: 0.5, Threshold: 3.0},
			).Next(nil)))

			So(out[1].Signal, ShouldEqual, statistic.CUSUMNone)
			So(out[1].UpperSum, ShouldEqual, 1.5)
			So(out[1].UpperStart, ShouldEqual, 1)

			So(out[2].Signal, ShouldEqual, statistic.CUSUMUpper)
			So(out[2].UpperStart, ShouldEqual, 1)
		})

		Convey("Persistent negative moves accumulate and trigger CUSUMLower with accurate Point A", func() {
			out := tests.CollectSeq[statistic.CUSUMReading](filter.Next(transport.NewValues(
				statistic.CUSUMObservation{Sequence: 10, Value: 100.0, Hurdle: 0.5, Threshold: 3.0},
				statistic.CUSUMObservation{Sequence: 11, Value: 98.0, Hurdle: 0.5, Threshold: 3.0},
				statistic.CUSUMObservation{Sequence: 12, Value: 95.5, Hurdle: 0.5, Threshold: 3.0},
			).Next(nil)))

			So(out[1].Signal, ShouldEqual, statistic.CUSUMNone)
			So(out[1].LowerSum, ShouldEqual, -1.5)
			So(out[1].LowerStart, ShouldEqual, 10)

			So(out[2].Signal, ShouldEqual, statistic.CUSUMLower)
			So(out[2].LowerStart, ShouldEqual, 10)
		})
	})
}
