package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGaugeScanMeanGaugeConfidence(t *testing.T) {
	Convey("Given a gauge scan with observed scores", t, func() {
		gaugeScan := GaugeScan{}

		gaugeScan.ObserveGaugeScore(0.2)
		gaugeScan.ObserveGaugeScore(0.6)

		Convey("It should return the arithmetic mean", func() {
			So(gaugeScan.MeanGaugeConfidence(), ShouldAlmostEqual, 0.4, 0.0001)
		})
	})
}

func TestGaugeScanResetGaugeScan(t *testing.T) {
	Convey("Given a reset gauge scan", t, func() {
		gaugeScan := GaugeScan{}
		gaugeScan.ObserveGaugeScore(0.8)
		gaugeScan.ResetGaugeScan()

		Convey("It should report zero mean confidence", func() {
			So(gaugeScan.MeanGaugeConfidence(), ShouldEqual, 0)
		})
	})
}

func TestGaugeScanIgnoresZeroScores(t *testing.T) {
	Convey("Given unscored symbols mixed with one reading", t, func() {
		gaugeScan := GaugeScan{}

		gaugeScan.ObserveGaugeScore(0)
		gaugeScan.ObserveGaugeScore(0)
		gaugeScan.ObserveGaugeScore(0.6)

		Convey("It should mean only positive scores", func() {
			So(gaugeScan.MeanGaugeConfidence(), ShouldAlmostEqual, 0.6, 0.0001)
		})
	})
}
