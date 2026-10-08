package temporal_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestVolumeBar(t *testing.T) {
	Convey("VolumeBar aggregates quantity, notional, and measures completed-bar duration and rates", t, func() {
		vb := temporal.NewVolumeBar(1.0)

		// Trade 1: price 50000, qty 0.5, at 1000000000 (1s), midpoint 50000
		var res1 []float64
		for ptr := range vb.Next(data.NewValue(50000.0, 0.5, 1000000000.0, 50000.0).Next(nil)) {
			res1 = append(res1, *(*float64)(ptr))
		}

		So(len(res1), ShouldEqual, 18)
		So(res1[0], ShouldEqual, 25000.0) // tradeNotional
		So(res1[1], ShouldEqual, 0.0)     // timeDelta
		So(res1[2], ShouldEqual, 1.0)     // barTarget
		So(res1[3], ShouldEqual, 0.5)     // barQuantity
		So(res1[4], ShouldEqual, 25000.0) // barNotional
		So(res1[5], ShouldEqual, 1.0)     // barTradeCount
		So(res1[6], ShouldEqual, 0.0)     // volumeBarDuration (not closed)
		So(res1[7], ShouldEqual, 0.0)     // volumeRate
		So(res1[10], ShouldEqual, 0.0)    // completedBars

		// Trade 2: price 50000, qty 0.5, at 1200000000 (1.2s), midpoint 50000 -> bar closes!
		var res2 []float64
		for ptr := range vb.Next(data.NewValue(50000.0, 0.5, 1200000000.0, 50000.0).Next(nil)) {
			res2 = append(res2, *(*float64)(ptr))
		}

		So(len(res2), ShouldEqual, 18)
		So(res2[0], ShouldEqual, 25000.0)
		So(res2[1], ShouldAlmostEqual, 0.2, 1e-9) // timeDelta
		So(res2[3], ShouldEqual, 1.0)             // barQuantity = 1.0
		So(res2[5], ShouldEqual, 2.0)             // barTradeCount = 2
		So(res2[6], ShouldAlmostEqual, 0.2, 1e-9) // volumeBarDuration = 0.2
		So(res2[7], ShouldAlmostEqual, 1.0/0.2, 1e-6)
		So(res2[8], ShouldAlmostEqual, 50000.0/0.2, 1e-6)
		So(res2[9], ShouldAlmostEqual, 2.0/0.2, 1e-6)
		So(res2[10], ShouldEqual, 1.0) // completedBars
		So(res2[13], ShouldEqual, 0.0) // midpointLogReturn (unchanged midpoint)

		// Trade 3: price 49950, qty 1.0, at 1400000000 (1.4s), midpoint 49950 -> dump bar closes!
		var res3 []float64
		for ptr := range vb.Next(data.NewValue(49950.0, 1.0, 1400000000.0, 49950.0).Next(nil)) {
			res3 = append(res3, *(*float64)(ptr))
		}
		expectedLogReturn := math.Log(49950.0 / 50000.0)
		So(res3[13], ShouldAlmostEqual, expectedLogReturn, 1e-12)
		So(res3[15], ShouldEqual, 0.0)
		So(res3[16], ShouldAlmostEqual, -expectedLogReturn, 1e-12)
	})
}
