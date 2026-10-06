package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestLeadLagNext(t *testing.T) {
	Convey("A two-second lag is recovered with its profile intact", t, func() {
		times := make([]int64, 16)
		left, right := make([]float64, 16), make([]float64, 16)

		for index := range times {
			times[index] = int64(index) * 1e9
			left[index] = math.Exp(math.Sin(float64(index)))
		}

		for index := range times {
			right[index] = left[max(0, index-2)]
		}

		node := correlation.NewLeadLag(algo.NewHayashiYoshida())
		var original []float64
		pair := [2][][2]float64{prices(times, left), prices(times, right)}

		for range 2 {
			out := tests.CollectSeq[[2][]float64](node.Next(tests.SliceToSeq([][2][][2]float64{pair})))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			got := out[0]
			So(got[0][8], ShouldEqual, 2)
			So(got[0][14], ShouldEqual, 1e9)
			So(got[0][15], ShouldEqual, 14)
			So(got[0][7], ShouldEqual, 2)
			So(got[0][5], ShouldEqual, 1)
			So(len(got[1])/10, ShouldEqual, 29)
			idx := int(got[0][6])
			So(got[1][idx*10+2], ShouldEqual, got[0][2])
			So(got[0][2], ShouldBeGreaterThan, 0)

			if original == nil {
				original = append([]float64(nil), got[1]...)
			}
		}

		empty := tests.CollectSeq[[2][]float64](node.Next(tests.SliceToSeq([][2][][2]float64{{nil, nil}})))
		So(node.Error(), ShouldBeNil)
		So(len(empty[0][1]), ShouldEqual, 0)
		So(empty[0][0][5], ShouldEqual, 0)
		So(original[8], ShouldEqual, -14)
	})
}

func TestLagShapeUsesSelectedIndex(t *testing.T) {
	Convey("Shape uses the selected index, not a new peak search", t, func() {
		profile := make([]float64, 0, 50)

		for index, y := range []float64{.1, .2, 1, .8, .2} {
			profile = append(profile,
				y, 0, 1, 1, 1, 1,
				float64(index), 0, float64(index-2), y,
			)
		}

		input := [2][]float64{profile, {3, 2, 1e9}}
		out := tests.CollectSeq[[3]float64](correlation.NewLagShape().Next(tests.SliceToSeq([][2][]float64{input})))
		So(len(out), ShouldEqual, 1)
		So(out[0][1], ShouldAlmostEqual, .2)
		So(out[0][2], ShouldAlmostEqual, .4)
	})
}
