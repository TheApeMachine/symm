package data_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestFilter(t *testing.T) {
	ctx := context.Background()

	Convey("Given a Filter primitive", t, func() {
		client := data.Filter_ServerToClient(data.NewFilter(ctx))

		payload, err := sonic.Marshal(map[string]any{"qty": 5.0})
		So(err, ShouldBeNil)

		apply := func(path, operator string, threshold float64) (bool, error) {
			err := client.Write(ctx, func(params data.Filter_write_Params) error {
				params.SetThreshold(threshold)

				if err := params.SetPath(path); err != nil {
					return err
				}

				if err := params.SetOperator(operator); err != nil {
					return err
				}

				return params.SetData(payload)
			})

			if err != nil {
				return false, err
			}

			if err := client.WaitStreaming(); err != nil {
				return false, err
			}

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()

			if err != nil {
				return false, err
			}

			return results.Passed(), nil
		}

		Convey("It passes a structure that satisfies the comparison", func() {
			passed, err := apply("qty", ">", 1.0)
			So(err, ShouldBeNil)
			So(passed, ShouldBeTrue)
		})

		Convey("It withholds a structure that does not", func() {
			passed, err := apply("qty", "<", 1.0)
			So(err, ShouldBeNil)
			So(passed, ShouldBeFalse)
		})

		Convey("It does not pass a path the structure lacks", func() {
			passed, err := apply("missing", ">", 1.0)
			So(err, ShouldBeNil)
			So(passed, ShouldBeFalse)
		})

		Convey("It reports an operator it does not implement", func() {
			_, err := apply("qty", "~=", 1.0)
			So(err, ShouldNotBeNil)
		})
	})
}

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

				return params.SetData(send)
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

				return params.SetData(scalar)
			})
			So(err, ShouldBeNil)
			So(fresh.WaitStreaming(), ShouldNotBeNil)
		})
	})
}

func TestReduce(t *testing.T) {
	ctx := context.Background()

	Convey("Given a Reduce primitive", t, func() {
		client := data.Reduce_ServerToClient(data.NewReduce(ctx))

		fold := func(operator string, value float64, flush bool) (float64, bool, error) {
			err := client.Write(ctx, func(params data.Reduce_write_Params) error {
				params.SetValue(value)
				params.SetFlush(flush)
				return params.SetOperator(operator)
			})

			if err != nil {
				return 0, false, err
			}

			if err := client.WaitStreaming(); err != nil {
				return 0, false, err
			}

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()

			if err != nil {
				return 0, false, err
			}

			return results.Out(), results.Ready(), nil
		}

		Convey("It sums a collection and publishes on flush", func() {
			_, ready, err := fold("sum", 1.0, false)
			So(err, ShouldBeNil)
			So(ready, ShouldBeFalse)

			_, ready, err = fold("sum", 2.0, false)
			So(err, ShouldBeNil)
			So(ready, ShouldBeFalse)

			out, ready, err := fold("sum", 3.0, true)
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(out, ShouldEqual, 6.0)
		})

		Convey("It averages a collection", func() {
			fold("mean", 2.0, false)
			fold("mean", 4.0, false)

			out, ready, err := fold("mean", 6.0, true)
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(out, ShouldEqual, 4.0)
		})

		Convey("It takes the extremes of a collection", func() {
			fold("max", 2.0, false)
			out, _, err := fold("max", 9.0, true)
			So(err, ShouldBeNil)
			So(out, ShouldEqual, 9.0)

			fold("min", 5.0, false)
			out, _, err = fold("min", 3.0, true)
			So(err, ShouldBeNil)
			So(out, ShouldEqual, 3.0)
		})

		Convey("It starts a new fold after publishing", func() {
			fold("sum", 10.0, true)

			out, ready, err := fold("sum", 4.0, true)
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(out, ShouldEqual, 4.0)
		})

		Convey("It reports an operator it does not implement", func() {
			_, _, err := fold("median", 1.0, false)
			So(err, ShouldNotBeNil)
		})
	})
}
