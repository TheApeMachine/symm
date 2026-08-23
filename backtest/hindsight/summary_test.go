package hindsight

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func recommendationOpportunity(
	symbol string,
	profit float64,
	current float64,
	suggested float64,
	category string,
	status string,
) MissedLeg {
	return MissedLeg{
		Leg:    Leg{Symbol: symbol, ProfitPct: profit},
		Missed: true,
		Diagnosis: Diagnosis{
			Category:        category,
			EvidenceQuality: 0.8,
			EvidenceStatus:  status,
			Recommendation: Recommendation{
				Key:          "admission:confidence",
				Kind:         RecommendationTuneParameter,
				Target:       "trading.admission.minimum_confidence",
				Title:        "Backtest confidence",
				Action:       "Replay the boundary.",
				Current:      current,
				Suggested:    suggested,
				HasCurrent:   true,
				HasSuggested: true,
				Adjustment:   "lower",
			},
		},
	}
}

func TestAggregateRecommendations(t *testing.T) {
	Convey("Given repeated misses against one stable admission boundary", t, func() {
		reports := []PerSymbol{
			{Symbol: "AAA/USD", Opportunities: []MissedLeg{
				recommendationOpportunity("AAA/USD", 0.1, 0.5, 0.45, DiagnosisAdmission, "complete"),
			}},
			{Symbol: "BBB/USD", Opportunities: []MissedLeg{
				recommendationOpportunity("BBB/USD", 0.2, 0.5, 0.48, DiagnosisAdmission, "partial"),
			}},
		}

		recommendations := AggregateRecommendations(reports)

		Convey("It should publish one sweep endpoint that covers the associated misses", func() {
			So(recommendations, ShouldHaveLength, 1)
			So(recommendations[0].ImpactPct, ShouldAlmostEqual, 0.3, 1e-12)
			So(recommendations[0].Occurrences, ShouldEqual, 2)
			So(recommendations[0].Suggested, ShouldAlmostEqual, 0.45, 1e-12)
			So(recommendations[0].Symbols, ShouldResemble, []string{"AAA/USD", "BBB/USD"})
			So(recommendations[0].Confidence, ShouldAlmostEqual, 0.8, 1e-12)
		})
	})

	Convey("Given a regulator boundary that moved during the capture", t, func() {
		reports := []PerSymbol{{Symbol: "AAA/USD", Opportunities: []MissedLeg{
			recommendationOpportunity("AAA/USD", 0.1, 0.5, 0.45, DiagnosisAdmission, "complete"),
			recommendationOpportunity("AAA/USD", 0.2, 0.55, 0.5, DiagnosisAdmission, "complete"),
		}}}

		recommendations := AggregateRecommendations(reports)

		Convey("It should keep the exact per-leg values instead of publishing a fictional aggregate", func() {
			So(recommendations, ShouldHaveLength, 1)
			So(recommendations[0].HasCurrent, ShouldBeFalse)
			So(recommendations[0].HasSuggested, ShouldBeFalse)
			So(recommendations[0].Action, ShouldContainSubstring, "boundary moved")
		})
	})
}

func TestRootCauseSummaries(t *testing.T) {
	Convey("Given missed legs with complete and missing evidence", t, func() {
		complete := recommendationOpportunity("AAA/USD", 0.2, 0.5, 0.48, DiagnosisAdmission, "complete")
		missing := recommendationOpportunity("BBB/USD", 0.1, 0.5, 0.48, DiagnosisObservability, "missing")
		reports := []PerSymbol{
			{Symbol: "AAA/USD", Opportunities: []MissedLeg{complete}},
			{Symbol: "BBB/USD", Opportunities: []MissedLeg{missing}},
		}

		Convey("It should rank causes by missed value and expose diagnostic coverage", func() {
			summaries := RootCauseSummaries(reports)
			So(summaries, ShouldHaveLength, 2)
			So(summaries[0].Category, ShouldEqual, DiagnosisAdmission)
			So(summaries[0].ImpactPct, ShouldAlmostEqual, 0.2, 1e-12)
			So(DiagnosticCoverage(reports), ShouldAlmostEqual, 0.5, 1e-12)
		})
	})
}
