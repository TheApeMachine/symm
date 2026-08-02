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

		Convey("Given a market with symbols", t, tests.WithMarket(t, symbols, func(m *tests.Market) {
			Convey("When the market is in a Baseline state", func() {
				Convey("It should collect Measurements from the Signals", func() {
					m.Start
				})
			})
		}))
	})
}
