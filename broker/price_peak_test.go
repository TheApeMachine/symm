package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
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

func TestApplyTickerRows(t *testing.T) {
	Convey("applyTickerRows stores quote volume and peak range", t, func() {
		price := &Price{normalizer: spot.NewNormalizer()}
		price.applyTickerRows(map[string]kraken.TickerRow{
			"AAA/USD": {
				Last:   []string{"109", "1"},
				Volume: []string{"10", "500"},
				VWAP:   []string{"2", "2"},
				High:   []string{"110", "110"},
				Low:    []string{"100", "100"},
				Open:   "100",
			},
		})

		So(price.VolumeUSD("AAA/USD"), ShouldEqual, 1000)
		frac, ok := price.PeakFrac("AAA/USD")
		So(ok, ShouldBeTrue)
		So(frac, ShouldAlmostEqual, 0.9, 1e-9)
	})
}
