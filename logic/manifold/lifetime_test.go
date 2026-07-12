package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLifetimeEstimatorCDF(t *testing.T) {
	Convey("Given an empirical lifetime sample that has already been queried", t, func() {
		estimator := NewLifetimeEstimator(4)
		estimator.RecordCompleted(time.Second)
		estimator.RecordCompleted(3 * time.Second)

		So(estimator.CDF(time.Second), ShouldEqual, 0.5)

		Convey("When a completed observation changes the risk set", func() {
			estimator.RecordCompleted(2 * time.Second)

			Convey("It should return the exact refreshed Kaplan-Meier CDF", func() {
				So(estimator.CDF(time.Second), ShouldAlmostEqual, 1.0/3.0)
				So(estimator.CDF(2*time.Second), ShouldAlmostEqual, 2.0/3.0)
			})
		})
	})

	Convey("Given completed and censored observations at the same duration", t, func() {
		estimator := NewLifetimeEstimator(4)
		estimator.RecordCompleted(2 * time.Second)
		estimator.Censor(2 * time.Second)
		estimator.RecordCompleted(4 * time.Second)

		Convey("It should apply the event count to the complete tied risk set", func() {
			So(estimator.CDF(time.Second), ShouldEqual, 0)
			So(estimator.CDF(2*time.Second), ShouldAlmostEqual, 1.0/3.0)
			So(estimator.CDF(3*time.Second), ShouldAlmostEqual, 1.0/3.0)
			So(estimator.CDF(4*time.Second), ShouldEqual, 1.0)
		})
	})
}

func TestLifetimeEstimatorCensor(t *testing.T) {
	Convey("Given a full chronological lifetime sample", t, func() {
		estimator := NewLifetimeEstimator(3)
		estimator.RecordCompleted(time.Second)
		estimator.Censor(2 * time.Second)
		estimator.RecordCompleted(3 * time.Second)

		So(estimator.CDF(time.Second), ShouldAlmostEqual, 1.0/3.0)

		Convey("When a censored observation evicts the oldest observation", func() {
			estimator.Censor(4 * time.Second)

			Convey("It should remove the evicted event from the next exact estimate", func() {
				So(estimator.CDF(time.Second), ShouldEqual, 0)
				So(estimator.CDF(3*time.Second), ShouldEqual, 0.5)
			})
		})
	})
}
