package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTrialSelectionScore(t *testing.T) {
	Convey("Given no holdout replays", t, func() {
		score := trialSelectionScore(12.5, nil)

		Convey("It should rank by train score", func() {
			So(score, ShouldEqual, 12.5)
		})
	})

	Convey("Given multiple holdout scores", t, func() {
		score := trialSelectionScore(40, []float64{8, 15, 11})

		Convey("It should rank by the minimum holdout score", func() {
			So(score, ShouldEqual, 8)
		})
	})
}
