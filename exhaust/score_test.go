package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/ring"
)

func symbolHistoryFrom(
	bidDepths, spreads, pressures, imbalances []float64,
) symbolHistory {
	history := symbolHistory{
		bidDepths:  ring.NewFloatRing(exitHistoryCap),
		spreads:    ring.NewFloatRing(exitHistoryCap),
		pressures:  ring.NewFloatRing(exitHistoryCap),
		imbalances: ring.NewFloatRing(exitHistoryCap),
	}

	for _, value := range bidDepths {
		history.bidDepths.Push(value)
	}

	for _, value := range spreads {
		history.spreads.Push(value)
	}

	for _, value := range pressures {
		history.pressures.Push(value)
	}

	for _, value := range imbalances {
		history.imbalances.Push(value)
	}

	return history
}

func TestExitScoreLong(t *testing.T) {
	Convey("Given thinning bid depth and fading buy pressure", t, func() {
		history := symbolHistoryFrom(
			[]float64{100, 95, 90, 40, 35},
			[]float64{10, 10, 10, 10, 10},
			[]float64{0.8, 0.75, 0.7, 0.2},
			[]float64{0.5, 0.4, 0.3, -0.1},
		)

		urgency, reason := exitScoreLong(history)

		Convey("It should recommend an early exit", func() {
			So(urgency, ShouldBeGreaterThan, 0.30)
			So(reason, ShouldNotBeBlank)
		})
	})
}

func TestExitScoreShort(t *testing.T) {
	Convey("Given pressure flipping against a short", t, func() {
		history := symbolHistory{
			bidDepths:  ring.NewFloatRing(exitHistoryCap),
			askDepths:  ring.NewFloatRing(exitHistoryCap),
			spreads:    ring.NewFloatRing(exitHistoryCap),
			pressures:  ring.NewFloatRing(exitHistoryCap),
			imbalances: ring.NewFloatRing(exitHistoryCap),
		}

		for _, value := range []float64{80, 75, 70, 65} {
			history.bidDepths.Push(value)
			history.askDepths.Push(value)
		}

		for _, value := range []float64{12, 12, 12, 12} {
			history.spreads.Push(value)
		}

		for _, value := range []float64{-0.7, -0.65, 0.2} {
			history.pressures.Push(value)
		}

		for _, value := range []float64{-0.4, -0.35, 0.2} {
			history.imbalances.Push(value)
		}

		urgency, reason := exitScoreShort(history)

		Convey("It should recommend an early cover", func() {
			So(urgency, ShouldBeGreaterThan, 0.2)
			So(reason, ShouldNotBeBlank)
		})
	})
}

func BenchmarkExitScoreLong(b *testing.B) {
	history := symbolHistoryFrom(
		[]float64{100, 95, 90, 40, 35},
		[]float64{10, 10, 10, 10, 10},
		[]float64{0.8, 0.75, 0.7, 0.2},
		[]float64{0.5, 0.4, 0.3, -0.1},
	)

	b.ReportAllocs()

	for b.Loop() {
		exitScoreLong(history)
	}
}

func BenchmarkHistoryStoreObserve(b *testing.B) {
	store := newHistoryStore()

	b.ReportAllocs()

	for b.Loop() {
		store.observe("BTC/EUR", 100, 90, 10, 5, 0.6, 0.2, 50000)
	}
}
