package driver

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/backtest/hindsight"
)

func TestHindsightWire(t *testing.T) {
	Convey("Given a capture report with structured causes, experiments, and losses", t, func() {
		recommendation := hindsight.Recommendation{
			Key:          "admission:confidence",
			Kind:         hindsight.RecommendationTuneParameter,
			Target:       "trading.admission.minimum_confidence",
			Title:        "Backtest confidence",
			Action:       "Replay the boundary.",
			Rationale:    "The exact gate stopped entry.",
			Current:      0.5,
			Suggested:    0.48,
			HasCurrent:   true,
			HasSuggested: true,
			Adjustment:   "lower",
			Confidence:   0.9,
			ImpactPct:    0.2,
			Occurrences:  1,
			Symbols:      []string{"TEST/USD"},
		}
		report := RealizedReport{
			CaptureID:          9,
			Status:             "ready",
			MissedPct:          0.2,
			RealizedPct:        0.1,
			LossPct:            0.05,
			UpboundPct:         0.3,
			MissedLegs:         1,
			TotalLegs:          2,
			LossPositions:      1,
			ValueCaptureRate:   1.0 / 3.0,
			LegCaptureRate:     0.5,
			DiagnosticCoverage: 1,
			RootCauses: []hindsight.RootCauseSummary{{
				Category:    hindsight.DiagnosisAdmission,
				ImpactPct:   0.2,
				Occurrences: 1,
				Symbols:     []string{"TEST/USD"},
			}},
			Recommendations: []hindsight.Recommendation{recommendation},
			LossRootCauses: []hindsight.RootCauseSummary{{
				Category:    hindsight.DiagnosisWhipsawStopout,
				ImpactPct:   0.05,
				Occurrences: 1,
				Symbols:     []string{"TEST/USD"},
			}},
			LossRecommendations: []hindsight.Recommendation{recommendation},
			Symbols: []hindsight.PerSymbol{{
				Symbol:        "TEST/USD",
				UpboundPct:    0.3,
				RealizedPct:   0.1,
				MissedPct:     0.2,
				LossPct:       0.05,
				Legs:          2,
				MissedLegs:    1,
				LossPositions: 1,
				Opportunities: []hindsight.MissedLeg{{
					Leg: hindsight.Leg{
						Symbol:         "TEST/USD",
						BuyAt:          time.Unix(1, 0).UTC(),
						SellAt:         time.Unix(2, 0).UTC(),
						BuyPrice:       100,
						SellPrice:      120,
						ProfitPct:      0.19,
						GrossProfitPct: 0.20,
						FrictionPct:    0.01,
					},
					Signal: hindsight.SignalContext{Action: "nothing"},
					Diagnosis: hindsight.Diagnosis{
						Category:        hindsight.DiagnosisAdmission,
						Summary:         "confidence below admission",
						EvidenceQuality: 0.9,
						EvidenceStatus:  "complete",
						Blockers: []hindsight.Blocker{{
							Key:         "admission:confidence",
							Category:    hindsight.DiagnosisAdmission,
							Label:       "entry confidence",
							Observed:    0.48,
							Target:      0.5,
							HasTarget:   true,
							Gap:         0.02,
							Severity:    0.02,
							Explanation: "below required confidence",
						}},
						Recommendation: recommendation,
					},
					Why:    "confidence below admission",
					Missed: true,
				}},
				Losses: []hindsight.PositionLoss{{
					Symbol:        "TEST/USD",
					DecisionID:    "dec-1",
					EntryAt:       time.Unix(1, 0).UTC(),
					ExitAt:        time.Unix(2, 0).UTC(),
					EntryPrice:    100,
					ExitPrice:     95,
					ReturnPct:     -0.05,
					GrossPct:      -0.05,
					TriggerReason: "stoploss: floor breached",
					Diagnosis: hindsight.Diagnosis{
						Category: hindsight.DiagnosisWhipsawStopout,
						Summary:  "stopped out by volatility wick",
					},
				}},
			}},
		}

		frame := hindsightWire(report)

		Convey("It should publish the backend verdicts and loss metrics", func() {
			So(frame.ValueCaptureRate, ShouldAlmostEqual, 1.0/3.0, 1e-12)
			So(frame.LossPct, ShouldAlmostEqual, 0.05, 1e-12)
			So(frame.LossPositions, ShouldEqual, 1)
			So(frame.RootCauses, ShouldHaveLength, 1)
			So(frame.LossRootCauses, ShouldHaveLength, 1)
			So(frame.Recommendations, ShouldHaveLength, 1)
			So(frame.Recommendations[0].Suggested, ShouldAlmostEqual, 0.48, 1e-12)
			So(frame.Symbols, ShouldHaveLength, 1)
			So(frame.Symbols[0].Opportunities, ShouldHaveLength, 1)
			So(frame.Symbols[0].Opportunities[0].Leg.GrossProfitPct, ShouldAlmostEqual, 0.20, 1e-12)
			So(frame.Symbols[0].Opportunities[0].Leg.FrictionPct, ShouldAlmostEqual, 0.01, 1e-12)
			So(frame.Symbols[0].Losses, ShouldHaveLength, 1)
			So(frame.Symbols[0].Losses[0].ReturnPct, ShouldAlmostEqual, -0.05, 1e-12)
			diagnosis := frame.Symbols[0].Opportunities[0].Diagnosis
			So(diagnosis, ShouldNotBeNil)
			So(diagnosis.Category, ShouldEqual, hindsight.DiagnosisAdmission)
			So(diagnosis.Blockers, ShouldHaveLength, 1)
		})
	})
}
