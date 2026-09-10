package correlation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPathUpdate(t *testing.T) {
	Convey("Given an append-only timestamped path with previously emitted slices", t, func() {
		path := &correlation.Path{}
		retained := make([][]equation.Price, 0, 32)

		for at := int64(1); at <= 32; at++ {
			fields, err := path.Update(equation.Price{At: at, Value: float64(at)})
			So(err, ShouldBeNil)
			So(cap(fields.Observations), ShouldEqual, len(fields.Observations))
			retained = append(retained, fields.Observations)
		}

		Convey("Restating and extending the path preserve every earlier observation", func() {
			for _, at := range []int64{32, 33, 34} {
				_, err := path.Update(equation.Price{At: at, Value: -float64(at)})
				So(err, ShouldBeNil)
			}

			for index, observations := range retained {
				So(len(observations), ShouldEqual, index+1)

				for offset, observation := range observations {
					So(observation.Value, ShouldEqual, offset+1)
				}
			}
		})
	})
}

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
			out := tests.CollectSeq(path.Next(transport.Values(equation.Price{At: test.at, Value: test.value})))
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

func TestPathRetentionIsConfiguration(t *testing.T) {
	Convey("Configured tail retention owns the visible span", t, func() {
		path := correlation.NewPath(collection.NewTail[equation.Price](2))
		out := tests.CollectSeq(path.Next(transport.Values(
			equation.Price{At: 1, Value: 1},
			equation.Price{At: 2, Value: 2},
			equation.Price{At: 3, Value: 3},
		)))
		So(path.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 3)
		So(out[2].Count, ShouldEqual, 2)
		So(out[2].From, ShouldEqual, 2)
		So(out[2].To, ShouldEqual, 3)
	})
}
