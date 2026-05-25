package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAlphaEMAUpdate(t *testing.T) {
	t.Parallel()

	Convey("Given an AlphaEMA on the first observation", t, func() {
		var ema AlphaEMA

		So(ema.Update(7.5, 0.1), ShouldBeNil)
		So(ema.Value(), ShouldEqual, 7.5)
		So(ema.Updates(), ShouldEqual, 1)
	})

	Convey("Given an AlphaEMA, Update rejects alpha outside (0,1]", t, func() {
		var ema AlphaEMA

		So(ema.Update(1, 0), ShouldNotBeNil)
		So(ema.Update(1, 1.01), ShouldNotBeNil)
	})

	Convey("Given an AlphaEMA after the seeding update", t, func() {
		var ema AlphaEMA

		_ = ema.Update(100, 0.5)
		_ = ema.Update(0, 0.5)

		So(ema.Value(), ShouldEqual, 50)
		So(ema.Updates(), ShouldEqual, 2)
	})
}

func TestAlphaEMAValue(t *testing.T) {
	t.Parallel()

	Convey("Given a fresh AlphaEMA", t, func() {
		var ema AlphaEMA

		Convey("Value should be zero before any Update", func() {
			So(ema.Value(), ShouldEqual, 0)
		})
	})
}

func TestAlphaEMAUpdates(t *testing.T) {
	t.Parallel()

	Convey("Given an AlphaEMA after three updates", t, func() {
		var ema AlphaEMA

		_ = ema.Update(1, 0.2)
		_ = ema.Update(2, 0.2)
		_ = ema.Update(3, 0.2)

		So(ema.Updates(), ShouldEqual, 3)
	})
}

func TestAlphaEMAReset(t *testing.T) {
	t.Parallel()

	Convey("Given an AlphaEMA with history", t, func() {
		var ema AlphaEMA

		_ = ema.Update(9, 0.3)

		ema.Reset()

		So(ema.Value(), ShouldEqual, 0)
		So(ema.Updates(), ShouldEqual, 0)
	})
}

func BenchmarkAlphaEMAUpdate(b *testing.B) {
	var ema AlphaEMA

	for idx := 0; idx < b.N; idx++ {
		_ = ema.Update(float64(idx%17), 0.05)
	}
}
