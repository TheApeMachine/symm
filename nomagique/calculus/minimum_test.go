package calculus

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestMinimumNext(t *testing.T) {
	tests.Check(t, tests.Case{Name: "minimum", Seed: 100, Operation: NewMinimum(transport.NewIO(core.From(float64(100))))})
}

func TestMinimumRead(t *testing.T) {
	Convey("Given a configured minimum whose latest output is separately retained", t, func() {
		operation := NewMinimum(transport.NewIO(core.From(100.0)))
		So(operation.Read(), ShouldBeNil)

		for _, value := range []float64{-4, 25, 120} {
			result := operation.Next(transport.NewIO(core.From(value)))
			So(core.To[float64](result), ShouldEqual, min(100, value))
			So(operation.Next(nil), ShouldBeNil)
			So(operation.Read(), ShouldEqual, min(100, value))
			So(operation.Error(), ShouldBeNil)
		}
	})
}

func BenchmarkMinimumNext(b *testing.B) {
	operation := NewMinimum(transport.NewIO(core.From(100.0)))
	input := transport.NewIO(core.From(12.0), core.From(-4.0), core.From(8.0))
	b.ReportAllocs()

	for b.Loop() {
		result := operation.Next(input)

		if result == nil || core.To[float64](result) != -4 || operation.Next(input) != nil {
			b.Fatal("minimum did not preserve its configured source and finite delivery")
		}
	}
}
