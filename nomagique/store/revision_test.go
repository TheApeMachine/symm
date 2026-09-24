package store_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestRevisionWrite(t *testing.T) {
	Convey("Given a store.Revision server", t, func() {
		ctx := context.Background()
		server := store.NewRevision(ctx)
		client := store.Revision_ServerToClient(server)
		defer client.Release()

		Convey("When pair evidence documents arrive", func() {
			err := client.Write(ctx, func(params store.Revision_write_Params) error {
				if err := params.SetKey("hy.correlation:absolute.out"); err != nil {
					return err
				}
				return params.SetData([]byte(`{"sympathy":2.5,"magnitude":1.2}`))
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Status(), ShouldEqual, runtime.Status_ready)
			So(results.Revision(), ShouldEqual, 1)
			So(results.Count(), ShouldEqual, 1)

			out, err := results.Out()
			So(err, ShouldBeNil)

			var decoded map[string]map[string]float64
			So(json.Unmarshal(out, &decoded), ShouldBeNil)
			So(decoded["hy.correlation:absolute.out"]["sympathy"], ShouldEqual, 2.5)

			Convey("And a second distinct pair arrives", func() {
				err := client.Write(ctx, func(params store.Revision_write_Params) error {
					if err := params.SetKey("hy.support:zscore.out"); err != nil {
						return err
					}
					return params.SetData([]byte(`{"sympathy":-0.8,"magnitude":0.5}`))
				})
				So(err, ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)

				future2, release2 := client.Done(ctx, nil)
				defer release2()

				results2, err := future2.Struct()
				So(err, ShouldBeNil)
				So(results2.Revision(), ShouldEqual, 2)
				So(results2.Count(), ShouldEqual, 2)

				out2, err := results2.Out()
				So(err, ShouldBeNil)
				var decoded2 map[string]map[string]float64
				So(json.Unmarshal(out2, &decoded2), ShouldBeNil)
				So(decoded2["hy.correlation:absolute.out"]["sympathy"], ShouldEqual, 2.5)
				So(decoded2["hy.support:zscore.out"]["sympathy"], ShouldEqual, -0.8)
			})
		})

		Convey("When reset is signalled", func() {
			err := client.Write(ctx, func(params store.Revision_write_Params) error {
				params.SetReset(true)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Count(), ShouldEqual, 0)
			So(results.Revision(), ShouldEqual, 0)
		})
	})
}
