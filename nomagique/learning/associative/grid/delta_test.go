package grid

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDelta(t *testing.T) {
	Convey("Given a Delta primitive configured for 2 regions", t, func() {
		delta := NewDelta(2)

		Convey("Sequential steps compute levels and changes accurately", func() {
			step1 := []float64{10.0, 20.0}
			results1 := make([][3]float64, 0, 2)
			for tuple := range data.ReadSeq[[3]float64](delta.Next(data.NewValue(step1))) {
				results1 = append(results1, tuple)
			}
			So(len(results1), ShouldEqual, 2)
			// Region 1: ID=1, level=10, change=10-0=10
			So(results1[0], ShouldResemble, [3]float64{1.0, 10.0, 10.0})
			// Region 2: ID=2, level=20, change=20-0=20
			So(results1[1], ShouldResemble, [3]float64{2.0, 20.0, 20.0})

			step2 := []float64{12.0, 15.0}
			results2 := make([][3]float64, 0, 2)
			for tuple := range data.ReadSeq[[3]float64](delta.Next(data.NewValue(step2))) {
				results2 = append(results2, tuple)
			}
			So(len(results2), ShouldEqual, 2)
			// Region 1: ID=1, level=12, change=12-10=2
			So(results2[0], ShouldResemble, [3]float64{1.0, 12.0, 2.0})
			// Region 2: ID=2, level=15, change=15-20=-5
			So(results2[1], ShouldResemble, [3]float64{2.0, 15.0, -5.0})
		})

		Convey("Shape mismatch records ErrShape", func() {
			invalid := []float64{1.0}
			data.Read[[3]float64](delta.Next(data.NewValue(invalid)))
			So(errors.Is(delta.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}

