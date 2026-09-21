package controlflow_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/controlflow"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestLoopPrimitive(t *testing.T) {
	Convey("Given a native Loop Cap'n Proto server", t, func() {
		ctx := context.Background()
		server := controlflow.NewLoop(ctx)
		So(server, ShouldNotBeNil)

		client := controlflow.Loop_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When active with a limit of 3", func() {
			payload := []byte(`{"ping":true}`)

			// Iteration 1
			err := client.Write(ctx, func(params controlflow.Loop_write_Params) error {
				params.SetActive(true)
				params.SetLimit(3)
				return params.SetData(payload)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future1, release1 := client.Done(ctx, nil)
			defer release1()
			res1, err := future1.Struct()
			So(err, ShouldBeNil)
			So(res1.Index(), ShouldEqual, 1)
			So(res1.Done(), ShouldBeFalse)
			out1, err := res1.Out()
			So(err, ShouldBeNil)
			So(string(out1), ShouldEqual, string(payload))
			So(server.Status(), ShouldEqual, runtime.READY)

			// Iteration 2
			err = client.Write(ctx, func(params controlflow.Loop_write_Params) error {
				params.SetActive(true)
				params.SetLimit(3)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future2, release2 := client.Done(ctx, nil)
			defer release2()
			res2, err := future2.Struct()
			So(err, ShouldBeNil)
			So(res2.Index(), ShouldEqual, 2)
			So(res2.Done(), ShouldBeFalse)

			// Iteration 3 (limit reached)
			err = client.Write(ctx, func(params controlflow.Loop_write_Params) error {
				params.SetActive(true)
				params.SetLimit(3)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future3, release3 := client.Done(ctx, nil)
			defer release3()
			res3, err := future3.Struct()
			So(err, ShouldBeNil)
			So(res3.Index(), ShouldEqual, 3)
			So(res3.Done(), ShouldBeFalse)

			// Iteration 4 (beyond limit: done is true, status is DONE)
			err = client.Write(ctx, func(params controlflow.Loop_write_Params) error {
				params.SetActive(true)
				params.SetLimit(3)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future4, release4 := client.Done(ctx, nil)
			defer release4()
			res4, err := future4.Struct()
			So(err, ShouldBeNil)
			So(res4.Done(), ShouldBeTrue)
			So(server.Status(), ShouldEqual, runtime.DONE)
		})

		Convey("When active is false, loop pauses in WAITING", func() {
			err := client.Write(ctx, func(params controlflow.Loop_write_Params) error {
				params.SetActive(false)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			So(server.Status(), ShouldEqual, runtime.WAITING)
		})
	})
}
