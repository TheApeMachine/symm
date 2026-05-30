package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func testMeasurement(
	category perspectives.CategoryType,
	snr float64,
) perspectives.Measurement {
	return perspectives.Measurement{Category: category, SNR: snr}
}

func trendMeasurements() []perspectives.Measurement {
	return []perspectives.Measurement{
		testMeasurement(perspectives.CategoryRiskOnSurge, 1.3),
		testMeasurement(perspectives.CategoryEndogenousAlpha, 1.4),
		testMeasurement(perspectives.CategoryFrenzy, 1.2),
		testMeasurement(perspectives.CategoryAggressiveDrive, 1.6),
	}
}

func TestDecisions(t *testing.T) {
	convey.Convey("Given measurements that satisfy multiple playbooks", t, func() {
		measurements := append(
			trendMeasurements(),
			testMeasurement(perspectives.CategoryAggressiveDrive, 1.6),
		)

		convey.Convey("It should return every actionable perspective in registry order", func() {
			decisions := Decisions(measurements, nil)

			convey.So(decisions, convey.ShouldHaveLength, 2)
			convey.So(decisions[0].Name, convey.ShouldEqual, "trend")
			convey.So(decisions[1].Name, convey.ShouldEqual, "drive")
			convey.So(decisions[0].Action, convey.ShouldEqual, perspectives.ActionEnter)
		})
	})
}

func TestExitDecisions(t *testing.T) {
	convey.Convey("Given a held trend position and an active reversal", t, func() {
		measurements := []perspectives.Measurement{
			testMeasurement(perspectives.CategoryActiveReversal, 1.5),
		}
		observations := []perspectives.ObservationType{perspectives.ObservationHolding}

		convey.Convey("It should return a stop from the opening playbook", func() {
			exits := ExitDecisions(measurements, observations, "trend", true)
			action := MostUrgentExit(exits)

			convey.So(action, convey.ShouldNotBeNil)
			convey.So(*action, convey.ShouldEqual, perspectives.ActionStopLoss)
		})
	})
}

func BenchmarkDecisions(b *testing.B) {
	measurements := trendMeasurements()

	for b.Loop() {
		_ = Decisions(measurements, nil)
	}
}
