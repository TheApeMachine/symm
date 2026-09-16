package statistic_test

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
	"testing"
)

func TestWeightedMomentsUpdate(t *testing.T) {
	Convey("Volume weights determine the mean and independent effective support", t, func() {
		var moments statistic.WeightedMoments
		moments.Update(1, 1)
		moments.Update(5, 3)
		So(moments.Mean, ShouldEqual, 4)
		So(moments.M2, ShouldEqual, 12)
		So(moments.Support(), ShouldAlmostEqual, 1.6)
	})
}

func TestWeightedMomentsMerge(t *testing.T) {
	Convey("Combining summaries preserves the observation calculation", t, func() {
		var left, right, complete statistic.WeightedMoments
		for index := 1; index <= 20; index++ {
			value, weight := float64(index), float64(index%3+1)
			complete.Update(value, weight)
			if index <= 10 {
				left.Update(value, weight)
				continue
			}
			right.Update(value, weight)
		}
		left.Merge(right)
		So(left.Mean, ShouldAlmostEqual, complete.Mean)
		So(left.M2, ShouldAlmostEqual, complete.M2)
		So(left.Support(), ShouldAlmostEqual, complete.Support())
	})
}

func BenchmarkWeightedMomentsUpdate(b *testing.B) {
	var moments statistic.WeightedMoments
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		moments.Update(float64(index%100), float64(index%3+1))
	}
}
