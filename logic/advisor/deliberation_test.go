package advisor

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
perspectiveFor builds a resolved perspective whose top class is the named state.
*/
func perspectiveFor(advisorName, state string, probability float64, support uint64) *types.Perspective {
	return &types.Perspective{
		Symbol:  "TEST/USD",
		Advisor: advisorName,
		Support: support,
		Classes: []types.PerspectiveClass{
			{State: types.PerspectiveState(state), Probability: probability},
			{State: "Other", Probability: 1 - probability},
		},
	}
}

func TestDeliberationVetoSuppressesThePump(t *testing.T) {
	Convey("Given momentum building into sellers absorbing", t, func() {
		room := NewWarRoom()

		bullish := room.Deliberate([]*types.Perspective{
			perspectiveFor("momentum", "Building", 0.8, 100),
		}, "TEST/USD", time.Unix(1, 0))

		vetoed := room.Deliberate([]*types.Perspective{
			perspectiveFor("momentum", "Building", 0.8, 100),
			perspectiveFor("auction", "SellersAbsorbing", 0.92, 100),
		}, "TEST/USD", time.Unix(1, 0))

		Convey("the veto is recorded with its physical reason", func() {
			So(vetoed.Vetoes, ShouldNotBeEmpty)
		})

		Convey("the pump probability collapses relative to the unopposed read", func() {
			So(vetoed.Probabilities[MoveExplosivePump],
				ShouldBeLessThan, bullish.Probabilities[MoveExplosivePump])
		})

		Convey("the consensus no longer leans bullish", func() {
			So(vetoed.DominantMove, ShouldBeLessThan, MoveSteadyTrend)
		})
	})
}

func TestDeliberationSynergyAmplifiesTheCoil(t *testing.T) {
	Convey("Given a sweep followed by a bid wall", t, func() {
		room := NewWarRoom()

		alone := room.Deliberate([]*types.Perspective{
			perspectiveFor("pullback", "LiquiditySweep", 0.75, 100),
		}, "TEST/USD", time.Unix(1, 0))

		together := room.Deliberate([]*types.Perspective{
			perspectiveFor("pullback", "LiquiditySweep", 0.75, 100),
			perspectiveFor("liquidity", "WallBuilding", 0.80, 100),
		}, "TEST/USD", time.Unix(1, 0))

		Convey("the synergy is recorded", func() {
			So(together.Synergies, ShouldNotBeEmpty)
		})

		Convey("the pump probability exceeds either advisor alone", func() {
			So(together.Probabilities[MoveExplosivePump],
				ShouldBeGreaterThan, alone.Probabilities[MoveExplosivePump])
		})
	})
}

func TestDeliberationIsADistribution(t *testing.T) {
	Convey("Given any deliberation", t, func() {
		room := NewWarRoom()
		outcome := room.Deliberate([]*types.Perspective{
			perspectiveFor("momentum", "Building", 0.8, 100),
			perspectiveFor("liquidity", "VacuumForming", 0.7, 100),
		}, "TEST/USD", time.Unix(1, 0))

		Convey("the move probabilities sum to one", func() {
			total := 0.0

			for _, move := range AllMarketMoves {
				total += outcome.Probabilities[move]
			}

			So(total, ShouldAlmostEqual, 1.0, 0.000001)
		})

		Convey("every move stays reachable", func() {
			for _, move := range AllMarketMoves {
				So(outcome.Probabilities[move], ShouldBeGreaterThan, 0)
			}
		})
	})
}

func TestDeliberationIgnoresForeignSymbols(t *testing.T) {
	Convey("Given a perspective for another symbol", t, func() {
		room := NewWarRoom()
		foreign := perspectiveFor("momentum", "Building", 0.9, 100)
		foreign.Symbol = "OTHER/USD"

		outcome := room.Deliberate([]*types.Perspective{foreign}, "TEST/USD", time.Unix(1, 0))

		Convey("it does not join this symbol's deliberation", func() {
			So(outcome.Participants, ShouldEqual, 0)
		})
	})
}

func TestCredibilityPunishesTheFalseVeto(t *testing.T) {
	Convey("Given an advisor that vetoed a move which then happened", t, func() {
		room := NewWarRoom()
		before := room.Credibility("auction")

		room.UpdateCredibility("auction", true, MoveExplosivePump, MoveStagnant)

		Convey("its standing at the table falls", func() {
			So(room.Credibility("auction"), ShouldBeLessThan, before)
		})
	})

	Convey("Given an advisor whose veto avoided a dump", t, func() {
		room := NewWarRoom()
		room.UpdateCredibility("auction", true, MoveFlashDump, MoveStagnant)
		room.UpdateCredibility("auction", true, MoveFlashDump, MoveStagnant)

		Convey("its credibility never exceeds one", func() {
			So(room.Credibility("auction"), ShouldBeLessThanOrEqualTo, 1.0)
		})
	})

	Convey("Given repeated failures", t, func() {
		room := NewWarRoom()

		for range 50 {
			room.UpdateCredibility("momentum", true, MoveExplosivePump, MoveStagnant)
		}

		Convey("the advisor is never silenced entirely", func() {
			So(room.Credibility("momentum"), ShouldBeGreaterThanOrEqualTo, credibilityFloor)
		})
	})
}

func TestCredibilityWeightsInfluence(t *testing.T) {
	Convey("Given two rooms differing only in one advisor's credibility", t, func() {
		trusted := NewWarRoom()
		discredited := NewWarRoom()

		for range 20 {
			discredited.UpdateCredibility("momentum", true, MoveExplosivePump, MoveStagnant)
		}

		perspectives := []*types.Perspective{
			perspectiveFor("momentum", "Building", 0.9, 100),
		}

		trustedOutcome := trusted.Deliberate(perspectives, "TEST/USD", time.Unix(1, 0))
		discreditedOutcome := discredited.Deliberate(perspectives, "TEST/USD", time.Unix(1, 0))

		Convey("the discredited advisor moves the consensus less", func() {
			So(discreditedOutcome.Probabilities[MoveExplosivePump],
				ShouldBeLessThan, trustedOutcome.Probabilities[MoveExplosivePump])
		})
	})
}

func TestResidentCouncilSurvivesBetweenEnvelopes(t *testing.T) {
	Convey("Given advisors that spoke on an earlier envelope", t, func() {
		room := NewWarRoom()

		// Advisors are clocked to completed volume bars, so they speak on
		// trade envelopes.
		room.Deliberate([]*types.Perspective{
			perspectiveFor("pullback", "LiquiditySweep", 0.80, 100),
			perspectiveFor("liquidity", "WallBuilding", 0.85, 100),
		}, "TEST/USD", time.Unix(1, 0))

		Convey("a later ticker envelope with no perspectives still deliberates", func() {
			// This is the ticker frame the planner decides on: its
			// Perspectives slice is empty.
			outcome := room.Deliberate(nil, "TEST/USD", time.Unix(2, 0))

			So(outcome.Participants, ShouldEqual, 2)

			Convey("and the council's synergy still applies", func() {
				So(outcome.Synergies, ShouldNotBeEmpty)
				So(outcome.DominantMove, ShouldEqual, MoveExplosivePump)
			})
		})
	})
}

func TestResidentCouncilKeepsOneSeatPerAdvisor(t *testing.T) {
	Convey("Given an advisor that speaks repeatedly", t, func() {
		room := NewWarRoom()

		room.Deliberate([]*types.Perspective{
			perspectiveFor("momentum", "Building", 0.8, 100),
		}, "TEST/USD", time.Unix(1, 0))

		outcome := room.Deliberate([]*types.Perspective{
			perspectiveFor("momentum", "Stalling", 0.9, 100),
		}, "TEST/USD", time.Unix(2, 0))

		Convey("its latest reading replaces the previous one", func() {
			So(outcome.Participants, ShouldEqual, 1)
			So(outcome.Probabilities[MoveStagnant],
				ShouldBeGreaterThan, outcome.Probabilities[MoveExplosivePump])
		})
	})
}

func TestResidentCouncilIsPerSymbol(t *testing.T) {
	Convey("Given two symbols with different councils", t, func() {
		room := NewWarRoom()

		first := perspectiveFor("momentum", "Building", 0.9, 100)
		second := perspectiveFor("momentum", "Stalling", 0.9, 100)
		second.Symbol = "OTHER/USD"

		room.Deliberate([]*types.Perspective{first, second}, "TEST/USD", time.Unix(1, 0))

		Convey("each symbol deliberates over only its own council", func() {
			So(room.Deliberate(nil, "TEST/USD", time.Unix(2, 0)).Participants, ShouldEqual, 1)
			So(room.Deliberate(nil, "OTHER/USD", time.Unix(2, 0)).Participants, ShouldEqual, 1)
		})
	})
}

func TestUnprovenAdvisorIsDiscountedNotMuted(t *testing.T) {
	Convey("Given an advisor whose perspective has no survival record", t, func() {
		room := NewWarRoom()

		// Support of 1 reports zero maturity.
		fresh := room.Deliberate([]*types.Perspective{
			perspectiveFor("momentum", "Building", 0.9, 1),
		}, "TEST/USD", time.Unix(1, 0))

		Convey("it still moves the consensus off the stagnant prior", func() {
			So(fresh.Probabilities[MoveExplosivePump],
				ShouldBeGreaterThan, priorMass()[MoveExplosivePump]/1.0)
			So(fresh.DominantMove, ShouldNotEqual, MoveStagnant)
		})

		Convey("but it carries less weight than a proven advisor", func() {
			proven := NewWarRoom().Deliberate([]*types.Perspective{
				perspectiveFor("momentum", "Building", 0.9, 100),
			}, "TEST/USD", time.Unix(1, 0))

			So(fresh.Probabilities[MoveExplosivePump],
				ShouldBeLessThan, proven.Probabilities[MoveExplosivePump])
		})
	})
}

func TestResidentCouncilEvictionAndStaleness(t *testing.T) {
	Convey("Given an admitted perspective with a declared lease", t, func() {
		room := NewWarRoom()

		perspective := perspectiveFor("momentum", "Building", 0.9, 100)
		perspective.Lease = types.PerspectiveLease{
			Clock: "pumpdump/completed_volume_bar_ordinal",
			From:  10,
			Until: 11,
		}

		outcome := room.Deliberate([]*types.Perspective{perspective}, "TEST/USD", time.Unix(1, 0))
		So(outcome.Participants, ShouldEqual, 1)

		Convey("explicit eviction removes the resident seat", func() {
			room.Evict("TEST/USD", "momentum")
			emptyOutcome := room.Deliberate(nil, "TEST/USD", time.Unix(2, 0))
			So(emptyOutcome.Participants, ShouldEqual, 0)
			So(emptyOutcome.DominantMove, ShouldEqual, MoveStagnant)
		})

		Convey("clock coordinate advance past Until expires the lease", func() {
			freshPerspective := perspectiveFor("auction", "LiquiditySweep", 0.8, 50)
			freshPerspective.Lease = types.PerspectiveLease{
				Clock: "pumpdump/completed_volume_bar_ordinal",
				From:  12,
				Until: 13,
			}

			nextOutcome := room.Deliberate([]*types.Perspective{freshPerspective}, "TEST/USD", time.Unix(3, 0))
			So(nextOutcome.Participants, ShouldEqual, 1)
			So(room.Admit(nil, "TEST/USD")["momentum"], ShouldBeNil)
			So(room.Admit(nil, "TEST/USD")["auction"], ShouldNotBeNil)
		})
	})
}

func TestMoveForReturn(t *testing.T) {
	Convey("Given fractional returns across market regimes", t, func() {
		So(MoveForReturn(0.025), ShouldEqual, MoveExplosivePump)
		So(MoveForReturn(0.010), ShouldEqual, MoveSteadyTrend)
		So(MoveForReturn(0.003), ShouldEqual, MoveWeakDrift)
		So(MoveForReturn(0.000), ShouldEqual, MoveStagnant)
		So(MoveForReturn(-0.0005), ShouldEqual, MoveStagnant)
		So(MoveForReturn(-0.003), ShouldEqual, MoveWeakBleed)
		So(MoveForReturn(-0.010), ShouldEqual, MoveStructuralPullback)
		So(MoveForReturn(-0.035), ShouldEqual, MoveFlashDump)
	})
}

/*
TestProjectClassCoversEveryAdvisorClass holds the class vocabulary closed.

projectClass is the only place an advisor's opinion becomes consensus mass. A
label with no rule there contributes nothing while the advisor still counts as
a participant, so the council deliberates on less evidence than it was handed
and nothing in the output says so. This test enumerates every Class an Advisor
can actually construct and asserts each one projects.
*/
func TestProjectClassCoversEveryAdvisorClass(t *testing.T) {
	Convey("Given every class the advisors can emit", t, func() {
		labels := map[string]bool{}

		for _, feature := range NewMomentum().Features {
			labels[feature.Class.Label] = true
		}
		for _, feature := range NewAuction().Features {
			labels[feature.Class.Label] = true
		}
		for _, feature := range NewParticipation().Features {
			labels[feature.Class.Label] = true
		}
		for _, feature := range NewPullback().Features {
			labels[feature.Class.Label] = true
		}
		for _, feature := range NewProfitRun().Features {
			labels[feature.Class.Label] = true
		}
		for _, feature := range NewLiquidity().Features {
			labels[feature.Class.Label] = true
		}
		for _, feature := range NewBasis().Features {
			labels[feature.Class.Label] = true
		}

		So(labels, ShouldNotBeEmpty)

		Convey("every one projects onto the move space", func() {
			for label := range labels {
				mass := map[MarketMove]float64{}

				So(projectClass(mass, label, 1.0), ShouldBeTrue)
				So(len(mass), ShouldBeGreaterThan, 0)
			}
		})

		Convey("every one also states the move it argued for", func() {
			// MoveForState defaults to Stagnant, so it cannot report an
			// unmapped label; assert instead that the two agree on direction
			// wherever projectClass expresses a clear one.
			for label := range labels {
				mass := map[MarketMove]float64{}
				projectClass(mass, label, 1.0)

				heaviest := MoveStagnant
				best := 0.0

				for move, weight := range mass {
					if weight > best {
						best, heaviest = weight, move
					}
				}

				stated := MoveForState(label)

				So(
					(stated > MoveStagnant) == (heaviest > MoveStagnant) ||
						stated == MoveStagnant || heaviest == MoveStagnant,
					ShouldBeTrue,
				)
			}
		})
	})
}

/*
TestSilenceExplainsItself pins the distinction the War Room could not previously
report: an advisor whose declared evidence never arrived, versus one that spoke
and whose reading the clock has since passed.

Both rendered as "awaiting bar", so four advisors sat mute for the life of the
process on two misspelled metric names and nothing in the system said so. The
two states have different causes and different fixes, and a round that cannot
name which one applies cannot surface either.
*/
func TestSilenceExplainsItself(t *testing.T) {
	Convey("Given a council where no advisor has ever published", t, func() {
		room := NewWarRoom()

		outcome := room.Deliberate(nil, "TEST/USD", time.Now())

		Convey("every seat is reported, not just the ones that spoke", func() {
			So(len(outcome.Silent), ShouldEqual, len(KnownAdvisors))
		})

		Convey("each is reported as having no evidence", func() {
			for _, silence := range outcome.Silent {
				So(silence.Reason, ShouldEqual, "incomplete")
			}
		})
	})

	Convey("Given an advisor that reported which metrics it lacks", t, func() {
		room := NewWarRoom()

		room.Note("TEST/USD", []types.AdvisorSilence{{
			Advisor:  MomentumName,
			Reason:   "incomplete",
			Missing:  []string{"pumpdump/notional_rate_velocity"},
			Declared: 29,
		}})

		outcome := room.Deliberate(nil, "TEST/USD", time.Now())

		Convey("the round names the blocking metric", func() {
			var momentum *types.AdvisorSilence

			for index := range outcome.Silent {
				if outcome.Silent[index].Advisor == MomentumName {
					momentum = &outcome.Silent[index]
				}
			}

			So(momentum, ShouldNotBeNil)
			So(momentum.Missing, ShouldResemble, []string{"pumpdump/notional_rate_velocity"})
			So(momentum.Declared, ShouldEqual, 29)
		})
	})
}
