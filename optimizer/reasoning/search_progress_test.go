package reasoning

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSearchProgressMessage(t *testing.T) {
	Convey("Given search progress phases", t, func() {
		Convey("It should format config and round lines", func() {
			configLine := SearchProgress{
				Phase:         "config",
				BeamSize:      8,
				MaxRounds:     20,
				MaxNodes:      24,
				Patience:      4,
				MinRoundTrips: 3,
				Workers:       16,
			}.Message()

			So(configLine, ShouldContainSubstring, "search config:")
			So(configLine, ShouldContainSubstring, "beam=8")
			So(configLine, ShouldContainSubstring, "workers=16")

			roundLine := SearchProgress{
				Phase:      "round",
				Round:      2,
				MaxRounds:  20,
				Evaluated:  40,
				RoundAdded: 12,
				BeamSize:   8,
				BestScore:  0.5,
				BestReturn: 0.4,
				BestTrades: 3,
				Stagnation: 1,
				Patience:   4,
			}.Message()

			So(roundLine, ShouldContainSubstring, "round 2/20")
			So(roundLine, ShouldContainSubstring, "evaluated=40 (+12)")
			So(roundLine, ShouldContainSubstring, "stagnation=1/4")
		})
	})
}

func TestSearchReportsProgress(t *testing.T) {
	Convey("Given a profitable tape and a progress hook", t, func() {
		rows := rallyTape()
		phases := make([]string, 0, 8)

		Search(context.Background(), rows, frictionlessCosts(), SearchConfig{
			BeamWidth: 4,
			MaxRounds: 2,
			Patience:  2,
			OnProgress: func(progress SearchProgress) {
				phases = append(phases, progress.Phase)
			},
		})

		Convey("It should emit the major search phases", func() {
			So(phases, ShouldContain, "config")
			So(phases, ShouldContain, "vocabulary")
			So(phases, ShouldContain, "precompile_start")
			So(phases, ShouldContain, "precompile_done")
			So(phases, ShouldContain, "seeds_start")
			So(phases, ShouldContain, "seeds_done")
			So(phases, ShouldContain, "done")
		})
	})
}
