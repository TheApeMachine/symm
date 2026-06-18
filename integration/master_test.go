package integration

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	. "github.com/theapemachine/symm/signal"
)

func TestMaster(testingTB *testing.T) {
	Convey("Given a trading system", testingTB, func() {
		ctx := testingTB.Context()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		pool := qpool.NewQ[any](ctx, 2, 4, &qpool.Config{Scaler: nil})
		trader := broker.NewTrader(ctx)
		tree, treeErr := logic.NewTree(ctx, pool)

		Convey("It should wire trader and playbook tree", func() {
			So(trader, ShouldNotBeNil)
			So(treeErr, ShouldBeNil)
			So(tree, ShouldNotBeNil)
			So(len(tree.Branches), ShouldBeGreaterThan, 0)
		})
	})
}

func TestMasterSignalCategoriesFromTreeFixtures(testingTB *testing.T) {
	Convey("Given tree-inserted classifier fixtures for every spectrum source", testingTB, func() {
		ctx := context.Background()
		pool := testPool(testingTB)

		defer pool.Close()

		tree := NewTestTree()
		story := market.NewStory(ctx, pool)

		defer story.Close()

		scope := "BTC/EUR"

		for _, fixture := range signalCategoryFixtures {
			insertClassifierMeasurement(
				tree,
				fixture.origin,
				scope,
				fixture.categoryIndex,
				0.82,
			)
		}

		ingestMeasurementsFromTree(tree, story, []string{scope})

		Convey("It should map classifier categories through the measurement contract", func() {
			measurements := story.Measurements()

			So(len(measurements), ShouldEqual, len(signalCategoryFixtures))

			for _, fixture := range signalCategoryFixtures {
				measurement, found := measurementBySource(measurements, fixture.wantSource)

				So(found, ShouldBeTrue)
				So(measurement.Symbol, ShouldEqual, scope)
				So(measurement.Category, ShouldEqual, fixture.wantCategory)
				So(measurement.Confidence, ShouldBeGreaterThan, 0)
			}
		})
	})
}

func TestMasterPlaybookWalkExitBeforeEntry(testingTB *testing.T) {
	Convey("Given tree fixtures with held exit and flat entry scopes", testingTB, func() {
		ctx := context.Background()
		pool := testPool(testingTB)

		defer pool.Close()

		tree := NewTestTree()
		story := market.NewStory(ctx, pool)

		defer story.Close()

		insertClassifierMeasurement(tree, "exhaust", "SOL/EUR", 1, 0.85)
		insertClassifierMeasurement(tree, "pumpdump", "BTC/EUR", 3, 0.82)
		insertClassifierMeasurement(tree, "sentiment", "BTC/EUR", 1, 0.82)
		insertTickerQuote(tree, "SOL/EUR", 100, 99.5, 100.5)
		insertTickerQuote(tree, "BTC/EUR", 50000, 49950, 50050)

		ingestMeasurementsFromTree(tree, story, []string{"SOL/EUR", "BTC/EUR"})

		story.SetBalances(&logic.Balances{
			Inventory: map[string]float64{"SOL/EUR": 1.5},
		})

		actions := sortActionsExitsFirst(story.Actions())

		Convey("It should evaluate exit and entry actions from embedded playbooks", func() {
			So(len(actions), ShouldBeGreaterThanOrEqualTo, 2)

			hasExit := false
			hasEntry := false

			for _, action := range actions {
				if action == nil {
					continue
				}

				if action.Type.IsExit() {
					hasExit = true
					So(action.Symbol, ShouldEqual, "SOL/EUR")
				}

				if action.Type == logic.ActionMarket {
					hasEntry = true
					So(action.Symbol, ShouldEqual, "BTC/EUR")
				}
			}

			So(hasExit, ShouldBeTrue)
			So(hasEntry, ShouldBeTrue)
			So(actions[0].Type.IsExit(), ShouldBeTrue)
			So(actions[1].Type, ShouldEqual, logic.ActionMarket)
		})

		Convey("It should record playbook walk traces for each scope", func() {
			exitTrace := story.WalkTrace("SOL/EUR")
			entryTrace := story.WalkTrace("BTC/EUR")

			So(exitTrace.Symbol, ShouldEqual, "SOL/EUR")
			So(entryTrace.Symbol, ShouldEqual, "BTC/EUR")
			So(len(exitTrace.ActivePath), ShouldBeGreaterThan, 0)
			So(len(entryTrace.ActivePath), ShouldBeGreaterThan, 0)
			So(walkTraceHasActionOutcome(exitTrace), ShouldBeTrue)
			So(walkTraceHasActionOutcome(entryTrace), ShouldBeTrue)
		})
	})
}

func TestMasterPlaybookActionTypes(testingTB *testing.T) {
	Convey("Given tree fixtures that match stop-loss and take-profit branches", testingTB, func() {
		ctx := context.Background()
		pool := testPool(testingTB)

		defer pool.Close()

		tree := NewTestTree()
		story := market.NewStory(ctx, pool)

		defer story.Close()

		insertClassifierMeasurement(tree, "exhaust", "ETH/EUR", 3, 0.88)
		insertClassifierMeasurement(tree, "pumpdump", "XRP/EUR", 4, 0.86)

		ingestMeasurementsFromTree(tree, story, []string{"ETH/EUR", "XRP/EUR"})

		story.SetBalances(&logic.Balances{
			Inventory: map[string]float64{
				"ETH/EUR": 2,
				"XRP/EUR": 500,
			},
		})

		actions := story.Actions()

		Convey("It should emit take-profit exits from category-mapped measurements", func() {
			foundTakeProfit := false

			for _, action := range actions {
				if action == nil {
					continue
				}

				if action.Type != logic.ActionTakeProfit {
					continue
				}

				foundTakeProfit = true
				So(action.Type.IsExit(), ShouldBeTrue)
				So(action.Symbol, ShouldBeIn, "ETH/EUR", "XRP/EUR")
			}

			So(foundTakeProfit, ShouldBeTrue)
		})
	})
}
