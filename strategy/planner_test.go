package strategy_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
)

func TestUsers(t *testing.T) {
	Convey("Setup", func() {
		symbols := []*tests.Symbol{
			tests.NewSymbol("SIM1/USD", 100.0, 42),
			tests.NewSymbol("SIM2/USD", 100.0, 1337),
			tests.NewSymbol("SIM3/USD", 100.0, 90210),
		}

		Convey("Given a market with symbols", t, tests.WithMarket(t, symbols, func(market *tests.Market) {
			Convey("When the market is in a Baseline state", func() {
				Convey("It should collect Measurements from the Signals", func() {
					market.Tick()
				})

				Convey("When the market transitions to a fast pump", func() {
					market.Transition(tests.FastPump)

					Convey("The PumpDump Signal should have the correct values", func() {
						// TODO: Make sure we can access the Thesis, so we can inspect the
						// full tick state.
					})

					Convey("The Strategy should generate an Opportunity Position", func() {
						// TODO: Make sure we can access the Thesis, so we can inspect the
						// full tick state.
					})

					Convey("When the pump has peaked", func() {
						market.Transition(tests.FastDump)

						Convey("The position StopLoss should trigger an Exit", func() {
							// TODO: Make sure we can access the Thesis, so we can inspect the
						})
					})
				})
			})
		}))
	})
}
