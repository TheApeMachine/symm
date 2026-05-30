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
	return perspectives.Measurement{Category: category, SNR: snr, Confidence: 0.8}
}

func TestDecisions(t *testing.T) {
	convey.Convey("Given measurements that satisfy multiple playbooks", t, func() {
		measurements := []perspectives.Measurement{
			testMeasurement(perspectives.CategoryEndogenousAlpha, 1.4),
			testMeasurement(perspectives.CategoryAggressiveDrive, 1.6),
		}

		convey.Convey("It should return every actionable perspective in registry order", func() {
			decisions := Decisions(measurements, nil)

			convey.So(decisions, convey.ShouldHaveLength, 2)
			convey.So(decisions[0].Name, convey.ShouldEqual, "trend")
			convey.So(decisions[1].Name, convey.ShouldEqual, "drive")
			convey.So(decisions[0].Action, convey.ShouldEqual, perspectives.ActionEnter)
		})

		convey.Convey("Decide should preserve the deterministic priority verdict", func() {
			action, perspective := Decide(measurements, nil)

			convey.So(action, convey.ShouldNotBeNil)
			convey.So(*action, convey.ShouldEqual, perspectives.ActionEnter)
			convey.So(perspective, convey.ShouldNotBeNil)
		})
	})
}

func BenchmarkDecisions(b *testing.B) {
	measurements := []perspectives.Measurement{
		testMeasurement(perspectives.CategoryEndogenousAlpha, 1.4),
		testMeasurement(perspectives.CategoryAggressiveDrive, 1.6),
	}

	for b.Loop() {
		_ = Decisions(measurements, nil)
	}
}
