package core_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestCarrierNextIsIdentity(t *testing.T) {
	Convey("A Carrier hands the incoming run through unchanged", t, func() {
		carrier := &core.Carrier[float64]{}
		in := transport.Values(7.0, 8.0)
		out := tests.CollectSeq(carrier.Next(in))

		So(out, ShouldResemble, []float64{7, 8})
	})
}

func TestAddPersistsAcrossRuns(t *testing.T) {
	Convey("Add folds across independent delivery runs from its configured start", t, func() {
		add := arithmetic.NewAdd[float64, float64](0.0)

		first := tests.CollectSeq(add.Next(transport.Values(2.0)))
		So(first[0], ShouldEqual, 2)

		second := tests.CollectSeq(add.Next(transport.Values(2.0)))
		So(second[0], ShouldEqual, 4)
		So(add.Read(), ShouldEqual, 4)
	})
}
