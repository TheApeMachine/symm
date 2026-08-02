package strategy_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestUsers(t *testing.T) {
	Convey("Setup", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
			testtypes.NewSymbol("SIM3/USD", 100.0, 90210),
		}

		Convey("Given a market with symbols", tests.WithMarket(t, symbols, func(market *tests.Market) {
			Convey("When the market is in a Baseline state", func() {
				market.Tick()

				Convey("It should collect Measurements from the Signals", func() {
					So(market.Measurements(), ShouldNotBeEmpty)
				})

				Convey("When the market transitions to a fast pump", func() {
					market.Transition(testtypes.FastPump)

					Convey("The PumpDump Signal should have the correct values", func() {
						So(market.Measurements(), ShouldContainKey, "pumpdump")
					})

					Convey("The Strategy should generate an Opportunity Position", func() {
						So(market.Thesis.Decisions, ShouldNotBeEmpty)
					})

					Convey("When the pump has peaked", func() {
						market.Transition(testtypes.FastDump)

						Convey("The position StopLoss should trigger an Exit", func() {
							So(market.Thesis.Decisions, ShouldNotBeEmpty)
						})
					})
				})
			})
		}))
	})
}
