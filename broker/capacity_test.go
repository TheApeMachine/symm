package broker

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
vwapOf is the average price of taking quantity from levels best first.
*/
func vwapOf(levels []BookLevel, quantity float64) float64 {
	gross, left := 0.0, quantity

	for _, level := range levels {
		take := math.Min(level.Quantity, left)
		gross += take * level.Price
		left -= take

		if left <= 0 {
			break
		}
	}

	return gross / (quantity - math.Max(left, 0))
}

/*
bisectCapacity is the brute-force oracle: the largest quantity, found by
bisection over the displayed depth, whose VWAP stays within limit (sign -1:
at or above for a sell; +1: at or below for a buy).
*/
func bisectCapacity(levels []BookLevel, limit, sign float64) float64 {
	total := 0.0

	for _, level := range levels {
		total += level.Quantity
	}

	within := func(quantity float64) bool {
		return quantity <= 0 || sign*(vwapOf(levels, quantity)-limit) <= 1e-12
	}

	if within(total) {
		return total
	}

	low, high := 0.0, total

	for range 200 {
		mid := (low + high) / 2

		if within(mid) {
			low = mid
		} else {
			high = mid
		}
	}

	return low
}

func handBook() *BookView {
	return &BookView{
		At: time.Unix(1_700_000_000, 0),
		Bids: []BookLevel{
			{Price: 100, Quantity: 1}, {Price: 99.5, Quantity: 1}, {Price: 99, Quantity: 5}, {Price: 95, Quantity: 100},
		},
		Asks: []BookLevel{
			{Price: 100.5, Quantity: 0.3}, {Price: 101, Quantity: 0.5}, {Price: 101.5, Quantity: 10},
		},
		Complete: true,
	}
}

func TestCapacityWalk(t *testing.T) {
	Convey("Given a hand-built book with mid 100.25", t, func() {
		view := handBook()
		mid := 100.25

		Convey("Exit capacity at a 1.7% budget walks into the 95 level and matches the bisection oracle", func() {
			capacity := ExitCapacity(view, 0.017)
			limit := mid * (1 - 0.017)

			So(capacity.Defined, ShouldBeTrue)
			So(capacity.Bounded, ShouldBeFalse)
			So(capacity.Reference, ShouldEqual, mid)
			So(capacity.Quantity, ShouldBeGreaterThan, 7)
			So(capacity.Quantity, ShouldAlmostEqual, bisectCapacity(view.Bids, limit, -1), 1e-9)
			So(vwapOf(view.Bids, capacity.Quantity), ShouldAlmostEqual, limit, 1e-9)
		})

		Convey("Entry capacity takes every displayed ask when all of them fit the budget", func() {
			capacity := EntryCapacity(view, 0.017)

			So(capacity.Defined, ShouldBeTrue)
			So(capacity.Quantity, ShouldAlmostEqual, 10.8, 1e-12)
			So(capacity.Bounded, ShouldBeFalse)

			Convey("and reports a lower bound when that side holds its full configured depth", func() {
				view.FullAsk = true
				So(EntryCapacity(view, 0.017).Bounded, ShouldBeTrue)
			})
		})

		Convey("Entry capacity stops inside a level exactly where the VWAP meets the limit", func() {
			capacity := EntryCapacity(view, 0.004)
			limit := mid * 1.004

			So(capacity.Quantity, ShouldAlmostEqual, bisectCapacity(view.Asks, limit, 1), 1e-9)
			So(capacity.Quantity, ShouldBeGreaterThan, 0.3)
			So(capacity.Quantity, ShouldBeLessThan, 10.8)
		})

		Convey("A budget inside the half spread leaves no capacity at all", func() {
			So(ExitCapacity(view, 0.001).Quantity, ShouldEqual, 0)
			So(EntryCapacity(view, 0.001).Quantity, ShouldEqual, 0)
		})

		Convey("Every budget on a grid matches the oracle on both sides", func() {
			for budget := 0.0; budget < 0.08; budget += 0.0005 {
				So(ExitCapacity(view, budget).Quantity, ShouldAlmostEqual, bisectCapacity(view.Bids, mid*(1-budget), -1), 1e-8)
				So(EntryCapacity(view, budget).Quantity, ShouldAlmostEqual, bisectCapacity(view.Asks, mid*(1+budget), 1), 1e-8)
			}
		})

		Convey("A crossed or one-sided book has no defined capacity", func() {
			view.Asks[0].Price = 99
			So(ExitCapacity(view, 0.01).Defined, ShouldBeFalse)

			So(ExitCapacity(&BookView{Bids: view.Bids}, 0.01).Defined, ShouldBeFalse)
			So(ExitCapacity(nil, 0.01).Defined, ShouldBeFalse)
		})
	})
}

func TestSlippageBudget(t *testing.T) {
	Convey("Given an 0.8% taker fee", t, func() {
		Convey("The budget is half of the median matched gain left after the round-trip fee", func() {
			budget, source, err := SlippageBudget(0.008, []float64{0.10, 0.04, 0.06})

			So(err, ShouldBeNil)
			So(source, ShouldEqual, BudgetMatchedEdge)
			So(budget, ShouldAlmostEqual, (0.06-0.016)/2, 1e-15)
		})

		Convey("Without matched gains it is one taker fee, reported as the fallback", func() {
			budget, source, err := SlippageBudget(0.008, nil)

			So(err, ShouldBeNil)
			So(source, ShouldEqual, BudgetFeeFallback)
			So(budget, ShouldEqual, 0.008)
		})

		Convey("An edge that does not clear the round-trip fee is refused", func() {
			_, _, err := SlippageBudget(0.008, []float64{0.01, 0.016})
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given matched durations, or none readable", t, func() {
		hold, ok := ExpectedHold([]time.Duration{time.Minute, 3 * time.Minute, 2 * time.Minute})
		So(ok, ShouldBeTrue)
		So(hold, ShouldEqual, 2*time.Minute)

		_, ok = ExpectedHold(nil)
		So(ok, ShouldBeFalse)

		_, ok = ExpectedHold([]time.Duration{0})
		So(ok, ShouldBeFalse)
	})
}

func TestFlowNoiseWarmup(t *testing.T) {
	Convey("Given signed flow observed for less than nine expected holds", t, func() {
		now := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
		hold := 9 * time.Minute

		Convey("with no trade, participation is undefined", func() {
			_, _, ok := NewFlow().Noise("X/USD", now, hold)
			So(ok, ShouldBeFalse)
		})

		Convey("nine minutes of alternating one-minute flow scale by sqrt(hold/window)", func() {
			flow := NewFlow()
			// A negligible first trade makes the span exactly nine minutes;
			// it sits on the oldest window's open boundary, outside it.
			flow.Record("X/USD", now.Add(-9*time.Minute), "buy", 0.0000001)

			// One trade per minute window, +1/-1 alternating, oldest first.
			for k := 8; k >= 0; k-- {
				side := "buy"
				if k%2 == 1 {
					side = "sell"
				}
				flow.Record("X/USD", now.Add(-time.Duration(k)*time.Minute-30*time.Second), side, 1)
			}
			noise, windows, ok := flow.Noise("X/USD", now, hold)
			So(ok, ShouldBeTrue)
			So(windows, ShouldEqual, 9)

			// Window sums, one-minute windows scaled to the nine-minute hold.
			sums := []float64{1, -1, 1, -1, 1, -1, 1, -1, 1}
			mean := 1.0 / 9
			variance := 0.0
			for _, sum := range sums {
				variance += (sum - mean) * (sum - mean)
			}
			expected := math.Sqrt(variance/8) * math.Sqrt(9)
			So(noise, ShouldAlmostEqual, expected, 1e-9)
		})

		Convey("with nine complete holds it is the unscaled hold-window deviation", func() {
			flow := NewFlow()
			for k := 9; k >= 0; k-- {
				side := "buy"
				if k%2 == 1 {
					side = "sell"
				}
				flow.Record("X/USD", now.Add(-time.Duration(k)*hold-time.Second), side, 2)
			}
			// Ten trades span nine holds and a second: nine complete holds.
			_, windows, ok := flow.Noise("X/USD", now, hold)
			So(ok, ShouldBeTrue)
			So(windows, ShouldEqual, 9)
		})
	})
}
