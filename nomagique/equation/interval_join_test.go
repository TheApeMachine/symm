package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
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
			left := intervals(fixture.left)
			right := intervals(fixture.right)
			var expected []equation.IntervalPair

			for _, first := range fixture.left {
				for _, second := range fixture.right {
					if first[0] < second[1] && second[0] < first[1] {
						expected = append(expected, equation.IntervalPair{
							Left:  equation.Interval{From: first[0], To: first[1]},
							Right: equation.Interval{From: second[0], To: second[1]},
						})
					}
				}
			}

			output := tests.CollectSeq(join.Next(transport.Values(equation.IntervalJoinInput{Left: left, Right: right})))
			So(join.Error(), ShouldBeNil)
			So(len(output), ShouldEqual, len(expected))
			So(output, ShouldResemble, expected)
		}
	})

	Convey("Given an invalid interval path", t, func() {
		for _, spans := range [][][2]int64{{{2, 1}}, {{1, 1}}, {{0, 3}, {2, 4}}, {{4, 5}, {0, 1}}} {
			join := equation.NewIntervalJoin()
			path := intervals(spans)
			output := tests.CollectSeq(join.Next(transport.Values(equation.IntervalJoinInput{Left: path, Right: path})))
			So(output, ShouldBeEmpty)
			So(join.Error(), ShouldNotBeNil)
		}
	})
}

func intervals(spans [][2]int64) []equation.Interval {
	out := make([]equation.Interval, len(spans))

	for index, span := range spans {
		out[index] = equation.Interval{From: span[0], To: span[1]}
	}

	return out
}
