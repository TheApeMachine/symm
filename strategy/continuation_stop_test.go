package strategy

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
TestContinuityManageHoldsWarmMidAfterColdBind proves Continuity.Manage widens
the survival band from live EntryTrail before Regulate when Paper/Simulator
ticker frames warm a wide book after a fee-only cold bind.
*/
func TestContinuityManageHoldsWarmMidAfterColdBind(t *testing.T) {
	Convey("Given Desk/Balance/Price wired through Paper Simulator", t, func() {
		ctx := context.Background()
		public := mockapi.NewConn()
		private := mockapi.NewConn()
		paper := websocket.NewPaper(ctx, websocket.NewSimulator())
		api := websocket.NewAPI(ctx, public, private, paper)
		price := broker.NewPrice(api)
		So(price.RememberFee("VANRY/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)

		instrument := broker.NewInstrument(api, price, nil)
		instrument.On([]byte(
			`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
				`"symbol":"VANRY/USD","base":"VANRY","quote":"USD","status":"online",` +
				`"qty_precision":8,"qty_increment":0.00000001,"price_precision":5,` +
				`"cost_precision":5,"cost_min":0.5,"tick_size":0.00001,` +
				`"price_increment":0.00001,"qty_min":1}]}}`,
		))

		lot := types.NewHolding(ctx, "VANRY/USD", decimal.NewFromFloat64(1))
		lot.Asset = "VANRY"
		lot.Status = types.OPEN
		lot.EntryPrice = decimal.NewFromFloat64(1.02)
		lot.EntryFee = decimal.NewFromFloat64(0.002652)
		lot.Qty = decimal.NewFromFloat64(1)
		lot.SellableQty = decimal.NewFromFloat64(1)

		balance := broker.NewBalance(api, nil, nil)
		balance.Seed(lot)
		desk := broker.NewDesk(api, instrument, price, balance)
		So(desk.Initialize(), ShouldBeNil)
		desk.AdoptOpen()

		position, ok := desk.Position("VANRY/USD")
		So(ok, ShouldBeTrue)
		So(position, ShouldNotBeNil)

		cold := position.EntryTrail(lot)
		So(cold, ShouldBeGreaterThan, 0)
		So(cold, ShouldBeLessThan, 0.01)
		position.BindStop(lot.EntryPrice.Float64(), cold)
		lot.Stoploss = position.Stop()
		balance.Seed(lot)

		price.TickerAck([]byte(
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"VANRY/USD","last":"1.01","bid":"1.00","ask":"1.02"}]}`,
		))
		pair, err := instrument.Pair("VANRY/USD")
		So(err, ShouldBeNil)
		So(price.Mark(pair, lot), ShouldBeNil)
		balance.Seed(lot)

		thesis := types.NewThesis(nil, nil)
		continuity := NewContinuity(price, balance, desk, NewRotate(), NewEvidence())

		Convey("When Continuity manages the warm mid cut", func() {
			continuity.Manage(thesis)

			Convey("Then no exit Decision with Cause stop is published", func() {
				for _, decision := range thesis.Decisions {
					So(decision.Cause, ShouldNotEqual, "stop")
					So(decision.Action, ShouldNotEqual, types.ActionExit)
				}

				So(position.Stop().Action, ShouldEqual, "hold")
			})
		})

		Convey("When mid is driven sincerely through the armed floor", func() {
			lot.StopMark = decimal.NewFromFloat64(0.95)
			lot.Mark = decimal.NewFromFloat64(0.95)
			balance.Seed(lot)
			deep := types.NewThesis(nil, nil)
			continuity.Manage(deep)

			Convey("Then Regulate still emits Cause stop", func() {
				found := false

				for _, decision := range deep.Decisions {
					if decision.Symbol == "VANRY/USD" && decision.Cause == "stop" {
						found = true
						So(decision.Action, ShouldEqual, types.ActionExit)
					}
				}

				So(found, ShouldBeTrue)
			})
		})
	})
}
