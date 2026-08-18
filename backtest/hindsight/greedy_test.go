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
		// 100 -> 120 (up), back to 100, up to 125, back to 100.
		result := RoundTrips(seriesFromPrices(100, 110, 120, 100, 110, 125, 100))

		Convey("It should carve two separate round trips", func() {
			So(len(result.Legs), ShouldEqual, 2)

			Convey("The first leg spans the first rise", func() {
				So(result.Legs[0].BuyPrice, ShouldEqual, 100)
				So(result.Legs[0].SellPrice, ShouldEqual, 120)
			})

			Convey("The second leg spans the second rise", func() {
				So(result.Legs[1].BuyPrice, ShouldEqual, 100)
				So(result.Legs[1].SellPrice, ShouldEqual, 125)
			})
		})

		Convey("Sum of leg profits should equal greedy", func() {
			expected := (120 - 100) + (125 - 100)
			So(result.Greedy, ShouldAlmostEqual, expected, 1e-9)

			legSum := result.Legs[0].ProfitPct + result.Legs[1].ProfitPct
			So(legSum, ShouldAlmostEqual, 0.45, 1e-9)
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
		// 100 -> 900 -> 100 -> 800. The leg model buys the trough and sells
		// the true peak, so the first leg books the 100->900 spike; a full
		// retracement to 100 then seeds a second leg into the 800 rebound.
		result := RoundTrips(seriesFromPrices(100, 900, 100, 800))

		Convey("It should capture every maximal round trip", func() {
			So(len(result.Legs), ShouldEqual, 2)
			So(result.Legs[0].BuyPrice, ShouldEqual, 100)
			So(result.Legs[0].SellPrice, ShouldEqual, 900)
			So(result.Legs[1].BuyPrice, ShouldEqual, 100)
			So(result.Legs[1].SellPrice, ShouldEqual, 800)
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
