package statistic_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestWeightedMomentsNext(t *testing.T) {
	Convey("Volume weights determine the mean and independent effective support", t, func() {
		op := statistic.NewWeightedMoments()
		out := tests.CollectSeq[[4]float64](op.Next(tests.SliceToSeq([][4]float64{
			{1, 1, 1, 0},
			{3, 9, 5, 0},
		})))
		moments := out[len(out)-1]

		So(op.Error(), ShouldBeNil)
		So(moments[2], ShouldEqual, 4)
		So(moments[3], ShouldEqual, 12)
		So(moments[0]*moments[0]/moments[1], ShouldAlmostEqual, 1.6)
	})

	Convey("Combining summaries preserves the observation calculation", t, func() {
		var leftIn, rightIn, completeIn [][4]float64

		for index := 1; index <= 20; index++ {
			value, weight := float64(index), float64(index%3+1)
			observation := [4]float64{weight, weight * weight, value, 0}
			completeIn = append(completeIn, observation)

			if index <= 10 {
				leftIn = append(leftIn, observation)
				continue
			}

			rightIn = append(rightIn, observation)
		}

		left := tests.CollectSeq[[4]float64](statistic.NewWeightedMoments().Next(tests.SliceToSeq(leftIn)))
		right := tests.CollectSeq[[4]float64](statistic.NewWeightedMoments().Next(tests.SliceToSeq(rightIn)))
		complete := tests.CollectSeq[[4]float64](statistic.NewWeightedMoments().Next(tests.SliceToSeq(completeIn)))
		merged := tests.CollectSeq[[4]float64](statistic.NewWeightedMoments().Next(tests.SliceToSeq([][4]float64{
			left[len(left)-1],
			right[len(right)-1],
		})))
		whole := complete[len(complete)-1]
		combined := merged[len(merged)-1]

		So(combined[2], ShouldAlmostEqual, whole[2])
		So(combined[3], ShouldAlmostEqual, whole[3])
		So(combined[0]*combined[0]/combined[1], ShouldAlmostEqual, whole[0]*whole[0]/whole[1])
	})
}

func BenchmarkWeightedMomentsNext(b *testing.B) {
	op := statistic.NewWeightedMoments()
	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		weight := float64(index%3 + 1)
		observation := [][4]float64{{weight, weight * weight, float64(index % 100), 0}}

		for range op.Next(tests.SliceToSeq(observation)) {
		}
	}
}
