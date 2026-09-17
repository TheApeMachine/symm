package sequence_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestZipNext(t *testing.T) {
	Convey("Zip pairs the inbound run with the held right run", t, func() {
		op := sequence.NewZip[float64, string](sequence.NewValues("a", "b", "c").Next(nil))
		out := tests.CollectSeq[sequence.Pair[float64, string]](
			op.Next(sequence.NewValues(1.0, 2.0, 3.0).Next(nil)),
		)

		So(len(out), ShouldEqual, 3)
		So(out[0], ShouldResemble, sequence.Pair[float64, string]{Left: 1, Right: "a"})
		So(out[2], ShouldResemble, sequence.Pair[float64, string]{Left: 3, Right: "c"})
		So(op.Error(), ShouldBeNil)
	})

	Convey("Zip stops when either run ends", t, func() {
		long := sequence.NewZip[float64, float64](sequence.NewValues(1.0).Next(nil))
		out := tests.CollectSeq[sequence.Pair[float64, float64]](
			long.Next(sequence.NewValues(1.0, 2.0, 3.0).Next(nil)),
		)
		So(len(out), ShouldEqual, 1)
	})
}
