package cmd

import (
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShouldSubmitTuneTrial(t *testing.T) {
	Convey("Given a bounded tune run", t, func() {
		options := tuneRunOptions{maxTrials: 3}

		Convey("It should submit until the trial limit is reached", func() {
			So(shouldSubmitTuneTrial(options, 0), ShouldBeTrue)
			So(shouldSubmitTuneTrial(options, 2), ShouldBeTrue)
			So(shouldSubmitTuneTrial(options, 3), ShouldBeFalse)
		})
	})

	Convey("Given a continuous tune run", t, func() {
		options := tuneRunOptions{maxTrials: 0}

		Convey("It should keep submitting trials", func() {
			So(shouldSubmitTuneTrial(options, 1_000_000), ShouldBeTrue)
		})
	})
}

func TestTuneTrialPerturbSeed(t *testing.T) {
	Convey("Given tune trial indices after the baseline", t, func() {
		Convey("It should reserve seed one for the baseline and assign stable trial seeds", func() {
			So(tuneTrialPerturbSeed(0), ShouldEqual, 2)
			So(tuneTrialPerturbSeed(1), ShouldEqual, 3)
			So(tuneTrialPerturbSeed(8), ShouldEqual, 10)
		})
	})
}

func TestCountRejectedTuneTrial(t *testing.T) {
	Convey("Given rejected tune trial reasons", t, func() {
		overfit := atomic.Int64{}
		noProfit := atomic.Int64{}

		countRejectedTuneTrial(tuneRejectOverfit, &overfit, &noProfit)
		countRejectedTuneTrial(tuneRejectNoProfit, &overfit, &noProfit)

		Convey("It should keep overfit and no-profit counters separate", func() {
			So(overfit.Load(), ShouldEqual, 1)
			So(noProfit.Load(), ShouldEqual, 1)
		})
	})
}
