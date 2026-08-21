package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
)

func TestGreaterThan(t *testing.T) {
	Convey("Given two living operands", t, func() {
		input := comparisonInput(3, 2)
		_, output, err := GreaterThan(nomagique.Frame{}, input)

		Convey("It should emit their strict ordering", func() {
			So(err, ShouldBeNil)
			So(output.MustGet(SymbolCondition), ShouldEqual, 1.0)
		})
	})
}

func TestLessThan(t *testing.T) {
	Convey("Given two living operands", t, func() {
		input := comparisonInput(2, 3)
		_, output, err := LessThan(nomagique.Frame{}, input)

		Convey("It should emit their strict ordering", func() {
			So(err, ShouldBeNil)
			So(output.MustGet(SymbolCondition), ShouldEqual, 1.0)
		})
	})
}

func TestEqual(t *testing.T) {
	Convey("Given two identical living operands", t, func() {
		input := comparisonInput(3, 3)
		_, output, err := Equal(nomagique.Frame{}, input)

		Convey("It should emit exact equality", func() {
			So(err, ShouldBeNil)
			So(output.MustGet(SymbolCondition), ShouldEqual, 1.0)
		})
	})
}

func comparisonInput(left float64, right float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(calculus.SymbolLeft, left)
	input.Put(calculus.SymbolRight, right)

	return input
}

func BenchmarkGreaterThan(benchmark *testing.B) {
	input := comparisonInput(3, 2)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = GreaterThan(nomagique.Frame{}, input)
	}
}
