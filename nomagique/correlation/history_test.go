package correlation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestHistoryNext(t *testing.T) {
	Convey("History tracks nearest neighbor trajectory distance and percentile", t, func() {
		node := correlation.NewHistory()

		var outInitial []float64
		for val := range node.Next(data.NewValue(0.0, 0.0).Next(nil)) {
			outInitial = append(outInitial, *(*float64)(val))
		}
		So(node.Error(), ShouldBeNil)
		So(len(outInitial), ShouldEqual, 2)
		So(outInitial[0], ShouldEqual, 0.0)
		So(outInitial[1], ShouldEqual, 0.0)

		var outNext []float64
		for val := range node.Next(data.NewValue(3.0, 4.0).Next(nil)) {
			outNext = append(outNext, *(*float64)(val))
		}
		So(node.Error(), ShouldBeNil)
		So(len(outNext), ShouldEqual, 2)
		So(outNext[0], ShouldAlmostEqual, 5.0)
	})
}
