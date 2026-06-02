package paper

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestOrdersSend(t *testing.T) {
	Convey("Given a paper orders socket", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewWebSocket(ctx, pool)
		orders := ws.sockets[public.OrdersChannel].(*Orders)

		Convey("It should fill add_order on executions", func() {
			out := orders.Send(&qpool.QValue[any]{
				Type: public.OrdersChannel,
				Value: map[string]any{
					"method": trading.MethodAddOrder,
					"params": trading.AddParams{
						OrderType:  trading.Market,
						Side:       trading.Buy,
						Symbol:     "BTC/EUR",
						OrderQty:   0.01,
						LimitPrice: 50_000,
					},
				},
			})

			So(out.Channel, ShouldEqual, public.ExecutionsChannel)
			So(out.Type, ShouldEqual, "update")
		})

		Convey("It should rest post-only limits on the book", func() {
			out := orders.Send(&qpool.QValue[any]{
				Type: public.OrdersChannel,
				Value: map[string]any{
					"method": trading.MethodAddOrder,
					"params": trading.AddParams{
						OrderType:  trading.Limit,
						Side:       trading.Sell,
						Symbol:     "BTC/EUR",
						OrderQty:   0.5,
						LimitPrice: 60_000,
						PostOnly:   true,
					},
				},
			})

			So(out.Channel, ShouldEqual, public.ExecutionsChannel)
			So(string(out.Data), ShouldContainSubstring, `"order_status":"open"`)
		})

		Convey("It should cancel resting orders on cancel_order", func() {
			open := orders.Send(&qpool.QValue[any]{
				Type: public.OrdersChannel,
				Value: map[string]any{
					"method": trading.MethodAddOrder,
					"params": trading.AddParams{
						OrderType:  trading.Limit,
						Side:       trading.Sell,
						Symbol:     "BTC/EUR",
						OrderQty:   0.5,
						LimitPrice: 60_000,
						PostOnly:   true,
					},
				},
			})

			orderID := extractOrderID(open)

			out := orders.Send(&qpool.QValue[any]{
				Type: public.OrdersChannel,
				Value: map[string]any{
					"method": trading.MethodCancelOrder,
					"params": map[string]any{
						"order_id": []string{orderID},
					},
				},
			})

			So(out.Channel, ShouldEqual, public.ExecutionsChannel)
			So(string(out.Data), ShouldContainSubstring, `"exec_type":"canceled"`)
		})
	})
}

func extractOrderID(message public.SocketMessage) string {
	start := string(message.Data)
	from := `"order_id":"`
	index := indexOf(start, from)

	if index < 0 {
		return ""
	}

	rest := start[index+len(from):]
	end := indexOf(rest, `"`)

	if end < 0 {
		return ""
	}

	return rest[:end]
}

func indexOf(text, part string) int {
	for index := 0; index+len(part) <= len(text); index++ {
		if text[index:index+len(part)] == part {
			return index
		}
	}

	return -1
}

func BenchmarkOrdersSend(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 1, 4, nil)
	ws := NewWebSocket(ctx, pool)
	orders := ws.sockets[public.OrdersChannel].(*Orders)
	message := &qpool.QValue[any]{
		Type: public.OrdersChannel,
		Value: map[string]any{
			"method": trading.MethodAddOrder,
			"params": trading.AddParams{
				OrderType:  trading.Market,
				Side:       trading.Buy,
				Symbol:     "BTC/EUR",
				OrderQty:   0.01,
				LimitPrice: 50_000,
			},
		},
	}

	for b.Loop() {
		orders.Send(message)
	}
}
