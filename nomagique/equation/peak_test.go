package equation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPeakNext(t *testing.T) {
	Convey("Given signed peaks and equal absolute magnitudes", t, func() {
		node := equation.NewPeak()
		points := []core.Primitive{
			tests.Record(map[string]any{"y": .1, "tag": "first"}),
			tests.Record(map[string]any{"y": -.9, "tag": "winner"}),
			tests.Record(map[string]any{"y": .9, "tag": "tie"}),
		}
		first := tests.Drain(t, node, transport.NewIO(points...))
		So(node.Error(), ShouldBeNil)
		So(len(first), ShouldEqual, 1)
		selected := tests.Fields(t, first[0])
		So(tests.Number(t, selected, "index"), ShouldEqual, 1)
		tag, err := core.Field[string](selected, "point", "tag")
		So(err, ShouldBeNil)
		So(tag, ShouldEqual, "winner")

		Convey("An empty run cannot repeat the preceding winner", func() {
			So(tests.Drain(t, node, tests.Values[core.Primitive]()), ShouldBeEmpty)
			So(node.Error(), ShouldBeNil)
		})

		Convey("An undefined ordinate remains selected through subsequent finite points", func() {
			points[1] = tests.Record(map[string]any{"y": math.NaN()})
			next := tests.Drain(t, node, transport.NewIO(points...))
			So(tests.Number(t, tests.Fields(t, next[0]), "index"), ShouldEqual, 1)
		})
	})
}

func BenchmarkPeakNext(b *testing.B) {
	points := make([]core.Primitive, 128)
	for index := range points {
		points[index] = tests.Record(map[string]any{"x": float64(index), "y": math.Sin(float64(index))})
	}
	node := equation.NewPeak()
	input := transport.NewIO(points...)
	b.ReportAllocs()
	for b.Loop() {
		if node.Next(input) == nil || node.Next(input) != nil {
			b.Fatal("expected one peak")
		}
		if err := node.Error(); err != nil {
			b.Fatal(err)
		}
	}
}
