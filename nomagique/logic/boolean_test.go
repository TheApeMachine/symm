package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
)

func TestAnd(t *testing.T) {
	Convey("Given two true operands", t, func() {
		_, output, err := And(nomagique.Frame{}, comparisonInput(1, 1))

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolCondition), ShouldEqual, 1.0)
	})
}

func TestOr(t *testing.T) {
	Convey("Given one true operand", t, func() {
		_, output, err := Or(nomagique.Frame{}, comparisonInput(0, 1))

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolCondition), ShouldEqual, 1.0)
	})
}

func TestXor(t *testing.T) {
	Convey("Given two true operands", t, func() {
		_, output, err := Xor(nomagique.Frame{}, comparisonInput(1, 1))

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolCondition), ShouldEqual, 0.0)
	})
}

func TestNot(t *testing.T) {
	Convey("Given a false condition", t, func() {
		input := nomagique.Frame{}
		input.Put(SymbolCondition, 0)
		_, output, err := Not(nomagique.Frame{}, input)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolCondition), ShouldEqual, 1.0)
	})
}

func BenchmarkAnd(benchmark *testing.B) {
	input := comparisonInput(1, 1)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = And(nomagique.Frame{}, input)
	}
}
