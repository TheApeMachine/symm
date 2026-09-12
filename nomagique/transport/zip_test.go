package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestZipNext(t *testing.T) {
	Convey("Zip pairs the inbound run with the held right run", t, func() {
		op := transport.NewZip[float64, string](
			transport.NewValues("a", "b", "c").Next(nil),
		)
		out := tests.CollectSeq[transport.Pair[float64, string]](
			op.Next(transport.NewValues(1.0, 2.0, 3.0).Next(nil)),
		)

		So(len(out), ShouldEqual, 3)
		So(out[0], ShouldResemble, transport.Pair[float64, string]{Left: 1, Right: "a"})
		So(out[2], ShouldResemble, transport.Pair[float64, string]{Left: 3, Right: "c"})
		So(op.Error(), ShouldBeNil)
	})

	Convey("Zip stops when either run ends", t, func() {
		long := transport.NewZip[float64, float64](
			transport.NewValues(1.0).Next(nil),
		)
		out := tests.CollectSeq[transport.Pair[float64, float64]](
			long.Next(transport.NewValues(1.0, 2.0, 3.0).Next(nil)),
		)
		So(len(out), ShouldEqual, 1)
	})
}

func TestZip2Next(t *testing.T) {
	Convey("Zip2 pairs same-typed runs as [2]T", t, func() {
		op := transport.NewZip2[float64](transport.NewValues(10.0, 20.0).Next(nil))
		out := tests.CollectSeq[[2]float64](
			op.Next(transport.NewValues(1.0, 2.0, 3.0).Next(nil)),
		)

		So(len(out), ShouldEqual, 2)
		So(out[0], ShouldResemble, [2]float64{1, 10})
		So(out[1], ShouldResemble, [2]float64{2, 20})
		So(op.Error(), ShouldBeNil)
	})
}
