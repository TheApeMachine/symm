package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
)

var routeOutput = nomagique.MustIntern("test/route/output")

func TestMux(t *testing.T) {
	Convey("Given a true condition and two living operands", t, func() {
		input := comparisonInput(3, 2)
		input.Put(SymbolCondition, 1)
		_, output, err := Mux(nomagique.Frame{}, input)

		So(err, ShouldBeNil)
		So(output.MustGet(SymbolResult), ShouldEqual, 3.0)
	})
}

func TestIf(t *testing.T) {
	Convey("Given a relational predicate and two branches", t, func() {
		branch := If(GreaterThan, routeValue(1), routeValue(-1))
		_, output, err := branch(nomagique.Frame{}, comparisonInput(3, 2))

		Convey("It should execute only the matching branch", func() {
			So(err, ShouldBeNil)
			So(output.MustGet(routeOutput), ShouldEqual, 1.0)
		})
	})
}

func TestCircuit(t *testing.T) {
	Convey("Given ordered relational rules", t, func() {
		circuit := Circuit([]Rule{
			{When: GreaterThan, Then: routeValue(1)},
		}, routeValue(-1))
		_, output, err := circuit(nomagique.Frame{}, comparisonInput(2, 3))

		Convey("It should execute the fallback when no rule matches", func() {
			So(err, ShouldBeNil)
			So(output.MustGet(routeOutput), ShouldEqual, -1.0)
		})
	})
}

func routeValue(value float64) nomagique.Primitive {
	return func(
		state nomagique.Frame,
		input nomagique.Frame,
	) (nomagique.Frame, nomagique.Frame, error) {
		output := input
		output.Put(routeOutput, value)

		return state, output, nil
	}
}

func BenchmarkIf(benchmark *testing.B) {
	branch := If(GreaterThan, routeValue(1), routeValue(-1))
	input := comparisonInput(3, 2)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = branch(nomagique.Frame{}, input)
	}
}

func BenchmarkMux(benchmark *testing.B) {
	input := comparisonInput(3, 2)
	input.Put(SymbolCondition, 1)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = Mux(nomagique.Frame{}, input)
	}
}
