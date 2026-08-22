package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRoundTrips(t *testing.T) {
	Convey("Given a monotonically rising series", t, func() {
		result := RoundTrips(seriesFromPrices(100, 110, 120, 130))

		Convey("It should carve one leg from trough to peak", func() {
			So(len(result.Legs), ShouldEqual, 1)
			So(result.Legs[0].BuyPrice, ShouldEqual, 100)
			So(result.Legs[0].SellPrice, ShouldEqual, 130)
			So(result.Legs[0].ProfitPct, ShouldAlmostEqual, 0.3, 1e-9)
		})

		Convey("Greedy should equal the full rise (final minus first)", func() {
			So(result.Greedy, ShouldAlmostEqual, 30.0, 1e-9)
			So(result.Greedy, ShouldAlmostEqual, 130-100, 1e-9)
		})
	})

	Convey("Given a monotonically falling series", t, func() {
		result := RoundTrips(seriesFromPrices(100, 90, 80, 70))

		Convey("It should carve no profitable legs", func() {
			So(len(result.Legs), ShouldEqual, 0)
		})

		Convey("Greedy should be zero", func() {
			So(result.Greedy, ShouldAlmostEqual, 0.0, 1e-9)
		})
	})

	Convey("Given a sawtooth series", t, func() {
		// 100 -> 120 (up), back to 100, up to 125, back to 100. The later
		// peak exceeds the first, so a hold from the first trough rides
		// through the retracement to the higher peak.
		result := RoundTrips(seriesFromPrices(100, 110, 120, 100, 110, 125, 100))

		Convey("It should hold one position from trough to the highest peak", func() {
			So(len(result.Legs), ShouldEqual, 1)

			Convey("The single hold spans the full rise", func() {
				So(result.Legs[0].BuyPrice, ShouldEqual, 100)
				So(result.Legs[0].SellPrice, ShouldEqual, 125)
			})
		})

		Convey("Leg profit should be trough-to-peak while greedy stays per-step", func() {
			So(result.Legs[0].ProfitPct, ShouldAlmostEqual, 0.25, 1e-9)
			So(result.Greedy, ShouldAlmostEqual, 45.0, 1e-9)
		})
	})

	Convey("Given a partial-retracement series", t, func() {
		// Rise to 120, retrace to 110, then rise to 150. The swing walk must
		// not close a leg at 110 (price is still above the 100 trough); it
		// buys at 100 and sells at 150.
		result := RoundTrips(seriesFromPrices(100, 110, 120, 110, 130, 150))

		Convey("It should treat the whole rise as one leg", func() {
			So(len(result.Legs), ShouldEqual, 1)
			So(result.Legs[0].BuyPrice, ShouldEqual, 100)
			So(result.Legs[0].SellPrice, ShouldEqual, 150)
		})

		Convey("Greedy should be strictly greater than the single leg", func() {
			// Greedy keeps the positive steps: 100->110, 110->120, then after
			// the drop to 110 it re-harvests 110->130, 130->150 = 60. The leg
			// only books the trough-to-peak 100->150 = 50.
			So(result.Greedy, ShouldAlmostEqual, 60.0, 1e-9)
			So(result.Legs[0].ProfitPct, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})

	Convey("Given a partial-retracement that recovers", t, func() {
		// 100 -> 120 -> 110 -> 150. Greedy: +20 (to 120), then falls to 110
		// (adds nothing), then +40 (110->150) = 60. Leg: buy 100 sell 150 = 50.
		result := RoundTrips(seriesFromPrices(100, 120, 110, 150))

		Convey("Greedy should capture the round-trip re-entry at 110", func() {
			So(result.Greedy, ShouldAlmostEqual, 60.0, 1e-9)
			So(len(result.Legs), ShouldEqual, 1)
			So(result.Legs[0].ProfitPct, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})

	Convey("Given a flat series", t, func() {
		result := RoundTrips(seriesFromPrices(100, 100, 100, 100))

		Convey("It should carve no legs and yield zero greedy", func() {
			So(len(result.Legs), ShouldEqual, 0)
			So(result.Greedy, ShouldAlmostEqual, 0.0, 1e-9)
		})
	})

	Convey("Given a single point or empty series", t, func() {
		Convey("It should not panic and should report zero greed", func() {
			So(RoundTrips(seriesFromPrices(100)).Greedy, ShouldAlmostEqual, 0.0, 1e-9)
			So(RoundTrips(&Series{Symbol: "X", Points: []Point{}}).Greedy, ShouldAlmostEqual, 0.0, 1e-9)
		})
	})

	Convey("Given a nil series", t, func() {
		Convey("It should return an empty result", func() {
			result := RoundTrips(nil)
			So(len(result.Legs), ShouldEqual, 0)
			So(result.Symbol, ShouldEqual, "")
		})
	})

	Convey("Given an adversarial spike-and-crash series", t, func() {
		// 100 -> 900 -> 100 -> 800. The long-hold view books one position
		// from the best entry to the best exit; the rebound to 800 never
		// beats the 900 peak, so it stays inside the same hold.
		result := RoundTrips(seriesFromPrices(100, 900, 100, 800))

		Convey("It should capture the one dominant hold", func() {
			So(len(result.Legs), ShouldEqual, 1)
			So(result.Legs[0].BuyPrice, ShouldEqual, 100)
			So(result.Legs[0].SellPrice, ShouldEqual, 900)
		})
	})

	Convey("Given a retracement that makes a new low", t, func() {
		// 100 -> 120 -> 90 -> 130. The 90 print is a strictly better entry
		// than 100, so the hold books 100->120 and a fresh hold starts at 90.
		result := RoundTrips(seriesFromPrices(100, 110, 120, 90, 110, 130))

		Convey("It should split once a better entry appears", func() {
			So(len(result.Legs), ShouldEqual, 2)
			So(result.Legs[0].BuyPrice, ShouldEqual, 100)
			So(result.Legs[0].SellPrice, ShouldEqual, 120)
			So(result.Legs[1].BuyPrice, ShouldEqual, 90)
			So(result.Legs[1].SellPrice, ShouldEqual, 130)
		})
	})
}

func sampleSeries(pointCount int) *Series {
	points := make([]Point, 0, pointCount)

	for index := 0; index < pointCount; index++ {
		amplitude := 100 + float64(index%64)
		points = append(points, Point{
			At:    epoch.Add(time.Duration(index) * time.Second),
			Price: amplitude + 20*sinWave(index),
			Qty:   1,
		})
	}

	return &Series{Symbol: "BENCH/USD", Points: points}
}

func sinWave(index int) float64 {
	return float64(index%7)/7.0 - 0.5
}

func BenchmarkRoundTrips(b *testing.B) {
	series := sampleSeries(10000)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		result := RoundTrips(series)
		_ = result.Greedy
	}
}
