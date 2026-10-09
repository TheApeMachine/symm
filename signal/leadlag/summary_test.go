package leadlag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
)

func peerReading(lag, gain float64, leads bool) nmcorrelation.LeadLagReading {
	return nmcorrelation.LeadLagReading{
		LagCandidate: nmcorrelation.LagCandidate{X: lag},
		Defined:      true,
		Leads:        leads,
		AbsoluteGain: gain,
	}
}

func TestSummarize(t *testing.T) {
	Convey("Given four defined peers with hand-computed summaries", t, func() {
		readings := []nmcorrelation.LeadLagReading{
			peerReading(-0.2, 0.10, true),
			peerReading(0.1, 0.05, true),
			peerReading(0.3, -0.02, false),
			peerReading(-0.4, 0.15, false),
		}

		summary := summarize(readings)

		Convey("Lags, the led share, and gains reduce to their robust cross-peer values", func() {
			So(summary, ShouldHaveLength, 6)
			So(summary["defined_peer_count"], ShouldEqual, 4)
			// sorted lags -0.4, -0.2, 0.1, 0.3: median (-0.2 + 0.1) / 2
			So(summary["best_lag_seconds_median"], ShouldAlmostEqual, -0.05, 1e-12)
			// |lag - median| = 0.15, 0.15, 0.35, 0.35: median 0.25
			So(summary["best_lag_seconds_mad"], ShouldAlmostEqual, 0.25, 1e-12)
			// only the first peer both clears the search and follows the focal path
			So(summary["led_peer_share"], ShouldEqual, 0.25)
			So(summary["correlation_gain_mean"], ShouldAlmostEqual, 0.07, 1e-12)
			// sorted gains -0.02, 0.05, 0.10, 0.15: median (0.05 + 0.10) / 2
			So(summary["correlation_gain_median"], ShouldAlmostEqual, 0.075, 1e-12)
		})
	})

	Convey("Given three defined peers", t, func() {
		summary := summarize([]nmcorrelation.LeadLagReading{
			peerReading(-0.2, 0.10, true),
			peerReading(0.1, 0.05, true),
			peerReading(0.3, -0.02, false),
		})

		Convey("The odd-count median is the middle order statistic", func() {
			So(summary["best_lag_seconds_median"], ShouldAlmostEqual, 0.1, 1e-12)
			// |lag - 0.1| = 0.3, 0, 0.2: median 0.2
			So(summary["best_lag_seconds_mad"], ShouldAlmostEqual, 0.2, 1e-12)
			So(summary["led_peer_share"], ShouldAlmostEqual, 1.0/3.0, 1e-12)
			So(summary["correlation_gain_mean"], ShouldAlmostEqual, 0.13/3.0, 1e-12)
			So(summary["correlation_gain_median"], ShouldAlmostEqual, 0.05, 1e-12)
		})
	})

	Convey("Given no defined peers", t, func() {
		Convey("There is no evidence, so no summary key exists", func() {
			So(summarize(nil), ShouldBeEmpty)
		})
	})
}
