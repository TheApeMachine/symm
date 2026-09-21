package store_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestGridWrite(t *testing.T) {
	ctx := context.Background()

	Convey("Given a grid told which fields to deliver", t, func() {
		client := store.Grid_ServerToClient(store.NewGrid(ctx))
		defer client.Release()

		declare := func(interests string) {
			err := client.Write(ctx, func(params store.Grid_write_Params) error {
				return params.SetInterests(interests)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
		}

		// One feed landing on the gathering port, the way a single venue does.
		publish := func(payload []byte) (out []byte, delivered int64) {
			err := client.Write(ctx, func(params store.Grid_write_Params) error {
				feeds, err := params.NewData(1)

				if err != nil {
					return err
				}

				return feeds.Set(0, payload)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			resolved, err := results.Out()
			So(err, ShouldBeNil)

			return bytes.Clone(resolved), results.Delivered()
		}

		Convey("When data carrying every declared field is written", func() {
			declare("trade.price,trade.qty")

			payload, err := sonic.Marshal(map[string]any{
				"trade":     map[string]any{"price": 100.0, "qty": 2.0, "side": "buy"},
				"unrelated": 1.0,
			})
			So(err, ShouldBeNil)

			out, delivered := publish(payload)

			Convey("Then only the declared fields are delivered", func() {
				So(delivered, ShouldEqual, 2)

				var resolved map[string]any
				So(sonic.Unmarshal(out, &resolved), ShouldBeNil)
				So(resolved, ShouldResemble, map[string]any{
					"trade.price": 100.0,
					"trade.qty":   2.0,
				})
			})
		})

		Convey("When data is missing one of the declared fields", func() {
			declare("trade.price,trade.qty")

			payload, err := sonic.Marshal(map[string]any{
				"trade": map[string]any{"price": 100.0},
			})
			So(err, ShouldBeNil)

			out, delivered := publish(payload)

			Convey("Then nothing is delivered, because a partial reading is not a reading", func() {
				So(delivered, ShouldEqual, 0)
				So(len(out), ShouldEqual, 0)
			})
		})

		Convey("When several feeds land on the one data port", func() {
			declare("trade.price")

			first, err := sonic.Marshal(map[string]any{"unrelated": 1.0})
			So(err, ShouldBeNil)

			second, err := sonic.Marshal(map[string]any{"trade": map[string]any{"price": 42.0}})
			So(err, ShouldBeNil)

			err = client.Write(ctx, func(params store.Grid_write_Params) error {
				feeds, err := params.NewData(2)

				if err != nil {
					return err
				}

				if err := feeds.Set(0, first); err != nil {
					return err
				}

				return feeds.Set(1, second)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			Convey("Then a feed carrying the declared field is observed, not just the first", func() {
				So(results.Delivered(), ShouldEqual, 1)

				resolved, err := results.Out()
				So(err, ShouldBeNil)

				var carried map[string]any
				So(sonic.Unmarshal(resolved, &carried), ShouldBeNil)
				So(carried["trade.price"], ShouldEqual, 42.0)
			})
		})

		Convey("When no metric has been wired into the grid", func() {
			declare("trade.price")

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			Convey("Then the grid reports that it is serving none", func() {
				So(results.Metrics(), ShouldEqual, 0)
			})
		})
	})
}
