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

func TestWrite(t *testing.T) {
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
			Convey("When Write receives "+tc.name, func() {
				err := paper.Write(tc.message)

				Convey("error matches expectation", func() {
					So(err != nil, ShouldEqual, tc.wantErr)
				})
			})
		}
	}))
}

func TestPost(t *testing.T) {
	Convey("Setup", t, withPaper(func(paper *Paper) {
		Convey("When Post targets an unknown path", func() {
			raw, err := paper.Post("/0/private/OpenOrders", nil)

			Convey("Post is rejected", func() {
				So(err, ShouldNotBeNil)
				So(raw, ShouldBeNil)
			})
		})

		Convey("When the context is cancelled before Balance Post", func() {
			ctx, cancel := context.WithCancel(context.Background())
			paper = NewPaper(ctx)
			cancel()

			raw, err := paper.Post("/0/private/Balance", nil)

			Convey("Post is cancelled", func() {
				So(err, ShouldNotBeNil)
				So(raw, ShouldBeNil)
			})
		})
	}))
}

func TestRead(t *testing.T) {
	Convey("Setup", t, withPaper(func(paper *Paper) {
		Convey("When the context is cancelled with an empty queue", func() {
			ctx, cancel := context.WithCancel(context.Background())
			paper = NewPaper(ctx)
			cancel()

			_, err := paper.Read()

			Convey("Read returns an error", func() {
				So(err, ShouldNotBeNil)
			})
		})
	}))
}

func TestWriteAddOrder(t *testing.T) {
	Convey("Setup", t, func() {
		restore := swapPaperCommand(func(ctx context.Context, entity string, command ...string) (map[string]any, error) {
			So(entity, ShouldEqual, "add_order")

			return map[string]any{
				"order_id":     "O1",
				"success":      true,
				"exec_type":    "trade",
				"order_status": "filled",
				"cl_ord_id":    "paper-cl-1",
				"pair":         "BTC/USD",
				"side":         "buy",
				"volume":       0.1,
				"price":        50000.0,
				"trade_id":     "T1",
			}, nil
		})
		defer restore()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		paper := NewPaper(ctx)

		Convey("When add_order Write succeeds", func() {
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

			fill, err := paper.Read()
			So(err, ShouldBeNil)

			exec := &Execution{}
			So(exec.UnmarshalJSON(fill), ShouldBeNil)

			Convey("an ack and tradable execution are queued", func() {
				So(order.Success, ShouldBeTrue)
				So(order.Result.OrderID, ShouldEqual, "O1")
				So(exec.Tradable(), ShouldBeTrue)
				So(exec.Data[0].ClientOrderID, ShouldEqual, "paper-cl-1")
				So(exec.Data[0].LastQty.String(), ShouldEqual, "0.1")
				So(exec.Data[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func TestPostBalance(t *testing.T) {
	Convey("Setup", t, func() {
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

		Convey("When Balance Post runs", func() {
			raw, err := paper.Post("/0/private/Balance", nil)
			So(err, ShouldBeNil)

			bal := &Balance{}
			So(bal.UnmarshalJSON(raw), ShouldBeNil)

			Convey("USD cash decodes from paper JSON", func() {
				So(bal.QuoteCash("USD").String(), ShouldEqual, "10000")
			})
		})
	})
}

func TestRestPath(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When RestPath receives a full URL", func() {
			path := RestPath("https://api.kraken.com/0/private/Balance")

			Convey("the path component is returned", func() {
				So(path, ShouldEqual, "/0/private/Balance")
			})
		})

		Convey("When RestPath receives a bare path", func() {
			So(RestPath("/0/private/Balance"), ShouldEqual, "/0/private/Balance")
		})
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

func BenchmarkWrite(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	paper := NewPaper(ctx)
	message := mustJSON(map[string]any{"method": "ping"})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = paper.Write(message)
	}
}

func BenchmarkPost(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	paper := NewPaper(ctx)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = paper.Post("/0/private/OpenOrders", nil)
	}
}

func BenchmarkRead(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	paper := NewPaper(ctx)
	cancel()

	b.ReportAllocs()

	for b.Loop() {
		_, _ = paper.Read()
	}
}

func BenchmarkWriteAddOrder(b *testing.B) {
	restore := swapPaperCommand(func(ctx context.Context, entity string, command ...string) (map[string]any, error) {
		return map[string]any{
			"order_id":     "O1",
			"success":      true,
			"exec_type":    "trade",
			"order_status": "filled",
			"cl_ord_id":    "paper-cl-1",
			"pair":         "BTC/USD",
			"side":         "buy",
			"volume":       0.1,
			"price":        50000.0,
			"trade_id":     "T1",
		}, nil
	})
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	paper := NewPaper(ctx)
	message := []byte(`{
		"method":"add_order",
		"params":{
			"symbol":"BTC/USD",
			"side":"buy",
			"order_type":"market",
			"order_qty":"0.1",
			"cl_ord_id":"paper-cl-1"
		}
	}`)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = paper.Write(message)
		_, _ = paper.Read()
		_, _ = paper.Read()
	}
}

func BenchmarkPostBalance(b *testing.B) {
	restore := swapPaperCommand(func(ctx context.Context, entity string, _ ...string) (map[string]any, error) {
		return map[string]any{
			"balances": map[string]any{
				"USD": map[string]any{"available": 10000.0},
			},
		}, nil
	})
	defer restore()

	paper := NewPaper(context.Background())
	bal := &Balance{}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		raw, err := paper.Post("/0/private/Balance", nil)
		if err != nil {
			continue
		}
		_ = bal.UnmarshalJSON(raw)
	}
}

func BenchmarkRestPath(b *testing.B) {
	url := "https://api.kraken.com/0/private/Balance"

	b.ReportAllocs()

	for b.Loop() {
		_ = RestPath(url)
	}
}
