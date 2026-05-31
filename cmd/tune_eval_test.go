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

func TestTrialTrainHoldoutGap(t *testing.T) {
	Convey("Given train and holdout scores", t, func() {
		gap := trialTrainHoldoutGap(25, []float64{10, 18})

		Convey("It should measure train minus minimum holdout", func() {
			So(gap, ShouldEqual, 15)
		})
	})
}

func TestTrialEligible(t *testing.T) {
	Convey("Given a suspicious train-holdout gap", t, func() {
		eligible := trialEligible(30, []float64{5}, 10)

		Convey("It should reject the candidate", func() {
			So(eligible, ShouldBeFalse)
		})
	})

	Convey("Given a modest train-holdout gap", t, func() {
		eligible := trialEligible(12, []float64{8}, 10)

		Convey("It should keep the candidate", func() {
			So(eligible, ShouldBeTrue)
		})
	})
}

func TestResolveMaxTrainHoldoutGap(t *testing.T) {
	Convey("Given zero requested gap", t, func() {
		gap := resolveMaxTrainHoldoutGap(0, 200)

		Convey("It should default to 3% of wallet EUR", func() {
			So(gap, ShouldEqual, 6)
		})
	})

	Convey("Given negative requested gap", t, func() {
		gap := resolveMaxTrainHoldoutGap(-1, 200)

		Convey("It should disable overfit rejection", func() {
			So(gap, ShouldEqual, -1)
		})
	})
}
