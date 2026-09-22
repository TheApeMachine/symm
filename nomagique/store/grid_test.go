package store_test

import (
	"bytes"
	"context"
	"strings"
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

		// What the grid hands its metrics: one slot per declared interest,
		// and which of them this record carried.
		slots := func(payload []byte) (values []float64, present []bool) {
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

			delivered, err := results.Values()
			So(err, ShouldBeNil)

			carried, err := results.Present()
			So(err, ShouldBeNil)

			for index := range delivered.Len() {
				values = append(values, delivered.At(index))
				present = append(present, carried.At(index))
			}

			return values, present
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

			// One record comes from one feed and carries that feed's fields.
			// Refusing anything less than all of them means a grid fed by
			// several venues delivers nothing at all.
			Convey("Then what it does carry is delivered", func() {
				So(delivered, ShouldEqual, 1)

				var resolved map[string]any
				So(sonic.Unmarshal(out, &resolved), ShouldBeNil)
				So(resolved, ShouldResemble, map[string]any{"trade.price": 100.0})
			})
		})

		Convey("When a declared field is absent from the data", func() {
			declare("ticker.last,trade.price")

			payload, err := sonic.Marshal(map[string]any{
				"trade": map[string]any{"price": 100.0},
			})
			So(err, ShouldBeNil)

			values, present := slots(payload)

			Convey("Then every interest keeps its own slot", func() {
				So(len(values), ShouldEqual, 2)
				So(present, ShouldResemble, []bool{false, true})

				// The absent field does not close the gap: trade.price is
				// read where it was declared, not where its neighbour was.
				So(values[1], ShouldEqual, 100.0)
			})
		})

		Convey("When a declared interest asks what a field holds", func() {
			declare("trade.side=sell,trade.price")

			sell, err := sonic.Marshal(map[string]any{
				"trade": map[string]any{"side": "sell", "price": 100.0},
			})
			So(err, ShouldBeNil)

			values, present := slots(sell)

			Convey("Then it reads as one when the field holds that literal", func() {
				So(present[0], ShouldBeTrue)
				So(values[0], ShouldEqual, 1.0)
			})

			Convey("And as zero when it holds something else", func() {
				buy, err := sonic.Marshal(map[string]any{
					"trade": map[string]any{"side": "buy", "price": 100.0},
				})
				So(err, ShouldBeNil)

				values, present := slots(buy)
				So(present[0], ShouldBeTrue)
				So(values[0], ShouldEqual, 0.0)
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

/*
Interests are configuration, not news: the same ones arrive with every
observation. Saying so once is the difference between a log and a flood.
*/
func TestGridDeclaresOnce(t *testing.T) {
	Convey("Given a grid told the same interests on every observation", t, func() {
		ctx := context.Background()
		server := store.NewGrid(ctx)

		client := store.Grid_ServerToClient(server)
		defer client.Release()

		declare := func(interests string) {
			err := client.Write(ctx, func(params store.Grid_write_Params) error {
				return params.SetInterests(interests)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
		}

		announcements := func() int {
			spoken := 0

			for _, entry := range server.Logs() {
				if strings.Contains(entry.Message, "delivering") {
					spoken++
				}
			}

			return spoken
		}

		Convey("When the same interests are declared repeatedly", func() {
			for range 5 {
				declare("trade.price,trade.qty")
			}

			Convey("Then it is said once", func() {
				So(announcements(), ShouldEqual, 1)
			})
		})

		Convey("When the interests actually change", func() {
			declare("trade.price")
			declare("trade.price,trade.qty")

			Convey("Then the change is said", func() {
				So(announcements(), ShouldEqual, 2)
			})
		})
	})
}
