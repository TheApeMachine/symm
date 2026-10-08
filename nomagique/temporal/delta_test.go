package temporal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestDelta(t *testing.T) {
	Convey("Delta computes sequential difference from previous value", t, func() {
		delta := temporal.NewDelta()

		v1, v2, v3 := 10.0, 15.0, 12.0

		var results []float64
		for ptr := range delta.Next(data.NewValue(v1, v2, v3).Next(nil)) {
			results = append(results, *(*float64)(ptr))
		}

		So(len(results), ShouldEqual, 3)
		So(results[0], ShouldEqual, 0.0)
		So(results[1], ShouldEqual, 5.0)
		So(results[2], ShouldEqual, -3.0)
	})
}
