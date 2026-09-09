package strategy

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"testing"
)

func TestRecordedBookStep(t *testing.T) {
	Convey("Captured quotes drive the normal funded position lifecycle", t, func() {
		learner, _ := learningFixture(t)
		source := &recordedBook{}
		price := broker.NewRecordedPrice(learner.price, source)
		trader, err := NewTrader(learner.Traders[0].api, price, "USD", learner.funding)
		So(err, ShouldBeNil)
		observations := rehearsalObservations()
		source.Step(observations[2])
		trader.At = observations[2].ReceivedAt
		actions, _, err := trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		quantity := trader.Quantities[Action{Kind: "enter", Power: 1}]
		So(quantity, ShouldNotBeNil)
		for index := 3; index < len(observations); index++ {
			observations[index].AskQty = 0
		}
		unchanged, _, err := trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(unchanged, ShouldResemble, actions) // No future quote can affect this cursor.
		So(trader.Execute(&agent.Decision[Action]{ID: 1, Label: "BTC/USD", At: trader.At, Action: Action{Kind: "enter", Power: 1}}), ShouldBeNil)
		So(trader.Balance.Cash().Sign(), ShouldBeGreaterThanOrEqualTo, 0)
		So(trader.Balance.Cash().Cmp(trader.Initial), ShouldBeLessThan, 0)
		So(trader.Positions["BTC/USD"].Holding.Qty.Cmp(quantity), ShouldEqual, 0)
		for index := 3; index <= 6; index++ {
			source.Step(observations[index])
			trader.At = observations[index].ReceivedAt
		}
		_, _, err = trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(trader.Execute(&agent.Decision[Action]{ID: 2, Label: "BTC/USD", At: trader.At, Action: Action{Kind: "exit", Reduce: true}}), ShouldBeNil)
		So(trader.Positions["BTC/USD"].Holding.Qty.Sign(), ShouldEqual, 0)
		So(trader.Fills, ShouldEqual, 2)
		So(learner.Traders[0].Fills, ShouldEqual, 0)
	})
}
