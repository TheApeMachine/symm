package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestObserve(t *testing.T) {
	Convey("Given out-of-order observations inside a bounded series", t, func() {
		series := MustNewSeries[[2]float64](3)
		base := 1_700_008_000.0

		So(series.Observe("one", base+2, 0, [2]float64{102, 103}), ShouldBeTrue)
		So(series.Observe("one", base, 0, [2]float64{100, 101}), ShouldBeTrue)
		So(series.Observe("one", base+1, 0, [2]float64{101, 102}), ShouldBeTrue)

		Convey("It should retain every observation", func() {
			value, found := series.AsOf("one", base+2, 0)

			So(found, ShouldBeTrue)
			So(value, ShouldResemble, [2]float64{102, 103})
		})

		Convey("It should replace a re-observed event time in place", func() {
			So(series.Observe("one", base+1, 0, [2]float64{101.5, 102.5}), ShouldBeTrue)

			value, found := series.AsOf("one", base+1, 0)

			So(found, ShouldBeTrue)
			So(value, ShouldResemble, [2]float64{101.5, 102.5})
		})
	})

	Convey("Given observations on unnormalized clock coordinates", t, func() {
		series := MustNewSeries[[2]float64](3)
		base := 1_700_008_000.0

		Convey("It should refuse them without failing", func() {
			So(series.Observe("one", base, 1e9, [2]float64{100, 101}), ShouldBeFalse)
			So(series.Observe("one", base, -1, [2]float64{100, 101}), ShouldBeFalse)
			So(series.Observe("", base, 0, [2]float64{100, 101}), ShouldBeFalse)

			_, found := series.AsOf("one", base, 0)
			So(found, ShouldBeFalse)
		})

		Convey("It should accept any normalized clock, including epoch-relative times", func() {
			So(series.Observe("one", 0, 5e8, [2]float64{100, 101}), ShouldBeTrue)

			value, found := series.AsOf("one", 0, 6e8)

			So(found, ShouldBeTrue)
			So(value, ShouldResemble, [2]float64{100, 101})
		})
	})

	Convey("Given a non-positive series capacity", t, func() {
		_, err := NewSeries[[2]float64](0)
		So(err, ShouldNotBeNil)
	})
}

func TestAsOf(t *testing.T) {
	Convey("Given out-of-order observations inside a bounded series", t, func() {
		series := MustNewSeries[[2]float64](3)
		base := 1_700_008_000.0

		So(series.Observe("one", base+2, 0, [2]float64{102, 103}), ShouldBeTrue)
		So(series.Observe("one", base, 0, [2]float64{100, 101}), ShouldBeTrue)
		So(series.Observe("one", base+1, 0, [2]float64{101, 102}), ShouldBeTrue)

		Convey("It should select the newest value no later than the event", func() {
			value, found := series.AsOf("one", base+1, 5e8)

			So(found, ShouldBeTrue)
			So(value, ShouldResemble, [2]float64{101, 102})
		})

		Convey("It should never explain an older event with a later value", func() {
			_, found := series.AsOf("one", base-1, 0)
			So(found, ShouldBeFalse)
		})

		Convey("It should never answer for a key it has not observed", func() {
			_, found := series.AsOf("two", base, 0)
			So(found, ShouldBeFalse)
		})
	})

	Convey("Given a ring that overflowed its capacity", t, func() {
		series := MustNewSeries[[2]float64](3)
		base := 1_700_008_000.0

		for offset := range 4 {
			So(series.Observe(
				"one", base+float64(offset), 0,
				[2]float64{100 + float64(offset), 101 + float64(offset)},
			), ShouldBeTrue)
		}

		Convey("It should answer from the retained window only", func() {
			_, found := series.AsOf("one", base, 0)
			So(found, ShouldBeFalse)

			value, found := series.AsOf("one", base+1, 0)

			So(found, ShouldBeTrue)
			So(value, ShouldResemble, [2]float64{101, 102})
		})
	})
}

func BenchmarkObserve(b *testing.B) {
	series := MustNewSeries[[2]float64](128)
	base := 1_700_008_000.0
	b.ReportAllocs()

	for b.Loop() {
		series.Observe("one", base, 0, [2]float64{100, 101})
		_, _ = series.AsOf("one", base, 0)
	}
}
