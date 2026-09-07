package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewArgmax(t *testing.T) {
	Convey("Given a reusable maximum selection", t, func() {
		node := equation.NewArgmax()
		Convey("Empty runs emit nothing and ties retain the first ordinal", func() {
			for _, sample := range []struct {
				values       []float64
				index, value float64
			}{
				{nil, 0, 0}, {[]float64{1, 9, 3}, 1, 9},
				{nil, 0, 0}, {[]float64{4, 4}, 0, 4},
				{[]float64{-3, -1, -2}, 1, -1},
			} {
				output := tests.Drain(t, node, tests.Values(sample.values...))
				So(node.Error(), ShouldBeNil)

				if len(sample.values) == 0 {
					So(output, ShouldBeEmpty)
					continue
				}

				So(output, ShouldHaveLength, 1)
				fields := tests.Fields(t, output[0])
				So(core.To[float64](fields["index"]), ShouldEqual, sample.index)
				So(core.To[float64](fields["value"]), ShouldEqual, sample.value)
			}
		})
	})
}

func BenchmarkNewArgmax(b *testing.B) {
	node := equation.NewArgmax()
	b.ReportAllocs()

	for b.Loop() {
		input := transport.NewIO(core.From(1.0), core.From(9.0), core.From(3.0))

		if node.Next(input) == nil || node.Next(input) != nil {
			b.Fatal("expected one maximum")
		}

		if err := node.Error(); err != nil {
			b.Fatal(err)
		}
	}
}
