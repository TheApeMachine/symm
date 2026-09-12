package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestFanNext(t *testing.T) {
	Convey("Fan presents a replayable run to every branch", t, func() {
		op := transport.NewFan(transport.NewPass(), transport.NewPass())

		for _, values := range [][]float64{{1, 2, 3}, {}, {4}, {5, 6}} {
			out := tests.CollectSeq[float64](op.Next(transport.NewValues(values...).Next(nil)))

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
		op := transport.NewFan()
		So(tests.CollectSeq[float64](op.Next(transport.NewValues(1.0, 2.0).Next(nil))), ShouldBeEmpty)
		So(op.Error(), ShouldBeNil)
	})
}
