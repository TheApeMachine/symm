package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestPeakFrac(t *testing.T) {
	Convey("PeakFrac places last in the 24h high–low range", t, func() {
		price := &Price{}
		price.SetTicker("Q/USD", 100_000, 2.0, 110, 100, 109)

		frac, ok := price.PeakFrac("Q/USD")
		So(ok, ShouldBeTrue)
		So(frac, ShouldAlmostEqual, 0.9, 1e-9)
	})

	Convey("PeakFrac fails when the range is flat or missing", t, func() {
		price := &Price{}
		price.SetTicker("Q/USD", 100_000, 0, 100, 100, 100)

		_, ok := price.PeakFrac("Q/USD")
		So(ok, ShouldBeFalse)
	})
}

func TestBelowPeak(t *testing.T) {
	Convey("BelowPeak respects max_entry_peak_frac", t, func() {
		prev := system.Cfg.Risk.MaxEntryPeakFrac
		system.Cfg.Risk.MaxEntryPeakFrac = 0.90
		Reset(func() { system.Cfg.Risk.MaxEntryPeakFrac = prev })

		price := &Price{}

		Convey("It approves when last is under the cap", func() {
			price.SetTicker("Q/USD", 100_000, 2.0, 110, 100, 108)
			So(price.BelowPeak("Q/USD"), ShouldBeTrue)
		})

		Convey("It vetoes when last is at or above the cap", func() {
			price.SetTicker("Q/USD", 100_000, 2.0, 110, 100, 109)
			So(price.BelowPeak("Q/USD"), ShouldBeFalse)
		})

		Convey("It vetoes when range data is missing", func() {
			So(price.BelowPeak("MISSING/USD"), ShouldBeFalse)
		})
	})

	Convey("BelowPeak is disabled when the cap is out of (0,1)", t, func() {
		prev := system.Cfg.Risk.MaxEntryPeakFrac
		Reset(func() { system.Cfg.Risk.MaxEntryPeakFrac = prev })

		price := &Price{}
		price.SetTicker("Q/USD", 100_000, 2.0, 110, 100, 110)

		system.Cfg.Risk.MaxEntryPeakFrac = 0
		So(price.BelowPeak("Q/USD"), ShouldBeTrue)

		system.Cfg.Risk.MaxEntryPeakFrac = 1
		So(price.BelowPeak("Q/USD"), ShouldBeTrue)
	})
}
