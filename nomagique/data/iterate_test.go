package data_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"testing"
)

func TestIterate(t *testing.T) {
	ctx := context.Background()

	Convey("Given an Iterate primitive", t, func() {
		client := data.Iterate_ServerToClient(data.NewIterate(ctx))

		payload, err := sonic.Marshal(map[string]any{
			"bids": []any{
				map[string]any{"price": 100.0},
				map[string]any{"price": 99.0},
				map[string]any{"price": 98.0},
			},
		})
		So(err, ShouldBeNil)

		step := func(send []byte) (out []byte, index, count int64, last, found bool) {
			err := client.Write(ctx, func(params data.Iterate_write_Params) error {
				if err := params.SetPath("bids"); err != nil {
					return err
				}

				if len(send) == 0 {
					return nil
				}

				values, err := params.NewData(1)
				if err != nil {
					return err
				}
				return values.Set(0, send)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			emitted, err := results.Out()
			So(err, ShouldBeNil)

			return bytes.Clone(emitted),
				results.Index(),
				results.Count(),
				results.Last(),
				results.Found()
		}

		Convey("It walks one element per evaluation", func() {
			first, index, count, last, found := step(payload)
			So(found, ShouldBeTrue)
			So(index, ShouldEqual, 0)
			So(count, ShouldEqual, 3)
			So(last, ShouldBeFalse)

			var element map[string]any
			So(sonic.Unmarshal(first, &element), ShouldBeNil)
			So(element["price"], ShouldEqual, 100.0)

			_, index, _, last, found = step(nil)
			So(found, ShouldBeTrue)
			So(index, ShouldEqual, 1)
			So(last, ShouldBeFalse)

			third, index, _, last, found := step(nil)
			So(found, ShouldBeTrue)
			So(index, ShouldEqual, 2)
			So(last, ShouldBeTrue)

			So(sonic.Unmarshal(third, &element), ShouldBeNil)
			So(element["price"], ShouldEqual, 98.0)
		})

		Convey("It reports nothing once the collection is exhausted", func() {
			step(payload)
			step(nil)
			step(nil)

			_, _, _, _, found := step(nil)
			So(found, ShouldBeFalse)
		})

		Convey("It reports a path that is not a collection", func() {
			fresh := data.Iterate_ServerToClient(data.NewIterate(ctx))

			scalar, err := sonic.Marshal(map[string]any{"bids": 1.0})
			So(err, ShouldBeNil)

			err = fresh.Write(ctx, func(params data.Iterate_write_Params) error {
				if err := params.SetPath("bids"); err != nil {
					return err
				}

				values, err := params.NewData(1)
				if err != nil {
					return err
				}
				return values.Set(0, scalar)
			})
			So(err, ShouldBeNil)
			So(fresh.WaitStreaming(), ShouldNotBeNil)
		})
	})
}

func TestIterateWrite(t *testing.T) {
	Convey("Given channel-tagged multi-record collections arriving before prior ones finish", t, func() {
		ctx := context.Background()
		client := data.Iterate_ServerToClient(data.NewIterate(ctx))
		defer client.Release()
		step := func(payload string) (string, uint64, uint64) {
			So(client.Write(ctx, func(args data.Iterate_write_Params) error {
				args.SetEnvelope(true)
				if err := args.SetPath("data"); err != nil {
					return err
				}
				if payload == "" {
					return nil
				}
				arrivals, err := args.NewData(1)
				if err != nil {
					return err
				}
				return arrivals.Set(0, []byte(payload))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			raw, err := result.Out()
			So(err, ShouldBeNil)
			return string(raw), result.Pending(), result.Ignored()
		}
		first, pending, _ := step(`{"channel":"ticker","data":[{"symbol":"A","last":1},{"symbol":"B","last":2}]}`)
		So(pending, ShouldEqual, 1)
		second, pending, _ := step(`{"channel":"trade","data":[{"symbol":"C","last":3}]}`)
		So(pending, ShouldEqual, 1)
		third, pending, ignored := step(`{"channel":"heartbeat"}`)
		So(pending, ShouldEqual, 0)
		So(ignored, ShouldEqual, 1)
		for index, raw := range []string{first, second, third} {
			var record struct {
				Channel string
				Data    struct {
					Symbol string
					Last   int
				}
			}
			So(json.Unmarshal([]byte(raw), &record), ShouldBeNil)
			So(record.Data.Last, ShouldEqual, index+1)
			if index < 2 {
				So(record.Channel, ShouldEqual, "ticker")
			}
			if index == 2 {
				So(record.Channel, ShouldEqual, "trade")
			}
		}
		Convey("Capture identities remain exact through envelope projection", func() {
			projected, _, _ := step(`{"sequence":9007199254740993,"channel":"ticker","data":[{"symbol":"A","sequence":18446744073709551615}]}`)
			So(projected, ShouldContainSubstring, `"sequence":9007199254740993`)
			So(projected, ShouldContainSubstring, `"sequence":18446744073709551615`)
		})
		empty, _, _ := step("")
		So(empty, ShouldBeEmpty)
	})
}

func BenchmarkIterateWrite(b *testing.B) {
	ctx := context.Background()
	client := data.Iterate_ServerToClient(data.NewIterate(ctx))
	defer client.Release()
	payload := []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD","last":100},{"symbol":"ETH/USD","last":200}]}`)
	b.ReportAllocs()
	for b.Loop() {
		for record := 0; record < 2; record++ {
			if err := client.Write(ctx, func(args data.Iterate_write_Params) error {
				args.SetEnvelope(true)
				if err := args.SetPath("data"); err != nil {
					return err
				}
				if record > 0 {
					return nil
				}
				arrivals, err := args.NewData(1)
				if err != nil {
					return err
				}
				return arrivals.Set(0, payload)
			}); err != nil {
				b.Fatal(err)
			}
			if err := client.WaitStreaming(); err != nil {
				b.Fatal(err)
			}
			future, release := client.Done(ctx, nil)
			_, err := future.Struct()
			release()
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
