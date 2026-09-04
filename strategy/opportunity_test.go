package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/types"
)

func TestSynthesizeOpportunity(t *testing.T) {
	Convey("Given opportunity evaluation context", t, func() {
		currentTime := time.Unix(1_700_000_000, 0)

		Convey("an isolated positive advisor does not create an opportunity", func() {
			isolatedConsensus := &advisor.DeliberationOutcome{
				Participants: 1,
				DominantMove: advisor.MoveExplosivePump,
				Confidence:   0.8,
			}

			opportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: isolatedConsensus,
				At:        currentTime,
			})

			So(opportunity, ShouldBeNil)
		})

		Convey("active vetoes invalidate the opportunity hypothesis", func() {
			vetoedConsensus := &advisor.DeliberationOutcome{
				Participants: 3,
				DominantMove: advisor.MoveExplosivePump,
				Confidence:   0.8,
				Vetoes:       []string{"sellers absorb every market buy at the ceiling"},
			}

			opportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: vetoedConsensus,
				At:        currentTime,
			})

			So(opportunity, ShouldBeNil)
		})

		Convey("a weak upward drift does not justify spending capital", func() {
			weakConsensus := &advisor.DeliberationOutcome{
				Participants: 3,
				DominantMove: advisor.MoveWeakDrift,
				Confidence:   0.6,
			}

			opportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: weakConsensus,
				At:        currentTime,
			})

			So(opportunity, ShouldBeNil)
		})

		Convey("a multi-advisor coherent bullish consensus synthesizes an opportunity", func() {
			strongConsensus := &advisor.DeliberationOutcome{
				Participants: 3,
				DominantMove: advisor.MoveExplosivePump,
				Confidence:   0.85,
				Probabilities: map[advisor.MarketMove]float64{
					advisor.MoveExplosivePump:      0.60,
					advisor.MoveSteadyTrend:        0.25,
					advisor.MoveWeakDrift:          0.05,
					advisor.MoveStagnant:           0.05,
					advisor.MoveWeakBleed:          0.02,
					advisor.MoveStructuralPullback: 0.02,
					advisor.MoveFlashDump:          0.01,
				},
			}

			opportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: strongConsensus,
				At:        currentTime,
			})

			So(opportunity, ShouldNotBeNil)
			So(opportunity.Direction, ShouldEqual, types.DirectionLong)
			So(opportunity.Phase, ShouldEqual, types.PhaseArmed)
			So(opportunity.Economics, ShouldNotBeNil)
			So(opportunity.Economics.Calibrated, ShouldBeTrue)
			So(opportunity.Economics.TransitionProbability, ShouldBeGreaterThan, 0.8)
			So(opportunity.Economics.FavorableExcursion.Mid, ShouldBeGreaterThan, 0)
		})

		Convey("high liquidation share rejects an artificial short squeeze pump", func() {
			squeezeConsensus := &advisor.DeliberationOutcome{
				Participants: 3,
				DominantMove: advisor.MoveExplosivePump,
				Confidence:   0.85,
				Probabilities: map[advisor.MarketMove]float64{
					advisor.MoveExplosivePump: 0.70,
					advisor.MoveSteadyTrend:   0.10,
				},
			}

			opportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:           "TEST/USD",
				Consensus:        squeezeConsensus,
				LiquidationShare: 0.50, // 50% of volume is forced liquidation
				At:               currentTime,
			})

			// Discounted probUp = 0.80 * (1 - 0.50) = 0.40 < 0.50, so rejected
			So(opportunity, ShouldBeNil)
		})

		Convey("cognition surprisal widens opportunity uncertainty", func() {
			strongConsensus := &advisor.DeliberationOutcome{
				Participants: 3,
				DominantMove: advisor.MoveExplosivePump,
				Confidence:   0.85,
				Probabilities: map[advisor.MarketMove]float64{
					advisor.MoveExplosivePump: 0.60,
					advisor.MoveSteadyTrend:   0.25,
				},
			}

			baselineOpportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: strongConsensus,
				At:        currentTime,
			})

			surprisedOpportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: strongConsensus,
				Cognition: &types.Cognition{
					InterpolatedSurprisal: 4.5,
				},
				At: currentTime,
			})

			So(baselineOpportunity, ShouldNotBeNil)
			So(surprisedOpportunity, ShouldNotBeNil)
			So(surprisedOpportunity.Economics.Uncertainty, ShouldBeGreaterThan, baselineOpportunity.Economics.Uncertainty)
		})
	})
}
