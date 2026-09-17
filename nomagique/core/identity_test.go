package core_test

import (
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestProtoIdentity(t *testing.T) {
	Convey("ProtoIdentity attaches an address and pipes through wrapped primitives", t, func() {
		negator := &testNegator{PrimitiveError: core.NewPrimitiveError()}
		proto := core.NewProtoIdentity[string](negator)
		proto.Identify("coord-1")

		So(proto.Identity(), ShouldEqual, "coord-1")

		Convey("Next executes wrapped primitives in streaming order", func() {
			val := 5.0
			out := tests.CollectSeq[float64](proto.Next(sequence.NewOne(unsafe.Pointer(&val)).Next(nil)))
			So(out, ShouldResemble, []float64{-5.0})
		})
	})
}

type testNegator struct {
	*core.PrimitiveError
}

func (n *testNegator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			neg := -val
			if !yield(unsafe.Pointer(&neg)) {
				return
			}
		}
	}
}
