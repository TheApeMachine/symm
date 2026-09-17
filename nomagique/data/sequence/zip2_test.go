package sequence_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestZip2Next(t *testing.T) {
	Convey("Zip2 pairs same-typed runs as [2]T", t, func() {
		op := sequence.NewZip2[float64](sequence.NewValues(10.0, 20.0).Next(nil))
		out := tests.CollectSeq[[2]float64](
			op.Next(sequence.NewValues(1.0, 2.0, 3.0).Next(nil)),
		)

		So(len(out), ShouldEqual, 2)
		So(out[0], ShouldResemble, [2]float64{1, 10})
		So(out[1], ShouldResemble, [2]float64{2, 20})
		So(op.Error(), ShouldBeNil)
	})
}
