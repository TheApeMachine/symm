package broker

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
TestEntryTrailCoversRoundTripFeeAndHalfSpread proves warm books widen the
fill-time survival band past fee-only width.
*/
func TestEntryTrailCoversRoundTripFeeAndHalfSpread(t *testing.T) {
	Convey("Given a filled lot with fee and a warm touch book", t, func() {
		price := NewPrice(nil)
		So(price.RememberFee("XCN/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)
		price.TickerAck([]byte(
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"XCN/USD","last":"1.00","bid":"0.995","ask":"1.005"}]}`,
		))

		position := &Position{price: price}
		holding := &types.Holding{
			Symbol:     "XCN/USD",
			EntryPrice: decimal.NewFromFloat64(1.0),
			EntryFee:   decimal.NewFromFloat64(0.0026),
			Qty:        decimal.NewFromFloat64(1),
		}

		trail := position.EntryTrail(holding)

		Convey("Then the bind width exceeds fee-only (~0.26%) survival", func() {
			So(trail, ShouldBeGreaterThan, 0.0026)
			So(trail, ShouldBeGreaterThanOrEqualTo, 0.01)
		})
	})
}

/*
TestColdBindWidenHoldsOnWarmMid drives Paper/Simulator-backed Price frames to
prove a fee-thin cold bind does not EXIT stop once the live trail widens and mid
sits inside that band — the VANRY instant-exit failure mode.
*/
func TestColdBindWidenHoldsOnWarmMid(t *testing.T) {
	Convey("Given a Paper Simulator stack and a fee-only cold bind", t, func() {
		ctx := context.Background()
		mock := mockapi.NewMockAPI()
		paper := websocket.NewPaper(ctx, websocket.NewSimulator())
		api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), paper)
		price := NewPrice(api)
		So(price.RememberFee("VANRY/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)

		position := &Position{price: price, api: api}
		holding := &types.Holding{
			Symbol:     "VANRY/USD",
			EntryPrice: decimal.NewFromFloat64(1.02),
			EntryFee:   decimal.NewFromFloat64(0.002652),
			Qty:        decimal.NewFromFloat64(1),
			Status:     types.OPEN,
		}

		cold := position.EntryTrail(holding)
		So(cold, ShouldBeGreaterThan, 0)
		So(cold, ShouldBeLessThan, 0.01)
		position.BindStop(holding.EntryPrice.Float64(), cold)
		holding.Stoploss = position.Stop()

		Convey("When the Simulator-fed ticker warms a wide book", func() {
			price.TickerAck([]byte(
				`{"channel":"ticker","type":"update","data":[{` +
					`"symbol":"VANRY/USD","last":"1.01","bid":"1.00","ask":"1.02"}]}`,
			))
			pair := &kraken.InstrumentPair{
				Symbol: "VANRY/USD", Base: "VANRY", Quote: "USD",
				QtyPrecision: 8, CostPrecision: 8,
			}
			So(price.Mark(pair, holding), ShouldBeNil)
			So(holding.StopMark, ShouldNotBeNil)

			warm := position.EntryTrail(holding)
			So(warm, ShouldBeGreaterThan, cold)
			holding.Stoploss.WidenSurvival(warm)

			mid := holding.StopMark.Float64()
			verdict := holding.Stoploss.Update(types.StopEvidence{
				Symbol: "VANRY/USD", Mark: mid, Entry: 1.02, Present: true,
			})

			Convey("Then mid-at-touch holds instead of Cause stop", func() {
				So(verdict.Action, ShouldEqual, "hold")
				So(holding.Stoploss.FloorDistance, ShouldEqual, warm)
			})

			Convey("And a partial peak ratchets the stop up under the peak", func() {
				// +~1% peak stays inside a ~1.5% warm survival band so the floor
				// has not locked above entry yet, but StopReturn must already rise.
				lifted := holding.Stoploss.Update(types.StopEvidence{
					Symbol: "VANRY/USD", Mark: 1.03, Entry: 1.02, Present: true,
				})
				So(lifted.Action, ShouldEqual, "hold")
				So(holding.Stoploss.StopReturn, ShouldBeGreaterThan, -warm)
				So(holding.Stoploss.StopReturn, ShouldBeLessThan, 0)
			})

			Convey("And a sincere ungated breach still stops", func() {
				deep := holding.Stoploss.Update(types.StopEvidence{
					Symbol: "VANRY/USD", Mark: 0.95, Entry: 1.02, Present: true,
				})
				So(deep.Action, ShouldEqual, "stop")
			})
		})
	})
}
