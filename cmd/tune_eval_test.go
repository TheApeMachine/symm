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

	Convey("Given no holdout scores", t, func() {
		gap := trialTrainHoldoutGap(25, nil)

		Convey("It should return zero gap", func() {
			So(gap, ShouldEqual, 0)
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

	Convey("Given no holdout scores", t, func() {
		eligible := trialEligible(30, nil, 10)

		Convey("It should accept the candidate", func() {
			So(eligible, ShouldBeTrue)
		})
	})

	Convey("Given a disabled max gap", t, func() {
		eligible := trialEligible(30, []float64{5}, -1)

		Convey("It should accept the candidate", func() {
			So(eligible, ShouldBeTrue)
		})
	})
}

func TestBetterTuneCandidate(t *testing.T) {
	Convey("Given equal holdout fitness", t, func() {
		Convey("It should prefer less negative train fitness on ties", func() {
			So(betterTuneCandidate(0, -2, 0, -5), ShouldBeTrue)
			So(betterTuneCandidate(0, -6, 0, -2), ShouldBeFalse)
		})
	})

	Convey("Given higher holdout fitness", t, func() {
		Convey("It should replace the incumbent even when train is worse", func() {
			So(betterTuneCandidate(3, -10, 1, 5), ShouldBeTrue)
		})
	})
}

func TestResolveMaxTrainHoldoutGap(t *testing.T) {
	Convey("Given zero requested gap", t, func() {
		gap := resolveMaxTrainHoldoutGap(0, 200)

		Convey("It should default to 3% of wallet EUR", func() {
			So(gap, ShouldAlmostEqual, 6, 0.0001)
		})
	})

	Convey("Given negative requested gap", t, func() {
		gap := resolveMaxTrainHoldoutGap(-1, 200)

		Convey("It should disable overfit rejection", func() {
			So(gap, ShouldEqual, -1)
		})
	})

	Convey("Given a positive requested gap", t, func() {
		gap := resolveMaxTrainHoldoutGap(12.5, 200)

		Convey("It should pass the requested value through", func() {
			So(gap, ShouldEqual, 12.5)
		})
	})

	Convey("Given zero wallet EUR with default requested gap", t, func() {
		gap := resolveMaxTrainHoldoutGap(0, 0)

		Convey("It should disable overfit rejection", func() {
			So(gap, ShouldEqual, -1)
		})
	})
}
