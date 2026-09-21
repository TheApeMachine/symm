package controlflow

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestBatchServer(t *testing.T) {
	Convey("Given a BatchServer with size 3", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewBatch(ctx)
		So(server, ShouldNotBeNil)
		So(server.Status(), ShouldEqual, runtime.READY)

		capBatch := Batch_ServerToClient(server)
		So(capBatch.IsValid(), ShouldBeTrue)

		Convey("Accumulating items below batch size", func() {
			err := capBatch.Write(ctx, func(params Batch_write_Params) error {
				params.SetSize(3)
				return params.SetItem([]byte(`"BTC/USD"`))
			})
			So(err, ShouldBeNil)

			future, release := capBatch.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Count(), ShouldEqual, 1)
			So(results.Ready(), ShouldBeFalse)
			So(results.HasOut(), ShouldBeFalse)
		})

		Convey("Reaching batch size emits batched array", func() {
			items := [][]byte{
				[]byte(`"BTC/USD"`),
				[]byte(`"ETH/USD"`),
				[]byte(`"SOL/USD"`),
			}

			for i, it := range items {
				err := capBatch.Write(ctx, func(params Batch_write_Params) error {
					params.SetSize(3)
					return params.SetItem(it)
				})
				So(err, ShouldBeNil)

				future, release := capBatch.Done(ctx, nil)
				defer release()

				results, err := future.Struct()
				So(err, ShouldBeNil)

				if i < 2 {
					So(results.Ready(), ShouldBeFalse)
					So(results.Count(), ShouldEqual, int64(i+1))
				}

				if i == 2 {
					So(results.Ready(), ShouldBeTrue)
					So(results.HasOut(), ShouldBeTrue)

					out, err := results.Out()
					So(err, ShouldBeNil)
					So(string(out), ShouldEqual, `["BTC/USD","ETH/USD","SOL/USD"]`)
				}
			}
		})

		Convey("Flush emits partial batch early", func() {
			err := capBatch.Write(ctx, func(params Batch_write_Params) error {
				params.SetSize(5)
				return params.SetItem([]byte(`"XRP/USD"`))
			})
			So(err, ShouldBeNil)

			err = capBatch.Write(ctx, func(params Batch_write_Params) error {
				params.SetFlush(true)
				return nil
			})
			So(err, ShouldBeNil)

			future, release := capBatch.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Ready(), ShouldBeTrue)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, `["XRP/USD"]`)
		})
	})
}
