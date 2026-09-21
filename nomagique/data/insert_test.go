package data_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func insert(
	ctx context.Context, client data.Insert, path string, value float64, payload []byte,
) ([]byte, error) {
	err := client.Write(ctx, func(params data.Insert_write_Params) error {
		params.SetValue(value)

		if err := params.SetPath(path); err != nil {
			return err
		}

		if len(payload) == 0 {
			return nil
		}

		return params.SetData(payload)
	})

	if err != nil {
		return nil, err
	}

	if err := client.WaitStreaming(); err != nil {
		return nil, err
	}

	future, release := client.Done(ctx, nil)
	defer release()

	results, err := future.Struct()

	if err != nil {
		return nil, err
	}

	out, err := results.Out()

	if err != nil {
		return nil, err
	}

	return bytes.Clone(out), nil
}

func TestInsert(t *testing.T) {
	ctx := context.Background()

	Convey("Given an Insert primitive", t, func() {
		client := data.Insert_ServerToClient(data.NewInsert(ctx))

		Convey("It names a value into an empty structure", func() {
			out, err := insert(ctx, client, "spread", 1.5, nil)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)
			So(document["spread"], ShouldEqual, 1.5)
		})

		Convey("It inserts into an existing structure without losing fields", func() {
			payload, err := sonic.Marshal(map[string]any{"bid": 100.0})
			So(err, ShouldBeNil)

			out, err := insert(ctx, client, "ask", 101.0, payload)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)
			So(document["bid"], ShouldEqual, 100.0)
			So(document["ask"], ShouldEqual, 101.0)
		})

		Convey("It creates the objects a nested path implies", func() {
			out, err := insert(ctx, client, "touch.bid.notional", 250.0, nil)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)

			touch := document["touch"].(map[string]any)
			bid := touch["bid"].(map[string]any)
			So(bid["notional"], ShouldEqual, 250.0)
		})

		Convey("It indexes an array when a segment is an integer", func() {
			out, err := insert(ctx, client, "levels.1.price", 99.5, nil)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)

			levels := document["levels"].([]any)
			So(len(levels), ShouldEqual, 2)
			So(levels[0], ShouldBeNil)
			So(levels[1].(map[string]any)["price"], ShouldEqual, 99.5)
		})

		Convey("Chained inserts accumulate into one structure", func() {
			first, err := insert(ctx, client, "bid", 100.0, nil)
			So(err, ShouldBeNil)

			second, err := insert(ctx, client, "ask", 101.0, first)
			So(err, ShouldBeNil)

			out, err := insert(ctx, client, "spread", 1.0, second)
			So(err, ShouldBeNil)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)
			So(document["bid"], ShouldEqual, 100.0)
			So(document["ask"], ShouldEqual, 101.0)
			So(document["spread"], ShouldEqual, 1.0)
		})

		Convey("It reports a path addressing a non-object as invalid", func() {
			payload, err := sonic.Marshal(map[string]any{"bid": 100.0})
			So(err, ShouldBeNil)

			_, err = insert(ctx, client, "bid.notional", 1.0, payload)
			So(err, ShouldNotBeNil)
		})

		Convey("It reports an undefined path rather than inventing one", func() {
			_, err := insert(ctx, client, "", 1.0, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("It reports a payload that is not a structure", func() {
			_, err := insert(ctx, client, "spread", 1.0, []byte("not json"))
			So(err, ShouldNotBeNil)
		})

		Convey("It publishes its status", func() {
			err := client.Write(ctx, func(params data.Insert_write_Params) error {
				params.SetValue(1.0)
				return params.SetPath("spread")
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Status(), ShouldEqual, runtime.Status(runtime.READY))
		})
	})
}
