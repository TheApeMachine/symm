package paper

import (
	"context"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestOrdersSend(t *testing.T) {
	Convey("Given a paper orders socket", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewWebSocket(ctx, pool)
		defer ws.Close()

		orders := ws.sockets[public.OrdersChannel].(*Orders)

		Convey("It should fill limit add_order on executions", func() {
			out := orders.Send(&qpool.QValue[any]{
				Type: public.OrdersChannel,
				Value: map[string]any{
					"method": trading.MethodAddOrder,
					"params": trading.AddParams{
						OrderType:  trading.Limit,
						Side:       trading.Buy,
						Symbol:     "BTC/EUR",
						OrderQty:   0.001,
						LimitPrice: 50_000,
						ClOrdID:    "paper-limit-fill",
					},
				},
			})

			So(out.Channel, ShouldEqual, public.ExecutionsChannel)
			So(string(out.Data), ShouldContainSubstring, `"cl_ord_id":"paper-limit-fill"`)
		})

		Convey("It should reject market add_order without a reference price", func() {
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

			So(out.Channel, ShouldBeEmpty)
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

			if orderID == "" {
				t.Fatal("extractOrderID returned empty orderID")
			}

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
	index := strings.Index(start, from)

	if index < 0 {
		return ""
	}

	rest := start[index+len(from):]
	end := strings.Index(rest, `"`)

	if end < 0 {
		return ""
	}

	return rest[:end]
}

func BenchmarkOrdersSend(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ(ctx, 1, 4, nil)
	ws := NewWebSocket(ctx, pool)
	defer ws.Close()

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
