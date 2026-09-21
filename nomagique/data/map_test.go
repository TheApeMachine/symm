package data_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
scale is a function a graph wires into a body port. It stands for whatever
node the graph connects there.
*/
type scale struct {
	factor float64
	calls  int
}

func (fn *scale) Apply(ctx context.Context, call data.Transform_apply) error {
	results, err := call.AllocResults()

	if err != nil {
		return err
	}

	fn.calls++
	results.SetOut(call.Args().Value() * fn.factor)

	return nil
}

func TestMap(t *testing.T) {
	ctx := context.Background()

	Convey("Given a Map primitive with a function wired to body", t, func() {
		fn := &scale{factor: 2}
		body := data.Transform_ServerToClient(fn)
		client := data.Map_ServerToClient(data.NewMap(ctx))

		apply := func(path string, payload []byte) ([]byte, int64, error) {
			err := client.Write(ctx, func(params data.Map_write_Params) error {
				if err := params.SetPath(path); err != nil {
					return err
				}

				if err := params.SetBody(body.AddRef()); err != nil {
					return err
				}

				return params.SetData(payload)
			})

			if err != nil {
				return nil, 0, err
			}

			if err := client.WaitStreaming(); err != nil {
				return nil, 0, err
			}

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()

			if err != nil {
				return nil, 0, err
			}

			out, err := results.Out()

			if err != nil {
				return nil, 0, err
			}

			return bytes.Clone(out), results.Count(), nil
		}

		Convey("It applies the function to every element of a collection", func() {
			payload, err := sonic.Marshal(map[string]any{
				"sizes": []any{1.0, 2.0, 3.0},
			})
			So(err, ShouldBeNil)

			out, count, err := apply("sizes", payload)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 3)
			So(fn.calls, ShouldEqual, 3)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)

			sizes := document["sizes"].([]any)
			So(sizes[0], ShouldEqual, 2.0)
			So(sizes[1], ShouldEqual, 4.0)
			So(sizes[2], ShouldEqual, 6.0)
		})

		Convey("It transforms a value inside every element", func() {
			payload, err := sonic.Marshal(map[string]any{
				"bids": []any{
					map[string]any{"price": 100.0, "qty": 1.0},
					map[string]any{"price": 99.0, "qty": 2.0},
				},
			})
			So(err, ShouldBeNil)

			out, count, err := apply("bids[].qty", payload)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 2)

			var document map[string]any
			So(sonic.Unmarshal(out, &document), ShouldBeNil)

			bids := document["bids"].([]any)
			first := bids[0].(map[string]any)
			second := bids[1].(map[string]any)

			So(first["qty"], ShouldEqual, 2.0)
			So(second["qty"], ShouldEqual, 4.0)
			So(first["price"], ShouldEqual, 100.0)
		})

		Convey("It maps a whole collection within one observation", func() {
			elements := make([]any, 64)

			for index := range elements {
				elements[index] = 1.0
			}

			payload, err := sonic.Marshal(map[string]any{"sizes": elements})
			So(err, ShouldBeNil)

			_, count, err := apply("sizes", payload)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 64)
		})

		Convey("It reports a path that is not a collection", func() {
			payload, err := sonic.Marshal(map[string]any{"sizes": 1.0})
			So(err, ShouldBeNil)

			_, _, err = apply("sizes", payload)
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given a Map primitive with nothing wired to body", t, func() {
		client := data.Map_ServerToClient(data.NewMap(ctx))

		payload, err := sonic.Marshal(map[string]any{"sizes": []any{1.0}})
		So(err, ShouldBeNil)

		err = client.Write(ctx, func(params data.Map_write_Params) error {
			if err := params.SetPath("sizes"); err != nil {
				return err
			}

			return params.SetData(payload)
		})
		So(err, ShouldBeNil)

		Convey("It reports the missing function rather than passing values through", func() {
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}
