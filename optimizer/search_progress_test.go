package optimizer

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSearchProgressStagnant(t *testing.T) {
	convey.Convey("Given a progress tracker with beam width 4", t, func() {
		progress := NewSearchProgress()
		improves := func(
			candidateScore float64, candidateTrades int,
			bestScore float64, bestTrades int,
		) bool {
			guard := OverfitGuard{}

			return guard.ImprovesPersistedBest(
				candidateScore, candidateTrades, bestScore, bestTrades,
			)
		}

		convey.Convey("It should stagnate after a full beam of non-improving scores", func() {
			progress.Record(0.10, 5, improves)

			for range 3 {
				progress.Record(0.05, 5, improves)
			}

			convey.So(progress.Stagnant(4), convey.ShouldBeFalse)

			progress.Record(0.05, 5, improves)

			convey.So(progress.Stagnant(4), convey.ShouldBeTrue)
		})

		convey.Convey("It should reset stagnation after a new best", func() {
			for range 4 {
				progress.Record(0.05, 5, improves)
			}

			convey.So(progress.Stagnant(4), convey.ShouldBeTrue)

			progress.Record(0.20, 6, improves)

			convey.So(progress.Stagnant(4), convey.ShouldBeFalse)
			convey.So(progress.SinceImprovement(), convey.ShouldEqual, 0)
		})
	})
}
