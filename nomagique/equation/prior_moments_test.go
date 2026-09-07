package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPriorMomentsObserve(t *testing.T) {
	Convey("Given weighted signed outcomes", t, func() {
		moments := PriorMoments{}
		So(moments.Observe(-1, 0.5, 0, 1), ShouldBeNil)
		So(moments.Observe(1, 0.5, 0, 2), ShouldBeNil)

		Convey("The normalized recurrence preserves mean, sample variance and support", func() {
			reading := moments.Summary(0)
			So(reading.Mean, ShouldEqual, 0)
			So(reading.Variance, ShouldEqual, 2)
			So(reading.Support, ShouldEqual, 2)
			So(reading.EvidenceAuthority, ShouldEqual, 0.5)
			So(reading.Authority, ShouldEqual, 0)
		})
		Convey("Zero-authority outcomes count but cannot advance the evidence clock", func() {
			So(moments.Observe(100, 0, 8, 100), ShouldBeNil)
			So(moments.Samples, ShouldEqual, 3)
			So(moments.LastEpoch, ShouldEqual, 2)
			So(moments.Weight, ShouldEqual, 1)
		})
		Convey("Invalid authority leaves every sufficient statistic untouched", func() {
			before := moments
			So(moments.Observe(100, -0.5, 8, 100), ShouldNotBeNil)
			So(moments, ShouldResemble, before)
			So(moments.Observe(100, 1.5, 8, 100), ShouldNotBeNil)
			So(moments, ShouldResemble, before)
		})
	})
}

func TestPriorMomentsAge(t *testing.T) {
	Convey("Given a prior with an eight-resolution retention fixture", t, func() {
		moments := PriorMoments{}
		So(moments.Observe(-2, 0.5, 8, 1), ShouldBeNil)
		So(moments.Observe(4, 0.5, 8, 2), ShouldBeNil)
		before := moments
		moments.Age(1, 8)
		So(moments, ShouldResemble, before)
		moments.Age(3, 8)
		So(moments.Weight, ShouldAlmostEqual, before.Weight*0.875)
		So(moments.Moment, ShouldEqual, before.Moment)
		So(moments.Support, ShouldEqual, before.Support)
		moments.Age(1_000_000, 8)
		So(moments.Summary(8).Defined, ShouldBeFalse)
		So(moments.Observe(-7, 0.5, 8, 1_000_001), ShouldBeNil)
		So(moments.Summary(8).Mean, ShouldEqual, -7)
		So(moments.Summary(8).Support, ShouldEqual, 1)
	})
}

func BenchmarkPriorMomentsObserve(b *testing.B) {
	moments := PriorMoments{}
	b.ReportAllocs()

	for b.Loop() {
		if err := moments.Observe(1, 0.5, 8); err != nil {
			b.Fatal(err)
		}
		moments.Summary(8)
	}
}
