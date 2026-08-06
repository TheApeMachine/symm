package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
snapshot builds the regulator geometry a lot would be scored against: entered at
100, hard floor 36 cents below, profit floor 87 cents above, and protection
arming 89 cents above.
*/
func snapshot(mark float64, armed bool) types.StopSnapshot {
	excursion := (mark - 100) / 0.36
	maxAdverse := min(0, excursion)
	maxFavorable := max(0, excursion)

	return types.StopSnapshot{
		Present:      true,
		Symbol:       "SIM1/USD",
		Phase:        types.PhaseDiscovery,
		ProfitArmed:  armed,
		Entry:        decimal.NewFromFloat64(100),
		Mark:         decimal.NewFromFloat64(mark),
		HardFloor:    decimal.NewFromFloat64(99.64),
		ProfitLine:   decimal.NewFromFloat64(100.65),
		ProfitFloor:  decimal.NewFromFloat64(100.87),
		ArmLine:      decimal.NewFromFloat64(100.89),
		RiskDistance: decimal.NewFromFloat64(0.36),
		NoiseBand:    decimal.NewFromFloat64(0.12),
		MaxAdverse:   decimal.NewFromFloat64(maxAdverse),
		MaxFavorable: decimal.NewFromFloat64(maxFavorable),
	}
}

func TestPassageRoom(t *testing.T) {
	Convey("Given a lot part-way between its boundaries", t, func() {
		upside, downside, ok := passageRoom(snapshot(100.00, false))

		So(ok, ShouldBeTrue)

		Convey("Both directions should be priced as fractions of the live mark", func() {
			So(upside, ShouldAlmostEqual, 0.0089, 1e-4)
			So(downside, ShouldAlmostEqual, 0.0036, 1e-4)
		})

		Convey("A mark already through a boundary should have no room left there", func() {
			through, _, _ := passageRoom(snapshot(101.00, false))
			So(through, ShouldEqual, 0)

			_, below, _ := passageRoom(snapshot(99.00, false))
			So(below, ShouldEqual, 0)
		})

		Convey("Missing geometry should price nothing", func() {
			bare := snapshot(100, false)
			bare.HardFloor = nil

			_, _, priced := passageRoom(bare)
			So(priced, ShouldBeFalse)
		})
	})
}

func TestPassageFeatures(t *testing.T) {
	Convey("Given an open lot's episode", t, func() {
		episode := &passageEpisode{
			horizon:    10,
			openedTick: 100,
			entryNoise: decimal.NewFromFloat64(0.12),
		}

		Convey("Drawdown should be stated in risk distances, not currency", func() {
			features, ok := passageFeatures(
				snapshot(99.82, false), candidate{}, episode, "neutral", 105,
			)

			So(ok, ShouldBeTrue)

			// Half a risk distance underwater, halfway through the horizon.
			So(features.Drawdown, ShouldAlmostEqual, -0.5, 1e-6)
			So(features.Age, ShouldAlmostEqual, 0.5, 1e-6)
			So(features.Regime, ShouldEqual, "neutral")
		})

		Convey("A widened book should show as liquidity above one", func() {
			widened := snapshot(100, false)
			widened.NoiseBand = decimal.NewFromFloat64(0.24)

			features, ok := passageFeatures(widened, candidate{}, episode, "neutral", 100)

			So(ok, ShouldBeTrue)
			So(features.Liquidity, ShouldAlmostEqual, 2, 1e-6)
		})

		Convey("A lot with no derived risk distance should not be scored", func() {
			bare := snapshot(100, false)
			bare.RiskDistance = nil

			_, ok := passageFeatures(bare, candidate{}, episode, "neutral", 100)
			So(ok, ShouldBeFalse)
		})
	})
}

func TestPassageEpisodeLabelling(t *testing.T) {
	Convey("Given a finished episode", t, func() {
		Convey("A lot that ever armed reached its profit line first", func() {
			episode := &passageEpisode{armed: true, lastTrigger: types.TriggerHardRisk}
			outcome, labelled := episode.outcome()

			/*
				Arming is sticky on purpose. A lot that cleared its profit line
				and later gave it back still answered the question being
				predicted with "profit first", and reading its eventual exit as
				a loss would teach the model that winners are losers.
			*/
			So(labelled, ShouldBeTrue)
			So(outcome, ShouldEqual, types.OutcomeProfitFirst)
		})

		Convey("A lot stopped at the hard floor reached loss first", func() {
			episode := &passageEpisode{lastTrigger: types.TriggerHardRisk}
			outcome, labelled := episode.outcome()

			So(labelled, ShouldBeTrue)
			So(outcome, ShouldEqual, types.OutcomeLossFirst)
		})

		Convey("A lot whose horizon expired reached neither", func() {
			episode := &passageEpisode{lastAge: 1.4}
			outcome, labelled := episode.outcome()

			So(labelled, ShouldBeTrue)
			So(outcome, ShouldEqual, types.OutcomeTimeout)
		})

		Convey("A lot closed early for another reason should be censored", func() {
			/*
				This is the bias the model would otherwise learn. A position
				rotated out at half its horizon never had its patience tested,
				and counting it as a timeout would say waiting was safe using
				exactly the episode where waiting did not happen.
			*/
			episode := &passageEpisode{lastAge: 0.4}
			_, labelled := episode.outcome()

			So(labelled, ShouldBeFalse)
		})
	})
}

func TestPassageLearnsFromFinishedLots(t *testing.T) {
	Convey("Given an evaluator tracking a lot that has been observed for a while", t, func() {
		evaluator := Evaluator{
			passage:  types.NewPassageModel(),
			episodes: map[string]*passageEpisode{},
		}

		episode := &passageEpisode{
			symbol:     "SIM1/USD",
			horizon:    10,
			openedTick: 100,
			entryNoise: decimal.NewFromFloat64(0.12),
		}

		evaluator.episodes["lot-1"] = episode

		for tick := range 5 {
			features, ok := passageFeatures(
				snapshot(99.90, false), candidate{}, episode, "neutral",
				int64(100+tick),
			)

			So(ok, ShouldBeTrue)
			episode.observe(features, snapshot(99.90, false), int64(100+tick))
		}

		So(evaluator.passage.Total(), ShouldEqual, 0)

		Convey("Retiring it at the hard floor should fold every state it passed through", func() {
			evaluator.retire("lot-1", types.TriggerHardRisk)

			/*
				Every state along the way carries the same label, but the result
				belongs to one finished position. Repeated ticks from that
				position must not manufacture five independent outcomes.
			*/
			So(evaluator.passage.Total(), ShouldEqual, 1)
			So(evaluator.episodes, ShouldNotContainKey, "lot-1")

			Convey("And the model should have learned that state", func() {
				scenario := evaluator.passage.Scenario(types.PassageFeatures{
					Drawdown: -0.277, Age: 0.2, Forecast: 0, Liquidity: 1,
					Regime: "neutral",
				})

				So(scenario.LossFirst, ShouldBeGreaterThan, 1.0/3)
			})
		})

		Convey("Retiring it for an unrelated reason should teach nothing", func() {
			evaluator.retire("lot-1", "")

			// Censored: the horizon had not expired and no boundary was
			// reached, so this lot's patience was never actually tested.
			So(evaluator.passage.Total(), ShouldEqual, 0)
			So(evaluator.episodes, ShouldNotContainKey, "lot-1")
		})

		Convey("Retiring an unknown lot should be harmless", func() {
			evaluator.retire("lot-missing", types.TriggerHardRisk)
			So(evaluator.passage.Total(), ShouldEqual, 0)
		})
	})
}

func TestPassageEpisodeRecord(t *testing.T) {
	Convey("Given a lot that swung both ways before it finished", t, func() {
		episode := &passageEpisode{
			symbol:     "SIM1/USD",
			horizon:    10,
			openedTick: 100,
			entryNoise: decimal.NewFromFloat64(0.12),
		}

		for index, mark := range []float64{99.90, 99.70, 100.30, 99.82} {
			features, ok := passageFeatures(
				snapshot(mark, false), candidate{}, episode, "neutral",
				int64(100+index),
			)

			So(ok, ShouldBeTrue)
			episode.observe(features, snapshot(mark, false), int64(100+index))
		}

		record := episode.record("lot-1", types.OutcomeLossFirst, true)

		Convey("The record should carry the excursions, not just the last state", func() {
			/*
				These two numbers are the corpus the adverse-excursion quantile
				replaces the configured multiples from. "How far do trades like
				this go against me before they work" cannot be answered from a
				closing price, only from the extremes along the way.
			*/
			So(record.MaxAdverse, ShouldBeLessThan, -0.8)
			So(record.MaxFavorable, ShouldBeGreaterThan, 0.8)
			So(record.Observations, ShouldHaveLength, 4)
		})

		Convey("It should carry both boundaries and the outcome", func() {
			So(record.PositionID, ShouldEqual, "lot-1")
			So(record.Symbol, ShouldEqual, "SIM1/USD")
			So(record.Entry, ShouldAlmostEqual, 100.0, 1e-6)
			So(record.HardFloor, ShouldAlmostEqual, 99.64, 1e-6)
			So(record.ProfitLine, ShouldAlmostEqual, 100.65, 1e-6)
			So(record.ArmLine, ShouldAlmostEqual, 100.89, 1e-6)
			So(record.Outcome, ShouldEqual, types.OutcomeLossFirst)
			So(record.Censored, ShouldBeFalse)
			So(record.ClosedTick, ShouldEqual, 103)
		})

		Convey("A censored episode should say so rather than be dropped", func() {
			/*
				An offline fit has to know which lots were closed before their
				patience was tested. Writing only the labelled ones would leave
				the corpus looking like every trade ran to a boundary.
			*/
			censored := episode.record("lot-1", "", false)

			So(censored.Censored, ShouldBeTrue)
			So(censored.Observations, ShouldNotBeEmpty)
		})
	})
}

func TestPassageIsOneDirectional(t *testing.T) {
	Convey("Given the room a lot has left in each direction", t, func() {
		upside, downside, ok := passageRoom(snapshot(100.00, false))
		So(ok, ShouldBeTrue)

		Convey("An unready scenario must not be able to close anything", func() {
			/*
				The caller only acts when the scenario reports Ready, so a model
				with nothing to say leaves the existing continuation rule
				exactly as it was. This is asserted rather than assumed because
				the whole safety argument rests on it: the model may add
				caution, never patience.
			*/
			unready := types.PassageScenario{
				ProfitFirst: 0.1, LossFirst: 0.8, Timeout: 0.1, Ready: false,
			}

			So(unready.HoldEV(upside, downside, 0.002), ShouldBeLessThan, 0)
			So(unready.Ready, ShouldBeFalse)
		})

		Convey("A confident winner should still not veto an exit the other rules made", func() {
			/*
				HoldEV is positive here, and the caller only ever consults it
				when the position is otherwise being held. Nothing in the
				scenario can reach the hard floor or reverse a decayed
				continuation.
			*/
			confident := types.PassageScenario{
				ProfitFirst: 0.9, LossFirst: 0.05, Timeout: 0.05, Ready: true,
			}

			So(confident.HoldEV(upside, downside, 0.002), ShouldBeGreaterThan, 0)
		})
	})
}
