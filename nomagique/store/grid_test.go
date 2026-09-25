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

		Convey("When one feed's reading is held for another's records", func() {
			declare("ticker.data.last,held:ticker.data.last,futures.data.mark")

			spot, err := sonic.Marshal(map[string]any{"channel": "ticker", "data": map[string]any{"last": 100.0}})
			So(err, ShouldBeNil)
			futures, err := sonic.Marshal(map[string]any{"channel": "futures", "data": map[string]any{"mark": 101.0}})
			So(err, ShouldBeNil)

			Convey("Then nothing is held before the field is ever carried", func() {
				_, present := slots(futures)
				So(present, ShouldResemble, []bool{false, false, true})

				Convey("and once carried, the held reading rides along with the other feed", func() {
					values, present := slots(spot)
					So(present, ShouldResemble, []bool{true, true, false})
					So(values[1], ShouldEqual, 100)

					values, present = slots(futures)
					So(present, ShouldResemble, []bool{false, true, true})
					So(values[1], ShouldEqual, 100)
					So(values[2], ShouldEqual, 101)
				})
			})
		})

		Convey("When a level 3 order book update is written", func() {
			declare("level3.data.bid,level3.data.bid_qty,level3.data.ask,level3.data.ask_qty,level3.data.bids")

			l3, err := sonic.Marshal(map[string]any{
				"channel": "level3",
				"data": map[string]any{
					"symbol": "BTC/USD",
					"bids": []any{
						map[string]any{"limit_price": 50000.0, "order_qty": 1.5},
						map[string]any{"limit_price": 49990.0, "order_qty": 2.0},
					},
					"asks": []any{
						map[string]any{"limit_price": 50001.0, "order_qty": 3.0},
					},
				},
			})
			So(err, ShouldBeNil)

			values, present := slots(l3)
			So(present, ShouldResemble, []bool{true, true, true, true, true})
			So(values[0], ShouldEqual, 50000.0)
			So(values[1], ShouldEqual, 1.5)
			So(values[2], ShouldEqual, 50001.0)
			So(values[3], ShouldEqual, 3.0)
			So(values[4], ShouldEqual, 3.5)
		})

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

		Convey("When metrics are wired into the grid", func() {
			err := client.Write(ctx, func(params store.Grid_write_Params) error {
				metrics, err := params.NewMetrics(3)
				if err != nil {
					return err
				}
				metrics.Set(0, 1.25)
				metrics.Set(1, -0.5)
				metrics.Set(2, 3.75)

				present, err := params.NewPresent(3)
				if err != nil {
					return err
				}
				present.Set(0, true)
				present.Set(1, false)
				present.Set(2, true)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			Convey("Then the grid reports metric count, observations, and presence", func() {
				So(results.Metrics(), ShouldEqual, 3)

				observations, err := results.Observations()
				So(err, ShouldBeNil)
				So(observations.Len(), ShouldEqual, 3)
				So(observations.At(0), ShouldEqual, 1.25)
				So(observations.At(2), ShouldEqual, 3.75)

				observed, err := results.Observed()
				So(err, ShouldBeNil)
				So(observed.Len(), ShouldEqual, 3)
				So(observed.At(0), ShouldBeTrue)
				So(observed.At(1), ShouldBeFalse)
				So(observed.At(2), ShouldBeTrue)

				Convey("And a reading is handed out once: the next evaluation has observed nothing new", func() {
					future, release := client.Done(ctx, nil)
					defer release()

					results, err := future.Struct()
					So(err, ShouldBeNil)
					observed, err := results.Observed()
					So(err, ShouldBeNil)
					So(observed.At(0), ShouldBeFalse)
					So(observed.At(2), ShouldBeFalse)
				})
			})
		})
	})
}

func TestGridScope(t *testing.T) {
	Convey("Given a grid holding a reading under one scope", t, func() {
		ctx := context.Background()
		client := store.Grid_ServerToClient(store.NewGrid(ctx))
		defer client.Release()

		write := func(scopes []string, reading bool) {
			So(client.Write(ctx, func(params store.Grid_write_Params) error {
				names, err := params.NewScope(int32(len(scopes)))

				if err != nil {
					return err
				}

				for position, scope := range scopes {
					if err := names.Set(position, scope); err != nil {
						return err
					}
				}

				if !reading {
					return nil
				}

				metrics, err := params.NewMetrics(1)

				if err != nil {
					return err
				}

				metrics.Set(0, 7)
				present, err := params.NewPresent(1)

				if err != nil {
					return err
				}

				present.Set(0, true)
				return nil
			}), ShouldBeNil)
		}

		done := func() (bool, string) {
			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			observed, err := results.Observed()
			So(err, ShouldBeNil)
			scope, err := results.Scope()
			So(err, ShouldBeNil)
			return observed.Len() > 0 && observed.At(0), scope
		}

		write([]string{"first"}, true)
		So(client.WaitStreaming(), ShouldBeNil)

		Convey("A write that carries no scope keeps the series", func() {
			write(nil, false)
			So(client.WaitStreaming(), ShouldBeNil)
			observed, scope := done()
			So(observed, ShouldBeTrue)
			So(scope, ShouldEqual, "first")
		})

		Convey("A new scope hands out nothing observed under the previous one", func() {
			write([]string{"second"}, false)
			So(client.WaitStreaming(), ShouldBeNil)
			observed, scope := done()
			So(observed, ShouldBeFalse)
			So(scope, ShouldEqual, "second")
		})

		Convey("Data written together under different scopes is rejected", func() {
			write([]string{"second", "third"}, false)
			So(client.WaitStreaming(), ShouldNotBeNil)
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
				if strings.Contains(entry.Message, "registered") {
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
