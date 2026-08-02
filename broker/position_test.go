package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestPositionOnExecution(t *testing.T) {
	Convey("Given a desk-owned pending position", t, func() {
		position := &Position{
			ID:     "decision-id",
			Status: types.PENDING,
			pair:   kraken.InstrumentPair{Symbol: "BTC/USD"},
			Holding: &types.Holding{
				Symbol: "BTC/USD",
				Status: types.PENDING,
			},
			cancel: func() {},
			ui:     make(chan []byte, 4),
		}

		desk := &Desk{positions: &sync.Map{}}
		desk.positions.Store(position.ID, position)

		Convey("When its correlated order-open event arrives without a fill", func() {
			position.onExecution(&kraken.Execution{Data: []kraken.ExecutionData{{
				ClientOrderID: position.ID,
				Symbol:        "BTC/USD",
				Side:          "buy",
				ExecType:      "new",
				OrderStatus:   "open",
			}}})

			Convey("It should remain pending", func() {
				So(position.Status, ShouldEqual, types.PENDING)
				So(position.Holding.Status, ShouldEqual, types.PENDING)
				So(desk.OpenPositions(), ShouldEqual, 1)
			})
		})

		Convey("When its correlated buy fill arrives", func() {
			position.onExecution(&kraken.Execution{Data: []kraken.ExecutionData{{
				ClientOrderID: position.ID,
				Symbol:        "BTC/USD",
				Side:          "buy",
				OrderStatus:   "filled",
				CumQty:        decimal.NewFromFloat64(0.25),
				AvgPrice:      decimal.NewFromInt64(100),
				FeeUsdEquiv:   decimal.NewFromFloat64(0.10),
				Timestamp:     time.Now().UTC(),
			}}})

			Convey("It should become open inventory visible to slot accounting", func() {
				So(position.Status, ShouldEqual, types.OPEN)
				So(position.Holding.Status, ShouldEqual, types.OPEN)
				So(position.Holding.SellableQty.String(), ShouldEqual, "0.25")
				So(desk.OpenPositions(), ShouldEqual, 1)
			})

			Convey("When its correlated sell fill arrives", func() {
				position.onExecution(&kraken.Execution{Data: []kraken.ExecutionData{{
					ClientOrderID: position.ID,
					Symbol:        "BTC/USD",
					Side:          "sell",
					OrderStatus:   "filled",
					CumQty:        decimal.NewFromFloat64(0.25),
					AvgPrice:      decimal.NewFromInt64(110),
					FeeUsdEquiv:   decimal.NewFromFloat64(0.11),
					Timestamp:     time.Now().UTC(),
				}}})

				Convey("It should release its desk slot", func() {
					So(position.Status, ShouldEqual, types.CLOSED)
					So(position.Holding.Status, ShouldEqual, types.CLOSED)
					So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)
					So(desk.OpenPositions(), ShouldEqual, 0)
				})
			})
		})
	})
}
