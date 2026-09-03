package types_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
state is one point in the feature space, written out so each test says which
dimension it is varying.
*/
func state(drawdown, age float64, regime string) types.PassageFeatures {
	return types.PassageFeatures{
		Drawdown:  drawdown,
		Age:       age,
		Forecast:  0.01,
		Liquidity: 1,
		Regime:    regime,
	}
}

func feed(
	model *types.PassageModel,
	features types.PassageFeatures,
	outcome types.PassageOutcome,
	count int,
) {
	for range count {
		model.Observe(features, outcome)
	}
}

func TestPassageModelPrior(t *testing.T) {
	Convey("Given a model that has seen nothing", t, func() {
		model := types.NewPassageModel()
		scenario := model.Scenario(state(-0.5, 0.3, "neutral"))

		Convey("It should answer with the uniform prior and refuse to be acted on", func() {
			So(scenario.ProfitFirst, ShouldAlmostEqual, 1.0/3, 1e-9)
			So(scenario.LossFirst, ShouldAlmostEqual, 1.0/3, 1e-9)
			So(scenario.Timeout, ShouldAlmostEqual, 1.0/3, 1e-9)
			So(scenario.Ready, ShouldBeFalse)
			So(scenario.Support, ShouldEqual, 0)
		})

		Convey("The outcomes should always be a distribution", func() {
			So(scenario.ProfitFirst+scenario.LossFirst+scenario.Timeout,
				ShouldAlmostEqual, 1.0, 1e-9)
		})
	})
}

func TestPassageModelSparseEvidence(t *testing.T) {
	Convey("Given a bucket seen only twice, both times a loss", t, func() {
		model := types.NewPassageModel()
		features := state(-0.8, 0.5, "reversal")
		feed(model, features, types.OutcomeLossFirst, 2)

		scenario := model.Scenario(features)

		Convey("It should not claim certainty", func() {
			/*
				Two losses is not proof that recovery is impossible. A model that
				returned 0% here would let the caller close every comparable lot
				on the strength of two trades.
			*/
			So(scenario.ProfitFirst, ShouldBeGreaterThan, 0)
			So(scenario.LossFirst, ShouldBeLessThan, 1)
			So(scenario.ProfitFirst+scenario.LossFirst+scenario.Timeout,
				ShouldAlmostEqual, 1.0, 1e-9)
		})

		Convey("But it should have moved toward the evidence", func() {
			So(scenario.LossFirst, ShouldBeGreaterThan, 1.0/3)
		})
	})
}

func TestPassageModelObserveEpisode(t *testing.T) {
	Convey("Given one finished position observed repeatedly in the same state", t, func() {
		model := types.NewPassageModel()
		features := state(-0.8, 0.5, "reversal")
		observations := make([]types.PassageFeatures, 128)

		for index := range observations {
			observations[index] = features
		}

		model.ObserveEpisode(observations, types.OutcomeLossFirst)
		scenario := model.Scenario(features)

		Convey("It should count one episode rather than one result per tick", func() {
			So(model.Total(), ShouldEqual, 1)
			So(scenario.Support, ShouldEqual, 1)
			So(scenario.Ready, ShouldBeFalse)
			So(scenario.LossFirst, ShouldBeGreaterThan, 1.0/3)
		})

		Convey("Only independently finished positions should satisfy readiness", func() {
			for range 39 {
				model.Observe(features, types.OutcomeLossFirst)
			}

			scenario = model.Scenario(features)

			So(model.Total(), ShouldEqual, 40)
			So(scenario.Support, ShouldEqual, 40)
			So(scenario.Ready, ShouldBeTrue)
		})
	})
}

func TestPassageModelConverges(t *testing.T) {
	Convey("Given a bucket with a great deal of consistent evidence", t, func() {
		model := types.NewPassageModel()
		features := state(-0.8, 0.5, "reversal")
		feed(model, features, types.OutcomeLossFirst, 400)

		scenario := model.Scenario(features)

		Convey("It should speak for itself", func() {
			So(scenario.LossFirst, ShouldBeGreaterThan, 0.9)
			So(scenario.Ready, ShouldBeTrue)
			So(scenario.Support, ShouldBeGreaterThanOrEqualTo, 400)
		})

		Convey("And still never reach certainty", func() {
			/*
				The hierarchy shrinks toward its parent once per level, so
				unanimous evidence populating every level would otherwise drive
				an outcome to about one in ten million. Four hundred episodes
				support "less than one in four hundred" and nothing more.
			*/
			So(scenario.LossFirst, ShouldBeLessThan, 1)
			So(scenario.ProfitFirst, ShouldBeGreaterThan, 1.0/(scenario.Support*2))
		})
	})
}

func TestPassageModelRefusesUnseenStates(t *testing.T) {
	Convey("Given a model well fed on one kind of state", t, func() {
		model := types.NewPassageModel()
		feed(model, state(-0.9, 0.4, "reversal"), types.OutcomeLossFirst, 300)

		So(model.Total(), ShouldBeGreaterThan, 40)

		Convey("A state it has never seen should not be actionable", func() {
			/*
				This is the failure the local support gate exists for. Knowing a
				great deal about deep drawdowns in a reversal is knowing nothing
				about a lot in profit during momentum, and a global readiness
				gate would hand back the uniform prior marked ready — closing
				that position on a three-way coin flip.
			*/
			unseen := model.Scenario(state(0.7, 0.2, "momentum"))

			So(unseen.Support, ShouldEqual, 0)
			So(unseen.Ready, ShouldBeFalse)
			So(unseen.ProfitFirst, ShouldAlmostEqual, 1.0/3, 1e-9)
		})
	})
}

func TestPassageModelBacksOff(t *testing.T) {
	Convey("Given evidence in one regime but none in a specific state of it", t, func() {
		model := types.NewPassageModel()

		// Shallow drawdowns in a reversal regime keep losing.
		feed(model, state(-0.1, 0.1, "reversal"), types.OutcomeLossFirst, 200)

		Convey("An unseen state in that regime should inherit its shape", func() {
			unseen := model.Scenario(state(-0.1, 0.9, "reversal"))

			So(unseen.LossFirst, ShouldBeGreaterThan, 0.5)
			So(unseen.ProfitFirst, ShouldBeGreaterThan, 0)
		})

		Convey("A state in a different regime should not", func() {
			/*
				The hierarchy splits on drawdown first, so a different regime at
				a different drawdown shares only the coarsest level and must not
				inherit the reversal regime's verdict.
			*/
			elsewhere := model.Scenario(state(0.6, 0.9, "momentum"))

			So(elsewhere.LossFirst, ShouldBeLessThan, 0.5)
		})
	})
}

func TestPassageModelSeparatesStates(t *testing.T) {
	Convey("Given deep drawdowns that fail and shallow ones that recover", t, func() {
		model := types.NewPassageModel()
		deep := state(-0.9, 0.4, "neutral")
		shallow := state(-0.1, 0.4, "neutral")

		feed(model, deep, types.OutcomeLossFirst, 150)
		feed(model, shallow, types.OutcomeProfitFirst, 150)

		Convey("The model should tell them apart", func() {
			So(model.Scenario(deep).LossFirst, ShouldBeGreaterThan, 0.8)
			So(model.Scenario(shallow).ProfitFirst, ShouldBeGreaterThan, 0.8)
		})

		Convey("And both should be ready to act on", func() {
			So(model.Scenario(deep).Ready, ShouldBeTrue)
			So(model.Total(), ShouldEqual, 300)
		})
	})
}

func TestPassageScenarioHoldEV(t *testing.T) {
	Convey("Given a scenario and the room a lot has left", t, func() {
		Convey("A likely winner with room should be worth holding", func() {
			scenario := types.PassageScenario{
				ProfitFirst: 0.7, LossFirst: 0.2, Timeout: 0.1, Ready: true,
			}

			So(scenario.HoldEV(0.02, 0.01, 0.005), ShouldBeGreaterThan, 0)
		})

		Convey("A likely loser with little room should not be", func() {
			scenario := types.PassageScenario{
				ProfitFirst: 0.15, LossFirst: 0.75, Timeout: 0.10, Ready: true,
			}

			So(scenario.HoldEV(0.02, 0.01, 0.005), ShouldBeLessThan, 0)
		})

		Convey("Room matters as much as probability", func() {
			even := types.PassageScenario{
				ProfitFirst: 0.5, LossFirst: 0.5, Timeout: 0, Ready: true,
			}

			// The same coin flip is worth taking for a wide target and not for
			// a narrow one against the same downside.
			So(even.HoldEV(0.04, 0.01, 0), ShouldBeGreaterThan, 0)
			So(even.HoldEV(0.004, 0.01, 0), ShouldBeLessThan, 0)
		})
	})
}

func TestPassageModelFoldAdverseQuantile(t *testing.T) {
	Convey("Given a model folding finished winners and losers", t, func() {
		model := types.NewPassageModel()

		for index := range 10 {
			model.Fold(types.PassageEpisode{
				Outcome:    types.OutcomeProfitFirst,
				MaxAdverse: 0.2 + float64(index)*0.1,
			})
			model.Fold(types.PassageEpisode{
				Outcome:    types.OutcomeLossFirst,
				MaxAdverse: 5.0,
			})
			model.Fold(types.PassageEpisode{
				Outcome:    types.OutcomeProfitFirst,
				MaxAdverse: 9.0,
				Censored:   true,
			})
		}

		Convey("It should refuse to speak before support", func() {
			_, ready := model.AdverseQuantile(0.95)
			So(ready, ShouldBeFalse)
		})

		model.Fold(types.PassageEpisode{
			Outcome:    types.OutcomeProfitFirst,
			MaxAdverse: 0.5,
		})
		model.Fold(types.PassageEpisode{
			Outcome:    types.OutcomeProfitFirst,
			MaxAdverse: 0.8,
		})

		Convey("It should quantify only the uncensored winners", func() {
			excursion, ready := model.AdverseQuantile(0.5)
			So(ready, ShouldBeTrue)

			// Twelve winners retained: 0.2..1.1 plus 0.5 and 0.8. The
			// median of the sorted samples is 0.65.
			So(excursion, ShouldAlmostEqual, 0.65, 1e-12)

			lower, stillReady := model.AdverseQuantile(1.0 / 3.0)
			So(stillReady, ShouldBeTrue)
			So(lower, ShouldAlmostEqual, 0.5, 1e-12)
		})
	})
}

func TestPassageModelAdverseQuantileForRegime(t *testing.T) {
	Convey("Given a model folding winners from distinct macro regimes", t, func() {
		model := types.NewPassageModel()

		// Fold 12 winners for stable regime (low adverse excursion).
		for index := range 12 {
			model.Fold(types.PassageEpisode{
				Regime:     "stable",
				Outcome:    types.OutcomeProfitFirst,
				MaxAdverse: 0.1 + float64(index)*0.02,
			})
		}

		// Fold 12 winners for crash regime (high adverse excursion).
		for index := range 12 {
			model.Fold(types.PassageEpisode{
				Regime:     "crash",
				Outcome:    types.OutcomeProfitFirst,
				MaxAdverse: 1.5 + float64(index)*0.1,
			})
		}

		Convey("Stable regime adverse quantile is isolated from crash regime excursions", func() {
			stableExcursion, ready := model.AdverseQuantileForRegime("stable", 0.5)
			So(ready, ShouldBeTrue)
			So(stableExcursion, ShouldBeLessThan, 0.3)

			crashExcursion, ready := model.AdverseQuantileForRegime("crash", 0.5)
			So(ready, ShouldBeTrue)
			So(crashExcursion, ShouldBeGreaterThan, 1.8)

			// The global adverse quantile is a blend of both regimes.
			globalExcursion, ready := model.AdverseQuantile(0.5)
			So(ready, ShouldBeTrue)
			So(globalExcursion, ShouldBeGreaterThan, stableExcursion)
			So(globalExcursion, ShouldBeLessThan, crashExcursion)
		})

		Convey("Sparse or unseen regimes fall back to global quantile", func() {
			fallbackExcursion, ready := model.AdverseQuantileForRegime("unseen", 0.5)
			So(ready, ShouldBeTrue)

			globalExcursion, _ := model.AdverseQuantile(0.5)
			So(fallbackExcursion, ShouldEqual, globalExcursion)
		})
	})
}

func BenchmarkPassageModelObserveEpisode(b *testing.B) {
	model := types.NewPassageModel()
	observations := make([]types.PassageFeatures, 128)

	for index := range observations {
		observations[index] = state(
			-float64(index%8)/8,
			float64(index%4)/4,
			"reversal",
		)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		model.ObserveEpisode(observations, types.OutcomeLossFirst)
	}
}
