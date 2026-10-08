package data_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestBatch(t *testing.T) {
	Convey("Given a Batch primitive with groups (3, 2)", t, func() {
		batch := data.NewBatch(3, 2)
		input := data.NewValue(10.0, 20.0, 30.0, 4.0, 2.0)

		Convey("It yields groups formatted as Value primitives", func() {
			var groups []core.Primitive

			for ptr := range batch.Next(input.Next(nil)) {
				group := *(*core.Primitive)(ptr)
				groups = append(groups, group)
			}

			So(len(groups), ShouldEqual, 2)

			var firstGroup []float64

			for ptr := range groups[0].Next(nil) {
				firstGroup = append(firstGroup, *(*float64)(ptr))
			}

			So(firstGroup, ShouldResemble, []float64{10.0, 20.0, 30.0})

			var secondGroup []float64

			for ptr := range groups[1].Next(nil) {
				secondGroup = append(secondGroup, *(*float64)(ptr))
			}

			So(secondGroup, ShouldResemble, []float64{4.0, 2.0})
		})

		Convey("It feeds directly into Parallel branches", func() {
			batchPair := data.NewBatch(2, 2)
			inputPair := data.NewValue(10.0, 20.0, 4.0, 2.0)

			parallel := transport.NewParallel(
				arithmetic.NewAdd(),    // consumes 2 arrivals: 10 + 20 = 30
				arithmetic.NewDivide(), // consumes 2 arrivals: 4 / 2 = 2
			)

			var results []float64

			for ptr := range parallel.Next(batchPair.Next(inputPair.Next(nil))) {
				results = append(results, *(*float64)(ptr))
			}

			So(len(results), ShouldEqual, 2)
			So(results[0], ShouldEqual, 30.0)
			So(results[1], ShouldEqual, 2.0)
		})
	})
}
