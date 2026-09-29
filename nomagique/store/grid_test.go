package store_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestGrid(t *testing.T) {
	Convey("Given a Grid primitive", t, func() {
		grid := store.NewGrid()
		So(grid, ShouldNotBeNil)
		So(grid.Settled, ShouldBeFalse)

		Convey("When measurements are streamed into Next", func() {
			meas := data.NewMeasurement[float64]("test", nil)
			meas.Metrics = map[string]data.Metric[float64]{
				"alpha": {Label: "alpha", Raw: 1.0},
				"beta":  {Label: "beta", Raw: 1.5},
			}

			in := transport.NewOne(unsafe.Pointer(meas)).Next(nil)

			for out := range grid.Next(in) {
				m := (*data.Measurement[float64])(out)
				So(m, ShouldNotBeNil)
				So(m.Metrics["alpha"].Region, ShouldBeGreaterThan, 0)
				So(m.Metrics["beta"].Region, ShouldBeGreaterThan, 0)
			}

			Convey("When sufficient settled ticks are processed", func() {
				for i := 0; i < 15; i++ {
					repeated := data.NewMeasurement[float64]("test", nil)
					repeated.Metrics = map[string]data.Metric[float64]{
						"alpha": {Label: "alpha", Raw: 1.0},
						"beta":  {Label: "beta", Raw: 1.5},
					}

					for range grid.Next(transport.NewOne(unsafe.Pointer(repeated)).Next(nil)) {
					}
				}

				So(grid.Settled, ShouldBeTrue)
			})
		})
	})
}
