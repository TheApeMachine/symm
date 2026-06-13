package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestQualifiesForOpportunityEntry(t *testing.T) {
	threshold := config.ThresholdConfig{
		EntryConfidenceBaseline:   0.55,
		TurbulenceConfidenceScale: 0.30,
		EntrySurpriseBaseline:     1.0,
	}

	Convey("Given a high-confidence pump measurement", t, func() {
		measurements := []Measurement{
			NewMeasurement(
				SourcePumpDump,
				"CELO/USD",
				0.25,
				1.2,
				100,
				0.01,
				1,
				CategoryVerticalIgnition,
				RegimeTypeNone,
				PositionTypeNone,
				0.9,
				1.4,
			),
		}

		Convey("It should qualify for an opportunity slot", func() {
			So(QualifiesForOpportunityEntry(measurements, threshold), ShouldBeTrue)
		})
	})

	Convey("Given elevated confidence without surprise", t, func() {
		measurements := []Measurement{
			NewMeasurement(
				SourcePumpDump,
				"CELO/USD",
				0.25,
				1.2,
				100,
				0.01,
				1,
				CategoryVerticalIgnition,
				RegimeTypeNone,
				PositionTypeNone,
				0.9,
				0.5,
			),
		}

		Convey("It should not qualify", func() {
			So(QualifiesForOpportunityEntry(measurements, threshold), ShouldBeFalse)
		})
	})

	Convey("Given surprise without a high-value category", t, func() {
		measurements := []Measurement{
			NewMeasurement(
				SourceHawkes,
				"BTC/USD",
				50_000,
				0.4,
				100,
				0.01,
				1,
				CategoryLaminar,
				RegimeTypeNone,
				PositionTypeNone,
				0.9,
				1.5,
			),
		}

		Convey("It should not qualify", func() {
			So(QualifiesForOpportunityEntry(measurements, threshold), ShouldBeFalse)
		})
	})
}
