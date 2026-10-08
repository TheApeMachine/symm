package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestParallel(t *testing.T) {
	Convey("Given 3 branches in Parallel", t, func() {
		parallel := transport.NewParallel(
			arithmetic.NewDivide(),
			arithmetic.NewDivide(),
			arithmetic.NewDivide(),
		)

		input := data.NewValue[core.Primitive](
			data.NewValue(10.0, 2.0),
			data.NewValue(20.0, 4.0),
			data.NewValue(30.0, 5.0),
		)

		var results []float64
		for ptr := range parallel.Next(input.Next(nil)) {
			results = append(results, *(*float64)(ptr))
		}

		So(len(results), ShouldEqual, 3)
		So(results[0], ShouldEqual, 5.0)
		So(results[1], ShouldEqual, 5.0)
		So(results[2], ShouldEqual, 6.0)
	})
}
