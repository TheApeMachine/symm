package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestFanNext(t *testing.T) {
	Convey("Fan presents a replayable run to every branch", t, func() {
		op := NewFan(NewPass[float64](), NewPass[float64]())

		for _, values := range [][]float64{{1, 2, 3}, {}, {4}, {5, 6}} {
			out := tests.CollectSeq(op.Next(Values(values...)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2*len(values))

			if len(values) == 0 {
				continue
			}

			for index, value := range out {
				So(value, ShouldEqual, values[index%len(values)])
			}
		}
	})

	Convey("Fan with no branches hands nothing over", t, func() {
		op := NewFan[float64]()
		So(tests.CollectSeq(op.Next(Values(1.0, 2.0))), ShouldBeEmpty)
		So(op.Error(), ShouldBeNil)
	})
}
