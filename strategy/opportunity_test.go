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

		Convey("a two-advisor council does not satisfy the minimum quorum", func() {
			twoAdvisorConsensus := &advisor.DeliberationOutcome{
				Participants: 2,
				DominantMove: advisor.MoveSteadyTrend,
				Confidence:   0.8,
			}

			opportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: twoAdvisorConsensus,
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

		Convey("an adverse dominant category aborts candidate entry", func() {
			strongConsensus := &advisor.DeliberationOutcome{
				Participants: 3,
				DominantMove: advisor.MoveExplosivePump,
				Confidence:   0.85,
				Probabilities: map[advisor.MarketMove]float64{
					advisor.MoveExplosivePump: 0.60,
					advisor.MoveSteadyTrend:   0.25,
				},
			}

			collapsedOpportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: strongConsensus,
				Categories: []types.Category{
					{Type: types.MechanicalCollapse, Confidence: 0.8},
				},
				At: currentTime,
			})

			So(collapsedOpportunity, ShouldBeNil)

			exhaustedOpportunity := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: strongConsensus,
				Categories: []types.Category{
					{Type: types.FadedExhaustion, Confidence: 0.75},
				},
				At: currentTime,
			})

			So(exhaustedOpportunity, ShouldBeNil)
		})

		Convey("a bullish dominant category reinforces probUp", func() {
			strongConsensus := &advisor.DeliberationOutcome{
				Participants: 3,
				DominantMove: advisor.MoveExplosivePump,
				Confidence:   0.85,
				Probabilities: map[advisor.MarketMove]float64{
					advisor.MoveExplosivePump: 0.60,
					advisor.MoveSteadyTrend:   0.25,
				},
			}

			baseline := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: strongConsensus,
				At:        currentTime,
			})

			reinforced := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: strongConsensus,
				Categories: []types.Category{
					{Type: types.VerticalIgnition, Confidence: 0.8},
				},
				At: currentTime,
			})

			So(baseline, ShouldNotBeNil)
			So(reinforced, ShouldNotBeNil)
			So(reinforced.Economics.TransitionProbability, ShouldBeGreaterThan, baseline.Economics.TransitionProbability)
		})

		Convey("cognition ambiguity and adverse predictions discount probability or reject entry", func() {
			strongConsensus := &advisor.DeliberationOutcome{
				Participants: 3,
				DominantMove: advisor.MoveExplosivePump,
				Confidence:   0.85,
				Probabilities: map[advisor.MarketMove]float64{
					advisor.MoveExplosivePump: 0.60,
					advisor.MoveSteadyTrend:   0.25,
				},
			}

			baseline := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: strongConsensus,
				At:        currentTime,
			})

			ambiguousOpp := SynthesizeOpportunity(OpportunityInput{
				Symbol:    "TEST/USD",
				Consensus: strongConsensus,
				Cognition: &types.Cognition{
					Ambiguous: true,
				},
				At: currentTime,
			})

			So(ambiguousOpp, ShouldNotBeNil)
			So(ambiguousOpp.Economics.Uncertainty, ShouldBeGreaterThan, baseline.Economics.Uncertainty)
			So(ambiguousOpp.Economics.TransitionProbability, ShouldBeLessThan, baseline.Economics.TransitionProbability)

			adversePredOpp := SynthesizeOpportunity(OpportunityInput{
				Symbol: "TEST/USD",
				Consensus: &advisor.DeliberationOutcome{
					Participants: 3,
					DominantMove: advisor.MoveExplosivePump,
					Confidence:   0.60,
					Probabilities: map[advisor.MarketMove]float64{
						advisor.MoveExplosivePump: 0.40,
						advisor.MoveSteadyTrend:   0.20,
					},
				},
				Cognition: &types.Cognition{
					Predictions: map[string]float64{
						"mechanical_collapse": 0.8,
						"faded_exhaustion":    0.6,
					},
				},
				At: currentTime,
			})

			// 0.60 * (1 - 0.50) = 0.30 < 0.50 -> rejected
			So(adversePredOpp, ShouldBeNil)
		})
	})
}

/*
TestSplitDirectionalMassIsNotVetoedByArgmax pins the case the DominantMove gate
used to reject.

The move space divides "up" between ExplosivePump and SteadyTrend while
Stagnant stays a single undivided bin. A council can therefore place most of
its mass on rising prices and still name Stagnant its heaviest single bin. The
old gate read that argmax and refused to enter, so an opportunity the same
council's own distribution supported could never be synthesized.
*/
func TestSplitDirectionalMassIsNotVetoedByArgmax(t *testing.T) {
	Convey("Given a council whose upward mass is split across two bins", t, func() {
		currentTime := time.Unix(1700000000, 0)

		split := &advisor.DeliberationOutcome{
			Participants: 3,
			// Stagnant is the heaviest single bin at 0.28 ...
			DominantMove: advisor.MoveStagnant,
			Confidence:   0.28,
			Probabilities: map[advisor.MarketMove]float64{
				// ... yet 0.56 of the mass says price rises.
				advisor.MoveExplosivePump:      0.28,
				advisor.MoveSteadyTrend:        0.28,
				advisor.MoveWeakDrift:          0.04,
				advisor.MoveStagnant:           0.28,
				advisor.MoveWeakBleed:          0.04,
				advisor.MoveStructuralPullback: 0.04,
				advisor.MoveFlashDump:          0.04,
			},
		}

		opportunity := SynthesizeOpportunity(OpportunityInput{
			Symbol:    "TEST/USD",
			Consensus: split,
			At:        currentTime,
		})

		Convey("the aggregated direction carries the decision", func() {
			So(opportunity, ShouldNotBeNil)
			So(opportunity.Economics.TransitionProbability, ShouldAlmostEqual, 0.58, 1e-9)
		})

		Convey("its maturity reflects the direction, not the argmax bin", func() {
			So(opportunity.Maturity, ShouldBeGreaterThan, split.Confidence)
		})
	})
}

/*
TestBearishArgmaxStillRefused is the other half: removing the DominantMove gate
must not admit a council that actually expects the price to fall.
*/
func TestBearishArgmaxStillRefused(t *testing.T) {
	Convey("Given a council whose mass is bearish", t, func() {
		bearish := &advisor.DeliberationOutcome{
			Participants: 3,
			DominantMove: advisor.MoveStagnant,
			Confidence:   0.28,
			Probabilities: map[advisor.MarketMove]float64{
				advisor.MoveExplosivePump:      0.04,
				advisor.MoveSteadyTrend:        0.04,
				advisor.MoveWeakDrift:          0.04,
				advisor.MoveStagnant:           0.28,
				advisor.MoveWeakBleed:          0.04,
				advisor.MoveStructuralPullback: 0.28,
				advisor.MoveFlashDump:          0.28,
			},
		}

		opportunity := SynthesizeOpportunity(OpportunityInput{
			Symbol:    "TEST/USD",
			Consensus: bearish,
			At:        time.Unix(1700000000, 0),
		})

		Convey("no opportunity is synthesized", func() {
			So(opportunity, ShouldBeNil)
		})
	})
}

/*
TestArchetypeDescribesTheCouncilsOwnReading pins the label to the evidence.

The archetype was a constant, so every opportunity claimed a vertical
ignition — including entries taken on a council whose mass sat on steady
trend, which is a different and weaker claim. The label is what the decision
surface, the journal and hindsight all display as the reason a position
exists, so a fixed one makes every entry describe the same setup and none of
them describe the real one.
*/
func TestArchetypeDescribesTheCouncilsOwnReading(t *testing.T) {
	currentTime := time.Unix(1700000000, 0)

	build := func(pump, trend float64) *types.OpportunityCandidate {
		return SynthesizeOpportunity(OpportunityInput{
			Symbol: "TEST/USD",
			At:     currentTime,
			Consensus: &advisor.DeliberationOutcome{
				Participants: 3,
				Confidence:   0.4,
				Probabilities: map[advisor.MarketMove]float64{
					advisor.MoveExplosivePump:      pump,
					advisor.MoveSteadyTrend:        trend,
					advisor.MoveWeakDrift:          0.04,
					advisor.MoveStagnant:           0.20,
					advisor.MoveWeakBleed:          0.03,
					advisor.MoveStructuralPullback: 0.03,
					advisor.MoveFlashDump:          0.03,
				},
			},
		})
	}

	Convey("Given an up-case carried by explosive pump", t, func() {
		opportunity := build(0.45, 0.22)

		Convey("the opportunity claims a vertical ignition", func() {
			So(opportunity, ShouldNotBeNil)
			So(opportunity.Archetype, ShouldEqual, types.ArchetypeVerticalIgnition)
		})
	})

	Convey("Given an up-case carried by steady trend", t, func() {
		opportunity := build(0.22, 0.45)

		Convey("it does not claim an ignition it has no evidence for", func() {
			So(opportunity, ShouldNotBeNil)
			So(opportunity.Archetype, ShouldEqual, types.ArchetypeSustainedTrend)
		})
	})
}
