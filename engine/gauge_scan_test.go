package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGaugeScanPeakGaugeConfidence(t *testing.T) {
	Convey("Given a gauge scan with observed scores", t, func() {
		gaugeScan := GaugeScan{}

		gaugeScan.ObserveGaugeScore(0.2)
		gaugeScan.ObserveGaugeScore(0.6)

		Convey("It should return the peak score", func() {
			So(gaugeScan.PeakGaugeConfidence(), ShouldAlmostEqual, 0.6, 0.0001)
		})
	})
}

func TestGaugeScanResetGaugeScan(t *testing.T) {
	Convey("Given a reset gauge scan", t, func() {
		gaugeScan := GaugeScan{}
		gaugeScan.ObserveGaugeScore(0.8)
		gaugeScan.ResetGaugeScan()

		Convey("It should report zero peak confidence", func() {
			So(gaugeScan.PeakGaugeConfidence(), ShouldEqual, 0)
		})
	})
}

func TestGaugeScanIgnoresZeroScores(t *testing.T) {
	Convey("Given unscored symbols mixed with one reading", t, func() {
		gaugeScan := GaugeScan{}

		gaugeScan.ObserveGaugeScore(0)
		gaugeScan.ObserveGaugeScore(0)
		gaugeScan.ObserveGaugeScore(0.6)

		Convey("It should ignore zero readings", func() {
			So(gaugeScan.PeakGaugeConfidence(), ShouldAlmostEqual, 0.6, 0.0001)
		})
	})
}
