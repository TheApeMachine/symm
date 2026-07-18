package tests_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	orderackfixture "github.com/theapemachine/symm/tests/fixtures/orderack"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
)

func TestTradesHistoryScenarios(t *testing.T) {
	Convey("Given named trade-history scenarios", t, func() {
		for _, testCase := range []struct {
			caseName string
			scenario tests.TradesScenario
			tradeKey string
		}{
			{
				caseName: "closed round trip with current lot",
				scenario: tests.TradesClosedRoundTripCurrentLot,
				tradeKey: "current-buy",
			},
			{
				caseName: "hydrate btc and gala",
				scenario: tests.TradesHydrateBTCAndGALA,
				tradeKey: "btc-buy",
			},
			{
				caseName: "managing entry order",
				scenario: tests.TradesManagingEntryOrder,
				tradeKey: "current-buy",
			},
		} {
			Convey(testCase.caseName, func() {
				history := tests.TradesHistory(testCase.scenario)

				So(history.Result.Trades, ShouldContainKey, testCase.tradeKey)
			})
		}
	})
}

func TestMarketPrefixPayload(t *testing.T) {
	Convey("Given channel and RPC prefix fixtures", t, func() {
		market := tests.NewMarket().
			Prefix(tickerfixture.NewFixture(tickerfixture.SNAPSHOT, 1)).
			PrefixPayload(orderackfixture.NewFixture(
				orderackfixture.Options{ReqID: 7, OrderID: "right", Success: true},
			)).
			Feed(executionfixture.NewFixture(executionfixture.BuyFill()))

		order := make([]string, 0)

		for frame := range market.Frames() {
			order = append(order, frame.Channel)
		}

		Convey("Then snapshots and channel frames preserve routing order", func() {
			So(order, ShouldResemble, []string{"ticker", "executions"})
		})
	})
}

func TestReplaySteps(t *testing.T) {
	Convey("Given ordered replay steps", t, func() {
		seen := make([]string, 0)
		tests.ReplaySteps(
			tests.ReplayStep{
				Deliver: func([]byte) { seen = append(seen, "order") },
				Payload: orderackfixture.Frame(orderackfixture.Options{
					ReqID: 7, OrderID: "right", Success: true,
				}),
			},
			tests.ReplayStep{
				Deliver: func([]byte) { seen = append(seen, "execution") },
				Payload: executionfixture.Frame(executionfixture.BuyFill()),
			},
		)

		Convey("Then callbacks run in configured order", func() {
			So(seen, ShouldResemble, []string{"order", "execution"})
		})
	})
}
