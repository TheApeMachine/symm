package correlation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestPathNext(t *testing.T) {
	Convey("Acceptance, restatement and regression keep prior observations intact", t, func() {
		path := correlation.NewPath()
		var earliest [2][]float64

		for index, test := range []struct {
			at                 float64
			value, count       float64
			accepted, restated float64
		}{
			{10, 100, 1, 1, 0}, {12, 105, 2, 1, 0}, {12, 106, 2, 1, 1},
			{11, 999, 2, 0, 0}, {13, 107, 3, 1, 0},
		} {
			out := tests.CollectSeq[[2][]float64](path.Next(tests.SliceToSeq([][2]float64{{test.at, test.value}})))
			So(path.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(out[0][0][3], ShouldEqual, test.count)
			So(out[0][0][4], ShouldEqual, test.accepted)
			So(out[0][0][5], ShouldEqual, test.restated)
			wanted := test.value

			if test.accepted == 0 {
				wanted = 106
			}

			flat := out[0][1]
			So(flat[len(flat)-1], ShouldEqual, wanted)

			if index == 1 {
				earliest = out[0]
			}
		}

		So(earliest[1][3], ShouldEqual, 105)
	})
}
