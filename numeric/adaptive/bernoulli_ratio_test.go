package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBernoulliRatioObserve(t *testing.T) {
	t.Parallel()

	Convey("Given a BernoulliRatio", t, func() {
		var ratio BernoulliRatio

		Convey("Observe should tally hits and trials atomically", func() {
			ratio.Observe(true)
			ratio.Observe(false)
			ratio.Observe(true)

			So(ratio.Total(), ShouldEqual, 3)
			So(ratio.Ratio(), ShouldAlmostEqual, 2.0/3.0, 1e-9)
		})
	})
}

func TestBernoulliRatioTotal(t *testing.T) {
	t.Parallel()

	Convey("Given an empty BernoulliRatio", t, func() {
		var ratio BernoulliRatio

		Convey("Total should report zero trials", func() {
			So(ratio.Total(), ShouldEqual, 0)
		})
	})
}

func TestBernoulliRatioRatio(t *testing.T) {
	t.Parallel()

	Convey("Given an empty BernoulliRatio", t, func() {
		var ratio BernoulliRatio

		Convey("Ratio should be zero without trials", func() {
			So(ratio.Ratio(), ShouldEqual, 0)
		})
	})
}

func TestBernoulliRatioReset(t *testing.T) {
	t.Parallel()

	Convey("Given a BernoulliRatio after observations", t, func() {
		var ratio BernoulliRatio

		ratio.Observe(true)

		ratio.Reset()

		So(ratio.Total(), ShouldEqual, 0)
		So(ratio.Ratio(), ShouldEqual, 0)
	})
}

func BenchmarkBernoulliRatioObserve(b *testing.B) {
	var ratio BernoulliRatio

	for idx := 0; idx < b.N; idx++ {
		ratio.Observe(idx%3 != 0)
	}
}
