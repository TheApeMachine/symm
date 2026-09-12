package adaptive_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestWindow(t *testing.T) {
	Convey("A stationary window expands without allocating observation records", t, func() {
		window := adaptive.NewWindow()
		values := make([]float64, 100)
		for i := range values {
			values[i] = 7
		}

		output := tests.CollectSeq[adaptive.WindowReading](window.Next(transport.NewValues(values...).Next(nil)))
		So(window.Error(), ShouldBeNil)
		So(len(output), ShouldEqual, 100)

		for index, reading := range output {
			So(reading.Capacity, ShouldEqual, index+1)
			So(reading.ShedRatio, ShouldEqual, 1)
		}
	})

	Convey("Window sheds on mean shift", t, func() {
		window := adaptive.NewWindow()
		values := []float64{1, 3, 5, 7, 9, 11, 13, 15, 17, 19}
		output := tests.CollectSeq[adaptive.WindowReading](window.Next(transport.NewValues(values...).Next(nil)))

		So(window.Error(), ShouldBeNil)
		So(len(output), ShouldEqual, len(values))
		So(output[0].Capacity, ShouldEqual, 1)
		So(output[len(output)-1].Capacity, ShouldBeGreaterThan, 0)
	})
}
