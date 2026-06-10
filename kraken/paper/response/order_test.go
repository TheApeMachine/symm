package response

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
)

func TestOrdersSend(t *testing.T) {
	Convey("Given an add_order frame", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		orders := NewOrders(ctx, pool, krakenmarket.NewBookStore(10))

		frame := types.KrakenMessage{
			Method: trading.MethodAddOrder,
			Params: &trading.AddParams{ClOrdID: "d7ce4944-f4df-4447-8314-14f020"},
			ReqID:  0,
		}

		response := orders.Send(&qpool.QValue[any]{Value: frame})

		Convey("It should marshal order updates as an array", func() {
			So(response, ShouldNotBeNil)
			So(response.Channel, ShouldEqual, "orders")

			decoded := []trading.OrderUpdate{}

			So(response.Unmarshal(&decoded), ShouldBeNil)
			So(len(decoded), ShouldEqual, 1)
			So(decoded[0].OrderID, ShouldEqual, "d7ce4944-f4df-4447-8314-14f020")
		})
	})
}
