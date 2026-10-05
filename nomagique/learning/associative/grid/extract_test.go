package grid

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestExtract(t *testing.T) {
	Convey("Given an Extract primitive configured for 3 channels", t, func() {
		extractor := NewExtract(3)

		Convey("A matching input slice passes through directly", func() {
			input := []float64{1.5, 2.5, -0.5}
			vec := data.Read[[]float64](extractor.Next(data.NewValue(input)))
			So(vec, ShouldResemble, input)
			So(extractor.Error(), ShouldBeNil)
		})

		Convey("A mismatching slice records ErrShape and yields nothing", func() {
			input := []float64{1.5, 2.5}
			vec := data.Read[[]float64](extractor.Next(data.NewValue(input)))
			So(len(vec), ShouldEqual, 0)
			So(errors.Is(extractor.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
