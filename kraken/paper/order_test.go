package paper

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
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

			channel, _ := out["channel"].(string)
			data, _ := out["data"].(json.RawMessage)

			So(channel, ShouldEqual, public.ExecutionsChannel)
			So(string(data), ShouldContainSubstring, `"cl_ord_id":"paper-limit-fill"`)
		})

		Convey("It should reject market add_order without a reference price", func() {
			out := orders.Send(&qpool.QValue[any]{
				Type: public.OrdersChannel,
				Value: map[string]any{
					"method": trading.MethodAddOrder,
					"params": trading.AddParams{
						OrderType:  trading.Market,
						Side:       trading.Buy,
						Symbol:     "NOQUOTE/EUR",
						OrderQty:   0.01,
						LimitPrice: 50_000,
					},
				},
			})

			channel, _ := out["channel"].(string)

			So(channel, ShouldBeEmpty)
		})

		Convey("It should rest post-only limits on the book", func() {
			broker.EnsureQuoteCache(ctx, pool).InstallQuoteForTest(broker.Quote{
				Symbol: "BTC/EUR",
				Bid:    59_000,
				Ask:    60_500,
				Last:   60_000,
			})

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

			channel, _ := out["channel"].(string)
			data, _ := out["data"].(json.RawMessage)

			So(channel, ShouldEqual, public.ExecutionsChannel)
			So(string(data), ShouldContainSubstring, `"order_status":"open"`)
		})

		Convey("It should cancel resting orders on cancel_order", func() {
			broker.EnsureQuoteCache(ctx, pool).InstallQuoteForTest(broker.Quote{
				Symbol: "BTC/EUR",
				Bid:    59_000,
				Ask:    60_500,
				Last:   60_000,
			})

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

			channel, _ := out["channel"].(string)
			data, _ := out["data"].(json.RawMessage)

			So(channel, ShouldEqual, public.ExecutionsChannel)
			So(string(data), ShouldContainSubstring, `"exec_type":"canceled"`)
		})
	})
}

func extractOrderID(message map[string]any) string {
	data, ok := message["data"].(json.RawMessage)

	if !ok {
		return ""
	}

	from := `"order_id":`
	index := strings.Index(string(data), from)

	if index < 0 {
		return ""
	}

	rest := strings.TrimSpace(string(data[index+len(from):]))

	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}

	rest = rest[1:]
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
