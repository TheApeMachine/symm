package strategy_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
priced answers whether a decision carries the whole friction breakdown, which is
what every utility assertion below is recomputed from.

A decision that could not be priced records the skip instead, and asserting a
utility bound against absent frictions would be asserting against zero.
*/
func priced(decision types.Decision) bool {
	return decision.ReferencePrice != nil &&
		decision.ReferencePrice.Sign() > 0 &&
		decision.ExpectedReturn != nil &&
		decision.ExpectedFees != nil &&
		decision.ExpectedSpread != nil &&
		decision.ExpectedImpact != nil
}

/*
executableFraction recomputes the candidate's executable edge from the decision's
own recorded frictions, as a fraction of the price it was judged against.
*/
func executableFraction(decision types.Decision) float64 {
	executable := decision.ExpectedReturn.
		Sub(decision.ExpectedFees).
		Sub(decision.ExpectedSpread).
		Sub(decision.ExpectedImpact)

	return executable.Div(decision.ReferencePrice).Float64()
}

/*
grossFraction is the same for a continuation, which is scored on the gross
forecast because the exit cost it is compared against is subtracted separately.
*/
func grossFraction(decision types.Decision) float64 {
	return decision.ExpectedReturn.Div(decision.ReferencePrice).Float64()
}

/*
utilityBounds is the exact interval the unified decision function can land in
for a given edge.

Every corroborating head is a bounded multiplier — causal on (0, 2), cognition
on (0, 1], graph on [0, 1] — so the corroborated edge lands within twice itself,
and the uncertainty charge then subtracts at most the edge again.

The interval is not symmetric, because the charge is a subtraction rather than a
shrink toward zero. A positive edge is pushed up to twice itself by the heads and
down to the negative of itself by the charge. A negative edge has both acting the
same way, so it runs to three times itself and can never come out positive: no
amount of corroboration argues for a trade whose own forecast does not.
*/
func utilityBounds(edge float64) (low, high float64) {
	const tolerance = 1e-12

	if edge >= 0 {
		return -tolerance, 2*edge + tolerance
	}

	return 2*edge - tolerance, tolerance
}

func positiveAmount(amount *decimal.Decimal) bool {
	return amount != nil && amount.Sign() > 0
}

/*
TestStrategyPipelineOnPump drives the whole stack through a pump and audits every
decision the planner published.

The properties under test are structural rather than statistical, so they are
asserted over every decision rather than sampled: utilities are return fractions,
entries require positive executable edge and funding before slots are contested,
and open inventory is never liquidated by strategy.
*/
func TestStrategyPipelineOnPump(t *testing.T) {
	/*
		SIM1 and SIM2 share a seed and differ only in price, so they are the
		same market quoted a hundred times apart. Any quantity that survives
		that difference is dimensionless; any that scales with it is carrying
		quote currency it should not be.
	*/
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 1.0, 42),
		testtypes.NewSymbol("SIM3/USD", 100.0, 1337),
	}

	Convey("Given a market that pumps through the full decision pipeline", t, tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
		market.WithAutoFill()

		peakOpenPositions := 0

		for range 64 {
			market.Tick()

			peakOpenPositions = max(peakOpenPositions, market.Desk.OpenPositions())
		}

		for _, symbol := range market.Symbols {
			So(market.Transition(symbol.Pair, testtypes.FastPump), ShouldBeNil)
		}

		for range 256 {
			market.Tick()

			peakOpenPositions = max(peakOpenPositions, market.Desk.OpenPositions())
		}

		decisions := market.Decisions()

		/*
			The reason each decision landed where it did, counted over the run.
			A suite that only reports which assertion failed says a trade never
			happened; this says which gate stopped it, which is the part worth
			knowing when the answer is that nothing ever traded.
		*/
		causes := map[string]int{}

		for _, decision := range decisions {
			causes[string(decision.Action)+"/"+decision.Cause]++
		}

		fmt.Printf("decision causes over %d ticks: %v\n", market.Thesis.Tick, causes)

		/*
			What each term of the utility contributed, so a run that decides
			nothing says which side of the comparison was responsible rather
			than only that the comparison failed.
		*/
		var (
			bestReturn   = math.Inf(-1)
			bestExec     = math.Inf(-1)
			worstExec    = math.Inf(1)
			totalFee     float64
			totalSpread  float64
			totalImpact  float64
			totalSurpris float64
			samples      int
		)

		for _, decision := range decisions {
			if !priced(decision) {
				continue
			}

			reference := decision.ReferencePrice
			samples++
			bestReturn = math.Max(bestReturn, decision.ExpectedReturn.Div(reference).Float64())
			exec := executableFraction(decision)
			bestExec = math.Max(bestExec, exec)
			worstExec = math.Min(worstExec, exec)
			totalFee += decision.ExpectedFees.Div(reference).Float64()
			totalSpread += decision.ExpectedSpread.Div(reference).Float64()
			totalImpact += decision.ExpectedImpact.Div(reference).Float64()
			totalSurpris += decision.Uncertainty
		}

		if samples > 0 {
			fmt.Printf(
				"fractions over %d priced: bestReturn=%.6f bestExec=%.6f worstExec=%.6f "+
					"meanFee=%.6f meanSpread=%.6f meanImpact=%.6f meanSurprise=%.4f\n",
				samples, bestReturn, bestExec, worstExec,
				totalFee/float64(samples),
				totalSpread/float64(samples),
				totalImpact/float64(samples),
				totalSurpris/float64(samples),
			)
		}

		Convey("The simulated market should have driven the whole stack", func() {
			/*
				Tick is incremented at the top of Planner.Update, before any
				gate, so it counts the times the planner was actually handed a
				thesis by the analyzer. That is well short of the number of
				market ticks fed in: the analyzer only passes a thesis on once
				every signal has measured it, so most simulated ticks are spent
				refilling evidence that the previous evaluation consumed.

				What matters is that the run evaluated enough ticks to be a test
				of anything, which the assertions below then measure.
			*/
			So(market.Thesis.Tick, ShouldBeGreaterThan, 20)
			So(market.Measurements(), ShouldHaveLength, 10)

			for source, rows := range market.Measurements() {
				So(source, ShouldNotBeBlank)
				So(rows, ShouldNotBeEmpty)
			}
		})

		/*
			Readiness is deliberately not asserted after the run. An evaluated
			tick ends by resetting the thesis, so what a finished run leaves
			behind is whichever tick was mid-refill when the feed stopped, not a
			statement about the pipeline. The decisions below are the observable
			that a gate opened, and they survive the reset because the planner
			publishes them to its subscribers first.
		*/
		Convey("The planner should publish decisions at all", func() {
			// Everything below is vacuous if the gate never opened, so this is
			// asserted first and on its own.
			So(decisions, ShouldNotBeEmpty)

			evaluated := 0

			for _, decision := range decisions {
				if decision.ArbitrationRound > 0 {
					evaluated++
				}
			}

			So(evaluated, ShouldEqual, len(decisions))
		})

		Convey("Every decision should be identifiable and account for itself", func() {
			for _, decision := range decisions {
				So(decision.ValidID(), ShouldBeTrue)
				So(decision.Symbol, ShouldNotBeBlank)
				So(decision.Cause, ShouldNotBeBlank)
				So(decision.Reason, ShouldNotBeBlank)
				So(math.IsNaN(decision.Utility), ShouldBeFalse)
				So(math.IsInf(decision.Utility, 0), ShouldBeFalse)
			}
		})

		Convey("Every utility should be a return fraction rather than an amount of currency", func() {
			pricedEntries := 0
			pricedContinuations := 0

			for _, decision := range decisions {
				/*
					A per-tick forecast net of friction is a small fraction of
					the price. A whole unit of utility is a forecast of doubling
					the position within one tick, which is not a reading this
					pipeline can produce — it is quote currency wearing a
					fraction's name.
				*/
				So(math.Abs(decision.Utility), ShouldBeLessThan, 1.0)

				if !priced(decision) {
					continue
				}

				switch decision.Action {
				case types.ActionEnter, types.ActionNothing:
					pricedEntries++

					low, high := utilityBounds(executableFraction(decision))

					So(decision.Utility, ShouldBeGreaterThanOrEqualTo, low)
					So(decision.Utility, ShouldBeLessThanOrEqualTo, high)
				case types.ActionHold:
					pricedContinuations++

					low, high := utilityBounds(grossFraction(decision))

					So(decision.Utility, ShouldBeGreaterThanOrEqualTo, low)
					So(decision.Utility, ShouldBeLessThanOrEqualTo, high)
				}
			}

			// Priced rejections exercise the same entry valuation as accepted
			// entries. Continuation exists only if that valuation admitted a lot.
			So(pricedEntries, ShouldBeGreaterThan, 0)

			if peakOpenPositions == 0 {
				So(pricedContinuations, ShouldEqual, 0)
				return
			}

			So(pricedContinuations, ShouldBeGreaterThan, 0)
		})

		Convey("The same market quoted a hundred times apart should score the same", func() {
			magnitude := func(symbol string) (float64, int) {
				total := 0.0
				count := 0

				for _, decision := range decisions {
					if decision.Symbol != symbol || !priced(decision) {
						continue
					}

					total += math.Abs(decision.Utility)
					count++
				}

				if count == 0 {
					return 0, 0
				}

				return total / float64(count), count
			}

			expensive, expensiveCount := magnitude("SIM1/USD")
			cheap, cheapCount := magnitude("SIM2/USD")

			So(expensiveCount, ShouldBeGreaterThan, 0)
			So(cheapCount, ShouldBeGreaterThan, 0)

			/*
				The two symbols are a hundred apart in price. A utility carrying
				quote currency separates by that factor; a fraction does not
				separate by it at all, and what remains is only the difference
				between two independently evolving books.
			*/
			So(expensive, ShouldBeLessThan, 0.05)
			So(cheap, ShouldBeLessThan, 0.05)
			So(math.Max(expensive, cheap), ShouldBeLessThan, 10*math.Min(expensive, cheap))
		})

		Convey("Every entry should be funded before it is allowed to take a slot", func() {
			entries := 0

			for _, decision := range decisions {
				if decision.Action != types.ActionEnter {
					continue
				}

				entries++

				// Allocation runs ahead of arbitration, so an entry that
				// survives to hold a slot has already been sized against the
				// wallet and quantised to the venue's rules.
				So(positiveAmount(decision.ProposedQuantity), ShouldBeTrue)
				So(positiveAmount(decision.ProposedNotional), ShouldBeTrue)
				So(positiveAmount(decision.ReferencePrice), ShouldBeTrue)
			}

			if bestExec <= 0 {
				So(entries, ShouldEqual, 0)
			}
		})

		Convey("Strategy should never liquidate open inventory", func() {
			exits := 0
			rotations := 0

			for _, decision := range decisions {
				if decision.Action == types.ActionExit {
					exits++
				}

				if decision.Cause == "rotation" {
					rotations++
				}
			}

			So(exits, ShouldEqual, 0)
			So(rotations, ShouldEqual, 0)
		})

		Convey("Continuation should report valuation without taking exit authority", func() {
			continuations := 0

			for _, decision := range decisions {
				if decision.Cause != "continuation" {
					continue
				}

				hold, hasHold := decision.Alternatives["hold"]

				if !hasHold {
					continue
				}

				continuations++

				exit, hasExit := decision.Alternatives["exit"]

				if !hasExit {
					continue
				}

				// Closing costs something, and that something is one crossing
				// stated as a fraction of the price rather than an amount of it.
				So(exit, ShouldBeLessThanOrEqualTo, 0.0)
				So(exit, ShouldBeGreaterThan, -1.0)
				So(decision.Action, ShouldEqual, types.ActionHold)
				So(decision.Utility, ShouldEqual, hold)
			}

			if peakOpenPositions == 0 {
				So(continuations, ShouldEqual, 0)
				return
			}

			So(continuations, ShouldBeGreaterThan, 0)
		})

		Convey("The desk should never carry a position without executable edge", func() {
			// A named pump regime is evidence, not profit. If the forecast never
			// clears its own friction, opening a position would recreate the paper
			// run's churn under a different decision label.
			So(peakOpenPositions == 0 || bestExec > 0, ShouldBeTrue)
		})
	}))
}
