package kraken

import (
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func withPaper(f func(*Paper)) func() {
	return func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f(NewPaper(ctx))
	}
}

func TestPaperWrite(t *testing.T) {
	Convey("Setup", t, withPaper(func(paper *Paper) {
		cases := []struct {
			name    string
			message []byte
			wantErr bool
		}{
			{"garbage JSON", []byte("{"), true},
			{"unknown method", mustJSON(map[string]any{"method": "ping"}), true},
			{
				"unknown subscribe channel",
				mustJSON(map[string]any{
					"method": "subscribe",
					"params": map[string]any{"channel": "ticker"},
				}),
				true,
			},
			{
				"subscribe without params",
				mustJSON(map[string]any{"method": "subscribe"}),
				true,
			},
		}

		for _, tc := range cases {
			Convey("When "+tc.name, func() {
				err := paper.Write(tc.message)
				So(err != nil, ShouldEqual, tc.wantErr)
			})
		}
	}))
}

func TestPaperPost(t *testing.T) {
	Convey("Setup", t, withPaper(func(paper *Paper) {
		Convey("When the path is unknown", func() {
			raw, err := paper.Post("/0/private/OpenOrders", nil)
			So(err, ShouldNotBeNil)
			So(raw, ShouldBeNil)
		})

		Convey("When the context is cancelled before Balance Post", func() {
			ctx, cancel := context.WithCancel(context.Background())
			paper = NewPaper(ctx)
			cancel()

			raw, err := paper.Post("/0/private/Balance", nil)
			So(err, ShouldNotBeNil)
			So(raw, ShouldBeNil)
		})
	}))
}

func TestPaperRead(t *testing.T) {
	Convey("Setup", t, withPaper(func(paper *Paper) {
		Convey("When the context is cancelled with an empty queue", func() {
			ctx, cancel := context.WithCancel(context.Background())
			paper = NewPaper(ctx)
			cancel()

			_, err := paper.Read()
			So(err, ShouldNotBeNil)
		})
	}))
}

func TestPaperAddOrderHappyPath(t *testing.T) {
	Convey("add_order Write queues an ack and a tradable execution", t, func() {
		restore := swapPaperCommand(func(ctx context.Context, entity string, command ...string) (map[string]any, error) {
			So(entity, ShouldEqual, "add_order")

			return map[string]any{
				"order_id":      "O1",
				"success":       true,
				"exec_type":     "trade",
				"order_status":  "filled",
				"cl_ord_id":     "paper-cl-1",
				"pair":          "BTC/USD",
				"side":          "buy",
				"volume":        0.1,
				"price":         50000.0,
				"trade_id":      "T1",
			}, nil
		})
		defer restore()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		paper := NewPaper(ctx)
		err := paper.Write([]byte(`{
			"method":"add_order",
			"params":{
				"symbol":"BTC/USD",
				"side":"buy",
				"order_type":"market",
				"order_qty":"0.1",
				"cl_ord_id":"paper-cl-1"
			}
		}`))
		So(err, ShouldBeNil)

		ack, err := paper.Read()
		So(err, ShouldBeNil)

		var order Order
		So(sonic.Unmarshal(ack, &order), ShouldBeNil)
		So(order.Success, ShouldBeTrue)
		So(order.Result.OrderID, ShouldEqual, "O1")

		fill, err := paper.Read()
		So(err, ShouldBeNil)

		exec := &Execution{}
		So(exec.UnmarshalJSON(fill), ShouldBeNil)
		So(exec.Tradable(), ShouldBeTrue)
		So(len(exec.Data), ShouldBeGreaterThan, 0)
		So(exec.Data[0].ClientOrderID, ShouldEqual, "paper-cl-1")
		So(exec.Data[0].LastQty.String(), ShouldEqual, "0.1")
		So(exec.Data[0].Symbol, ShouldEqual, "BTC/USD")
	})
}

func TestPaperBalancePost(t *testing.T) {
	Convey("Balance Post returns venue JSON through the paper runner", t, func() {
		restore := swapPaperCommand(func(ctx context.Context, entity string, _ ...string) (map[string]any, error) {
			So(entity, ShouldEqual, "balances")

			return map[string]any{
				"balances": map[string]any{
					"USD": map[string]any{"available": 10000.0},
				},
			}, nil
		})
		defer restore()

		paper := NewPaper(context.Background())
		raw, err := paper.Post("/0/private/Balance", nil)
		So(err, ShouldBeNil)
		So(raw, ShouldNotBeEmpty)

		bal := &Balance{}
		So(bal.UnmarshalJSON(raw), ShouldBeNil)
		So(bal.QuoteCash("USD").String(), ShouldEqual, "10000")
	})
}

func swapPaperCommand(fn paperCommandFunc) func() {
	prev := runPaperCommand
	runPaperCommand = fn

	return func() { runPaperCommand = prev }
}

func mustJSON(value map[string]any) []byte {
	buf, err := sonic.Marshal(value)
	if err != nil {
		panic(err)
	}
	return buf
}
