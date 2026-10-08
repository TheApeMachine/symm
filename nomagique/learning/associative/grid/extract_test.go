package grid

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestExtract(t *testing.T) {
	Convey("Given an Extract primitive configured for 3 channels", t, func() {
		extractor := NewExtract(3)

		Convey("A matching input slice passes through directly", func() {
			out := tests.CollectSeq[float64](extractor.Next(data.NewValue(1.5, 2.5, -0.5).Next(nil)))
			So(extractor.Error(), ShouldBeNil)
			So(out, ShouldResemble, []float64{1.5, 2.5, -0.5})
		})

		Convey("A mismatching slice records ErrShape and yields nothing", func() {
			res := tests.CollectSeq[float64](extractor.Next(data.NewValue(1.5, 2.5).Next(nil)))
			So(len(res), ShouldEqual, 0)
			So(errors.Is(extractor.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
