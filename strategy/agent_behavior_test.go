package strategy

import (
	"math"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

func TestBehavioralTraderLearningModel(t *testing.T) {
	Convey("Behavioral verification of the restored causal learning model", t, func() {
		// 1. Temporal order matters (A -> B -> C != C -> B -> A).
		Convey("1. Temporal order matters in precursor context", func() {
			model := NewEconomicModel(512)
			action := LearningAction{Kind: types.ActionEnter, Power: 10}
			key := [2]string{"BTC/USD", "flat"}

			tokenA, tokenB, tokenC := uint64(101), uint64(202), uint64(303)
			pathABC := []uint64{tokenA, FrameDelimiter, tokenB, FrameDelimiter, tokenC}
			pathCBA := []uint64{tokenC, FrameDelimiter, tokenB, FrameDelimiter, tokenA}

			err := model.Observe(key, pathABC, action, 0.05, 10.0, 1.0)
			So(err, ShouldBeNil)

			readingABC := model.Recall(key, pathABC, action)
			So(readingABC.Defined, ShouldBeTrue)
			So(readingABC.Samples, ShouldEqual, 1)
			So(readingABC.Rate, ShouldAlmostEqual, 0.005)

			readingCBA := model.Recall(key, pathCBA, action)
			So(readingCBA.Depth, ShouldEqual, 0)
			So(readingCBA.Samples, ShouldEqual, 1) // Only matches unconditioned root
		})

		// 2. Current snapshot is not whole precursor.
		Convey("2. Current snapshot alone does not identify whole precursor", func() {
			market1 := &learningMarket{symbol: "BTC/USD"}
			market2 := &learningMarket{symbol: "BTC/USD"}

			regionA := learning.Region{Condition: 101, Strength: 1.0, Authority: 1.0}
			regionB := learning.Region{Condition: 202, Strength: 1.0, Authority: 1.0}
			regionC := learning.Region{Condition: 303, Strength: 1.0, Authority: 1.0}

			// Market 1 evolved from B to A
			market1.AdvanceImpulse([]learning.Region{regionB})
			market1.AdvanceImpulse([]learning.Region{regionA})

			// Market 2 evolved from C to A
			market2.AdvanceImpulse([]learning.Region{regionC})
			market2.AdvanceImpulse([]learning.Region{regionA})

			ctx1 := market1.PrecursorContext()
			ctx2 := market2.PrecursorContext()

			So(ctx1, ShouldResemble, []uint64{101, FrameDelimiter, 202})
			So(ctx2, ShouldResemble, []uint64{101, FrameDelimiter, 303})
			So(ctx1, ShouldNotResemble, ctx2)
		})

		// 3. Within-state order survives ([A strongest, B second] != [B strongest, A second]).
		Convey("3. Within-state order survives without permutation collapse", func() {
			model := NewEconomicModel(512)
			action := LearningAction{Kind: types.ActionEnter}
			key := [2]string{"ETH/USD", "flat"}

			orderAB := []uint64{101, 202}
			orderBA := []uint64{202, 101}

			err := model.Observe(key, orderAB, action, 0.08, 4.0, 1.0)
			So(err, ShouldBeNil)

			readingAB := model.Recall(key, orderAB, action)
			So(readingAB.Depth, ShouldEqual, 2)
			So(readingAB.Rate, ShouldAlmostEqual, 0.02)

			readingBA := model.Recall(key, orderBA, action)
			So(readingBA.Depth, ShouldEqual, 0)
		})

		// 4. Orientation survives (ConditionToken distinguishes level & change direction).
		Convey("4. Orientation survives in ConditionToken", func() {
			tokenBullish := learning.ConditionToken(10, 1.5, 0.5)
			tokenBearish := learning.ConditionToken(10, -1.5, -0.5)
			tokenMixed := learning.ConditionToken(10, 1.5, -0.5)

			So(tokenBullish, ShouldNotEqual, tokenBearish)
			So(tokenBullish, ShouldNotEqual, tokenMixed)
			So(tokenBearish, ShouldNotEqual, tokenMixed)

			model := NewEconomicModel(512)
			action := LearningAction{Kind: types.ActionEnter}
			key := [2]string{"SOL/USD", "flat"}

			So(model.Observe(key, []uint64{tokenBullish}, action, 0.10, 2.0, 1.0), ShouldBeNil)
			readingBull := model.Recall(key, []uint64{tokenBullish}, action)
			readingBear := model.Recall(key, []uint64{tokenBearish}, action)

			So(readingBull.Depth, ShouldEqual, 1)
			So(readingBear.Depth, ShouldEqual, 0)
		})

		// 5. Enter and Wait learn separately under (precursor, flat).
		Convey("5. Enter and Wait learn separately under (precursor, flat)", func() {
			model := NewEconomicModel(512)
			key := [2]string{"BTC/USD", "flat"}
			context := []uint64{100, FrameDelimiter, 200}

			enterAction := LearningAction{Kind: types.ActionEnter, Power: 50}
			waitAction := LearningAction{Kind: types.ActionHold}

			So(model.Observe(key, context, enterAction, 0.05, 5.0, 1.0), ShouldBeNil)

			readingEnter := model.Recall(key, context, enterAction)
			readingWait := model.Recall(key, context, waitAction)

			So(readingEnter.Defined, ShouldBeTrue)
			So(readingEnter.Samples, ShouldEqual, 1)
			So(readingEnter.Rate, ShouldAlmostEqual, 0.01)

			So(readingWait.Defined, ShouldBeFalse)
			So(readingWait.Samples, ShouldEqual, 0)
		})

		// 6. Hold and Exit learn separately under (precursor, holding).
		Convey("6. Hold and Exit learn separately under (precursor, holding)", func() {
			model := NewEconomicModel(512)
			keyHolding := [2]string{"BTC/USD", "holding"}
			keyFlat := [2]string{"BTC/USD", "flat"}
			context := []uint64{100}

			holdAction := LearningAction{Kind: types.ActionHold}
			exitAction := LearningAction{Kind: types.ActionExit}

			So(model.Observe(keyHolding, context, exitAction, -0.02, 1.0, 1.0), ShouldBeNil)

			readingExit := model.Recall(keyHolding, context, exitAction)
			readingHold := model.Recall(keyHolding, context, holdAction)
			readingFlatExit := model.Recall(keyFlat, context, exitAction)

			So(readingExit.Defined, ShouldBeTrue)
			So(readingExit.Rate, ShouldAlmostEqual, -0.02)
			So(readingHold.Defined, ShouldBeFalse)
			So(readingFlatExit.Defined, ShouldBeFalse)
		})

		// 7. Time survives resolution (elapsed time separate from wealth change).
		Convey("7. Time survives resolution as an independent fact", func() {
			model := NewEconomicModel(512)
			key := [2]string{"TEST/USD", "flat"}
			action := LearningAction{Kind: types.ActionEnter}

			ticket, err := model.Issue(key, []uint64{1}, action, 1.0)
			So(err, ShouldBeNil)

			reading, err := model.Resolve(ticket, 0.06, 3*time.Second)
			So(err, ShouldBeNil)

			So(reading.TotalGrowth, ShouldAlmostEqual, 0.06)
			So(reading.TotalTime, ShouldAlmostEqual, 3.0)
			So(reading.GrowthMean, ShouldAlmostEqual, 0.06)
			So(reading.TimeMean, ShouldAlmostEqual, 3.0)
			So(reading.Rate, ShouldAlmostEqual, 0.02)
		})

		// 8. Ratio of sums: sum(g)/sum(t) != mean(g/t).
		Convey("8. Ratio of sums objective strictly replaces arithmetic mean of rates", func() {
			model := NewEconomicModel(512)
			key := [2]string{"TEST/USD", "flat"}
			action := LearningAction{Kind: types.ActionEnter}

			// Observation 1: g = 0.10, t = 1.0s (rate = 0.10/s)
			So(model.Observe(key, nil, action, 0.10, 1.0, 1.0), ShouldBeNil)
			// Observation 2: g = 0.10, t = 9.0s (rate = 0.0111/s)
			So(model.Observe(key, nil, action, 0.10, 9.0, 1.0), ShouldBeNil)

			reading := model.Recall(key, nil, action)
			// Arithmetic mean of rates: (0.10 + 0.011111) / 2 ~= 0.0555
			// Ratio of sums: (0.10 + 0.10) / (1.0 + 9.0) = 0.20 / 10.0 = 0.020
			arithmeticMeanRate := 0.5 * (0.10/1.0 + 0.10/9.0)
			ratioOfSumsRate := 0.20 / 10.0

			So(reading.Rate, ShouldAlmostEqual, ratioOfSumsRate, 0.0001)
			So(math.Abs(reading.Rate-arithmeticMeanRate), ShouldBeGreaterThan, 0.02)
		})

		// 9. No future leakage: context frozen at issue time.
		Convey("9. Context and wealth are frozen at issue time without future leakage", func() {
			agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
			local := agent.LocalLearning

			market := &learningMarket{
				symbol: "TEST/USD",
				lanes:  make([]learningLane, 1),
				currentConditions: []uint64{101},
			}
			wallet, _ := virtualFixture()
			market.lanes[0].wallet = wallet
			market.lanes[0].equity = 1000.0
			local.markets["TEST/USD"] = market

			book := spotbook.New()
			book.NoBookCrossing = false
			book.Update(&spotbook.UpdateOptions{
				Direction: spotbook.Ask, ID: "ask",
				Price: decimal.NewFromInt64(100), Quantity: decimal.NewFromInt64(10), Silent: true,
			})
			book.Update(&spotbook.UpdateOptions{
				Direction: spotbook.Bid, ID: "bid",
				Price: decimal.NewFromInt64(99), Quantity: decimal.NewFromInt64(10), Silent: true,
			})

			t0 := time.Unix(100, 0)
			market.at = t0
			err := market.lanes[0].issue(local, market, 0, book, t0)
			So(err, ShouldBeNil)

			exp := market.lanes[0].trace[0]
			So(exp.context, ShouldResemble, []uint64{101})
			So(exp.wealthBefore, ShouldEqual, 1000.0)
			So(exp.at, ShouldEqual, t0)

			// Market evolves to condition 202 and then 303 in the future
			market.AdvanceImpulse([]learning.Region{{Condition: 202, Strength: 1.0, Authority: 1.0}})
			market.AdvanceImpulse([]learning.Region{{Condition: 303, Strength: 1.0, Authority: 1.0}})

			// Experience context remains frozen at [101]
			So(exp.context, ShouldResemble, []uint64{101})
		})

		// 10. No fixed lookback: backoff between shorter and longer patterns.
		Convey("10. Trie backoff allows evidence sharing between prefix depths", func() {
			model := NewEconomicModel(512)
			key := [2]string{"TEST/USD", "flat"}
			action := LearningAction{Kind: types.ActionEnter}

			shortPath := []uint64{101}
			longPath := []uint64{101, FrameDelimiter, 202}

			// Train only the short prefix
			So(model.Observe(key, shortPath, action, 0.04, 2.0, 1.0), ShouldBeNil)

			// Querying long path falls back to depth 1 prefix
			readingLong := model.Recall(key, longPath, action)
			So(readingLong.Defined, ShouldBeTrue)
			So(readingLong.Depth, ShouldEqual, 1)
			So(readingLong.ContextLength, ShouldEqual, 3)
			So(readingLong.Rate, ShouldAlmostEqual, 0.02)
		})

		// 11. Normal execution refusal does not freeze learning.
		Convey("11. Untradable quantity degrades to Hold without error", func() {
			agent, _ := agentFixture(t, func(hindsight.LearningEvent) error { return nil })
			local := agent.LocalLearning

			wallet, book := virtualFixture()
			market := &learningMarket{
				symbol: "TEST/USD",
				lanes:  make([]learningLane, 1),
			}

			// Give wallet negligible cash so minimum increment cannot be afforded
			wallet.cash = decimal.NewFromInt64(0)
			market.lanes[0].wallet = wallet
			local.markets["TEST/USD"] = market

			err := market.lanes[0].issue(local, market, 0, book, time.Now())
			So(err, ShouldBeNil)
			So(market.lanes[0].action.Kind, ShouldEqual, types.ActionHold)
		})

		// 12. Warmup is honest: unconditioned when quantities are missing.
		Convey("12. Warmup does not fabricate precursor history when quantities are missing", func() {
			knowledge := NewKnowledge(learning.NewGrid())
			at := time.Unix(100, 0)
			events := []hindsight.LearningEvent{
				{
					Run: "run1", ID: 1, Symbol: "TEST/USD", Kind: "issued",
					At: at, Context: []uint64{1, 2, 3}, Action: "enter", Authority: 1.0,
					// No Quantities registered!
				},
				{
					Run: "run1", ID: 1, Symbol: "TEST/USD", Kind: "resolved",
					At: at.Add(time.Second), TargetUnit: "absolute_return_per_second", Target: 0.05,
				},
			}

			report, err := knowledge.Warmup(events)
			So(err, ShouldBeNil)
			So(report.Resolved, ShouldEqual, 1)
			So(report.Unconditioned, ShouldEqual, 1)

			// The evidence should be at depth 0 (unconditioned root), not depth 3
			reading := knowledge.Model.Recall([2]string{"TEST/USD", "flat"}, []uint64{1, 2, 3}, LearningAction{Kind: types.ActionEnter})
			So(reading.Defined, ShouldBeTrue)
			So(reading.Depth, ShouldEqual, 0)
		})

		// 13. Real economics: virtual exploration uses real Price/Instrument paths.
		Convey("13. Virtual exploration charges real venue fees and respects order book depth", func() {
			wallet, book := virtualFixture()
			enterAction := LearningAction{Kind: types.ActionEnter}
			requested := decimal.NewFromInt64(2)

			initialCash := wallet.cash
			qty, gross, fee, err := wallet.fill(book, enterAction, requested)
			So(err, ShouldBeNil)
			So(qty.Cmp(requested), ShouldEqual, 0)
			So(gross.Sign(), ShouldBeGreaterThan, 0)
			So(fee.Sign(), ShouldBeGreaterThan, 0)

			// Total cash deducted must equal gross cost + fee
			expectedCash := initialCash.Sub(gross).Sub(fee)
			So(wallet.cash.Cmp(expectedCash), ShouldEqual, 0)
		})
	})
}
