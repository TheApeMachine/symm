package grid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestEmit(t *testing.T) {
	Convey("Given an Emit primitive configured for 3 regions", t, func() {
		emitter := NewEmit(3)

		Convey("Accumulates tokens and yields when matching region count", func() {
			out := tests.CollectSeq[float64](emitter.Next(data.NewValue(101.0, 102.0, 103.0).Next(nil)))

			So(emitter.Error(), ShouldBeNil)
			So(out, ShouldResemble, []float64{101.0, 102.0, 103.0})
		})
	})
}
