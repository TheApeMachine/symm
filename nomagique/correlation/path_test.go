package correlation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPathNext(t *testing.T) {
	Convey("Acceptance, restatement and regression keep prior observations intact", t, func() {
		path := correlation.NewPath()
		var earliest correlation.PathReading

		for index, test := range []struct {
			at                 int64
			value, count       float64
			accepted, restated bool
		}{
			{10, 100, 1, true, false}, {12, 105, 2, true, false}, {12, 106, 2, true, true},
			{11, 999, 2, false, false}, {13, 107, 3, true, false},
		} {
			out := tests.CollectSeq[correlation.PathReading](path.Next(transport.NewValues(temporal.Price{At: test.at, Value: test.value}).Next(nil)))
			So(path.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0].Count, ShouldEqual, test.count)
			So(out[0].Accepted, ShouldEqual, test.accepted)
			So(out[0].Restated, ShouldEqual, test.restated)
			wanted := test.value

			if !test.accepted {
				wanted = 106
			}

			So(out[0].Observations[len(out[0].Observations)-1].Value, ShouldEqual, wanted)

			if index == 1 {
				earliest = out[0]
			}
		}

		So(earliest.Observations[1].Value, ShouldEqual, 105)
	})
}
