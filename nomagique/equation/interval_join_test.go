package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestIntervalJoinNext(t *testing.T) {
	Convey("Given ordered disjoint intervals on each path", t, func() {
		join := equation.NewIntervalJoin()
		for _, fixture := range []struct{ left, right [][2]int64 }{
			{[][2]int64{{0, 4}, {4, 8}}, [][2]int64{{0, 1}, {1, 3}, {3, 7}, {7, 8}}},
			{nil, nil},
			{[][2]int64{{0, 1}}, [][2]int64{{1, 2}}},
			{[][2]int64{{2, 5}, {8, 9}}, [][2]int64{{0, 4}, {4, 7}, {9, 10}}},
		} {
			paths := [2][]core.Primitive{}
			for side, intervals := range [2][][2]int64{fixture.left, fixture.right} {
				for _, interval := range intervals {
					paths[side] = append(paths[side], core.Record(map[string]any{"from": interval[0], "to": interval[1]}))
				}
			}
			expected := [][2]core.Primitive{}
			for left, first := range fixture.left {
				for right, second := range fixture.right {
					if first[0] < second[1] && second[0] < first[1] {
						expected = append(expected, [2]core.Primitive{paths[0][left], paths[1][right]})
					}
				}
			}
			output := tests.Drain(t, join, transport.NewIO(core.Record(map[string]any{"left": paths[0], "right": paths[1]})))
			So(join.Error(), ShouldBeNil)
			So(len(output), ShouldEqual, len(expected))
			for index, result := range output {
				pair := result.([]core.Primitive)
				So(pair[0], ShouldEqual, expected[index][0])
				So(pair[1], ShouldEqual, expected[index][1])
			}
		}
	})
	Convey("Given an invalid interval path", t, func() {
		for _, intervals := range [][][2]int64{{{2, 1}}, {{1, 1}}, {{0, 3}, {2, 4}}, {{4, 5}, {0, 1}}} {
			join := equation.NewIntervalJoin()
			path := []core.Primitive{}
			for _, interval := range intervals {
				path = append(path, core.Record(map[string]any{"from": interval[0], "to": interval[1]}))
			}
			output := tests.Drain(t, join, transport.NewIO(core.Record(map[string]any{"left": path, "right": path})))
			So(output, ShouldBeEmpty)
			So(join.Error(), ShouldNotBeNil)
		}
	})
}

func BenchmarkIntervalJoinNext(b *testing.B) {
	// 128 asynchronous intervals exercise a retained path without a production cutoff.
	paths := [2][]core.Primitive{}
	for side := range paths {
		for index := range 128 {
			paths[side] = append(paths[side], core.Record(map[string]any{
				"from": int64(index*2 + side), "to": int64((index+1)*2 + side),
			}))
		}
	}
	input := transport.NewIO(core.Record(map[string]any{"left": paths[0], "right": paths[1]}))
	for _, method := range []string{"cartesian", "ordered"} {
		b.Run(method, func(b *testing.B) {
			var graph core.Primitive = equation.NewIntervalJoin()
			if method == "cartesian" {
				graph = transport.NewPipe(
					transport.NewCross(transport.NewPipe(store.NewGet("left"), transport.NewSpread[core.Primitive]()),
						transport.NewPipe(store.NewGet("right"), transport.NewSpread[core.Primitive]())),
					transport.NewMap(logic.NewGate(equation.NewIntervalOverlap(), transport.NewPipe(), transport.NewDiscard())),
				)
			}
			b.ReportAllocs()
			for b.Loop() {
				count := 0
				for output := graph.Next(input); output != nil; output = graph.Next(input) {
					count++
				}
				if count != 255 || graph.Error() != nil {
					b.Fatal("overlap count changed", count, graph.Error())
				}
			}
		})
	}
}
