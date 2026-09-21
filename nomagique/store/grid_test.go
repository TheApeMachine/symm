package store_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestGrid(t *testing.T) {
	ctx := context.Background()

	Convey("Given a Grid metrics register with", t, func() {
		client := store.Grid_ServerToClient(store.NewGrid(ctx))

		register := func(metric, interests string) error {
			err := client.Write(ctx, func(params store.Grid_write_Params) error {
				if err := params.SetMetric(metric); err != nil {
					return err
				}

				return params.SetInterests(interests)
			})

			if err != nil {
				return err
			}

			return client.WaitStreaming()
		}

		publish := func(payload []byte) (out []byte, metric string, registered int64) {
			err := client.Write(ctx, func(params store.Grid_write_Params) error {
				return params.SetData(payload)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			delivered, err := results.Out()
			So(err, ShouldBeNil)

			name, err := results.Metric()
			So(err, ShouldBeNil)

			return bytes.Clone(delivered), name, results.Metrics()
		}

		Convey("It delivers a metric only the fields it declared", func() {
			So(register("cvd", "trade.price,trade.qty"), ShouldBeNil)

			payload, err := sonic.Marshal(map[string]any{
				"trade": map[string]any{
					"price": 100.0, "qty": 2.0, "side": "buy",
				},
				"unrelated": 1.0,
			})
			So(err, ShouldBeNil)

			out, metric, registered := publish(payload)
			So(metric, ShouldEqual, "cvd")
			So(registered, ShouldEqual, 1)

			var delivered map[string]any
			So(sonic.Unmarshal(out, &delivered), ShouldBeNil)

			So(delivered["trade.price"], ShouldEqual, 100.0)
			So(delivered["trade.qty"], ShouldEqual, 2.0)
			So(delivered, ShouldNotContainKey, "trade.side")
			So(delivered, ShouldNotContainKey, "unrelated")
		})

		Convey("It delivers nothing to a metric the data cannot satisfy", func() {
			So(register("depth", "book.bids.0.price"), ShouldBeNil)

			payload, err := sonic.Marshal(map[string]any{
				"trade": map[string]any{"price": 100.0},
			})
			So(err, ShouldBeNil)

			out, metric, _ := publish(payload)
			So(len(out), ShouldEqual, 0)
			So(metric, ShouldEqual, "")
		})

		Convey("It resolves an interest indexing a collection", func() {
			So(register("depth", "book.bids.0.price"), ShouldBeNil)

			payload, err := sonic.Marshal(map[string]any{
				"book": map[string]any{
					"bids": []any{
						map[string]any{"price": 99.0},
						map[string]any{"price": 98.0},
					},
				},
			})
			So(err, ShouldBeNil)

			out, metric, _ := publish(payload)
			So(metric, ShouldEqual, "depth")

			var delivered map[string]any
			So(sonic.Unmarshal(out, &delivered), ShouldBeNil)
			So(delivered["book.bids.0.price"], ShouldEqual, 99.0)
		})

		Convey("It counts every metric registered with it", func() {
			So(register("cvd", "trade.price"), ShouldBeNil)
			So(register("hawkes", "trade.timestamp"), ShouldBeNil)
			So(register("liquidity", "book.bid"), ShouldBeNil)

			payload, err := sonic.Marshal(map[string]any{
				"trade": map[string]any{"price": 1.0},
			})
			So(err, ShouldBeNil)

			_, metric, registered := publish(payload)
			So(registered, ShouldEqual, 3)
			So(metric, ShouldEqual, "cvd")
		})

		Convey("It replaces what a metric asked for when it registers again", func() {
			So(register("cvd", "trade.price"), ShouldBeNil)
			So(register("cvd", "trade.qty"), ShouldBeNil)

			payload, err := sonic.Marshal(map[string]any{
				"trade": map[string]any{"price": 100.0, "qty": 5.0},
			})
			So(err, ShouldBeNil)

			out, _, registered := publish(payload)
			So(registered, ShouldEqual, 1)

			var delivered map[string]any
			So(sonic.Unmarshal(out, &delivered), ShouldBeNil)
			So(delivered, ShouldContainKey, "trade.qty")
			So(delivered, ShouldNotContainKey, "trade.price")
		})

		Convey("It reports data that is not a structure", func() {
			So(register("cvd", "trade.price"), ShouldBeNil)

			err := client.Write(ctx, func(params store.Grid_write_Params) error {
				return params.SetData([]byte("not json"))
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}
