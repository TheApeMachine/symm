package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPositionExecution(t *testing.T) {
	Convey("Given a position receiving execution snapshots", t, func() {
		Convey("When an open execution snapshot arrives", func() {
			position := NewPosition(&recordingPrivate{}, &PositionData{
				Symbol:     "ETH/USD",
				Qty:        1,
				EntryPrice: *decimal.NewFromFloat64(100),
			})
			execution := &kraken.ExecutionData{
				AvgPrice:       *decimal.NewFromFloat64(101),
				LastQty:        2,
				OrderStatus:    "filled",
				PositionStatus: "open",
				Side:           "buy",
				Symbol:         "ETH/USD",
			}

			err := position.Execution(execution)

			Convey("Then the position remains open with the exchange-owned size and entry", func() {
				So(err, ShouldBeNil)
				So(position.status, ShouldEqual, types.OPEN)
				So(position.closing, ShouldBeFalse)
				So(position.data.Qty, ShouldAlmostEqual, 2)
				So(position.data.EntryPrice.String(), ShouldEqual, "101")
			})
		})

		Convey("When a closing sell execution fills", func() {
			private := &recordingPrivate{}
			position := NewPosition(private, &PositionData{
				Symbol:     "ETH/USD",
				Qty:        1,
				EntryPrice: *decimal.NewFromFloat64(100),
			})
			position.status = types.OPEN

			err := position.Exit()
			So(err, ShouldBeNil)
			So(position.status, ShouldEqual, types.PENDING)
			So(position.closing, ShouldBeTrue)

			err = position.Execution(&kraken.ExecutionData{
				OrderStatus: "filled",
				Side:        "sell",
				Symbol:      "ETH/USD",
			})

			Convey("Then the position is marked closed for the desk reaper", func() {
				So(err, ShouldBeNil)
				So(position.status, ShouldEqual, types.CLOSED)
			})
		})

		Convey("When a public ticker marks the open position", func() {
			position := NewPosition(&recordingPrivate{}, &PositionData{
				Symbol:     "ETH/USD",
				Qty:        1,
				EntryPrice: *decimal.NewFromFloat64(100),
			})
			position.SetFeeRate(0)

			err := position.AddTicker(&kraken.TickerData{
				Symbol: "ETH/USD",
				Bid:    *decimal.NewFromFloat64(101),
				Ask:    *decimal.NewFromFloat64(101.1),
				Last:   *decimal.NewFromFloat64(101),
			})

			Convey("Then mark, PnL, return, and readiness are initialized together", func() {
				So(err, ShouldBeNil)
				So(position.data.Mark.String(), ShouldEqual, "101")
				So(position.data.PnL.String(), ShouldEqual, "1")
				So(position.data.ReturnPct, ShouldAlmostEqual, 0.01)
			})
		})
	})
}
