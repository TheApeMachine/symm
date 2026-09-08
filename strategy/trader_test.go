package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/tests/market"
)

func TestTraderExecute(t *testing.T) {
	Convey("Independent accounts use the real regulator for entry, reduction and exit", t, func() {
		learner, conn := learningFixture(t)
		tape := market.NewLevel3Tape("BTC/USD", time.Now())

		for _, message := range tape.Messages[:4] {
			conn.ApplyLevel3(message)
		}
		trader := learner.Traders[1]
		trader.At, trader.Version = time.Now(), 1
		actions, _, err := trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(len(actions), ShouldBeGreaterThan, 1)
		chosen := Action{Kind: "enter", Power: 1}
		So(trader.Quantities[chosen], ShouldNotBeNil)
		So(trader.Execute(&agent.Decision[Action]{ID: 1, Label: "BTC/USD", At: trader.At, Action: chosen}), ShouldBeNil)
		position := trader.Positions["BTC/USD"]
		So(position.Guardian, ShouldBeNil) // Synchronous fills need no background event listener.
		originalQuantity := position.Holding.Qty
		So(originalQuantity.Sign(), ShouldEqual, 1)
		So(trader.Balance.Cash().Cmp(trader.Initial), ShouldBeLessThan, 0)
		So(learner.Traders[0].Balance.Cash().Cmp(learner.Traders[0].Initial), ShouldEqual, 0)
		_, err = trader.Objective()
		So(err, ShouldBeNil)
		So(trader.Wealth, ShouldBeLessThan, 0)

		_, _, err = trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(trader.Execute(&agent.Decision[Action]{ID: 2, Label: "BTC/USD", At: trader.At, Action: Action{Kind: "scale", Power: 1, Reduce: true}}), ShouldBeNil)
		So(position.Holding.Qty.Sign(), ShouldEqual, 1)
		So(position.Holding.Qty.Cmp(originalQuantity), ShouldBeLessThan, 0)

		_, _, err = trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(trader.Execute(&agent.Decision[Action]{ID: 3, Label: "BTC/USD", At: trader.At, Action: Action{Kind: "exit", Reduce: true}}), ShouldBeNil)
		So(position.Holding.Qty.Sign(), ShouldEqual, 0)
		So(position.Holding.Basis.Sign(), ShouldEqual, 0)
		So(position.Holding.EntryFee.Sign(), ShouldEqual, 0)
		So(trader.Fills, ShouldEqual, 3)
		So(trader.Balance.Cash().Sub(trader.Initial).Cmp(position.Holding.RealizedPnL), ShouldEqual, 0)
	})
}
