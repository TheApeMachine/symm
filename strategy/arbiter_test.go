package strategy

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/types"
)

/*
TestArbiterSelect admits the higher-utility enter when only one normal slot is
free, and records the lower-utility enter as nothing.
*/
func TestArbiterSelect(t *testing.T) {
	Convey("Given one free slot and two enter candidates", t, func() {
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		Reset(func() {
			viper.Set("market.quote_currency", previousQuote)
		})

		trading := config.Fixture().Trading
		trading.SlotsNormal = 1
		trading.SlotsReserved = 0
		balance := broker.NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		desk := broker.NewDesk(t.Context(), nil, nil, nil, balance, trading)
		rotate := NewRotate()
		admit := NewAdmit(context.Background(), balance, desk, rotate)
		arbiter := NewArbiter(desk, broker.NewPrice(nil), balance, admit, rotate)
		thesis := types.NewThesis(nil)
		thesis.Decisions = []types.Decision{
			{Action: types.ActionEnter, Symbol: "LOW", Utility: 0.1},
			{Action: types.ActionEnter, Symbol: "HIGH", Utility: 0.9},
		}

		arbiter.Select(thesis)

		var entered, rejected int

		for _, decision := range thesis.Decisions {
			switch decision.Action {
			case types.ActionEnter:
				entered++
				So(decision.Symbol, ShouldEqual, "HIGH")
			case types.ActionNothing:
				rejected++
				So(decision.Symbol, ShouldEqual, "LOW")
			}
		}

		So(entered, ShouldEqual, 1)
		So(rejected, ShouldEqual, 1)
	})
}
