package strategy

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
TestArbiterSelect admits the higher-utility enter when only one normal slot is
free, and records the lower-utility enter as nothing.
*/
func TestArbiterSelect(t *testing.T) {
	Convey("Given one free slot and two enter candidates", t, func() {
		previousNormal := viper.GetInt("trading.slots.normal")
		previousReserved := viper.GetInt("trading.slots.reserved")
		viper.Set("trading.slots.normal", 1)
		viper.Set("trading.slots.reserved", 0)
		Reset(func() {
			viper.Set("trading.slots.normal", previousNormal)
			viper.Set("trading.slots.reserved", previousReserved)
		})

		balance := broker.NewBalance(nil, nil, make(chan []byte, 1))
		desk := broker.NewDesk(nil, nil, nil, balance)
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
