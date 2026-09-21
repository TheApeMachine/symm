package execution_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/execution"
)

func TestExecutionPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given execution primitives", t, func() {
		Convey("Decide selects winner above minContrast", func() {
			server := execution.NewDecide()
			client := execution.Decide_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params execution.Decide_write_Params) error {
				params.SetMinContrast(0.5)
				params.SetWinner("buy")
				params.SetContrast(0.8)
				params.SetIsBreak(false)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(out, ShouldEqual, "buy")

			Convey("When contrast is below minContrast, it waits", func() {
				err = client.Write(ctx, func(params execution.Decide_write_Params) error {
					params.SetMinContrast(0.5)
					params.SetWinner("buy")
					params.SetContrast(0.2)
					params.SetIsBreak(false)
					return nil
				})
				So(err, ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)

				secondFuture, secondRelease := client.Done(ctx, nil)
				defer secondRelease()

				secondResults, err := secondFuture.Struct()
				So(err, ShouldBeNil)

				secondOut, err := secondResults.Out()
				So(err, ShouldBeNil)
				So(secondOut, ShouldEqual, "wait")
			})
		})

		Convey("Gate controls position entry and exit state", func() {
			server := execution.NewGate()
			client := execution.Gate_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params execution.Gate_write_Params) error {
				return params.SetText("enter")
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(out, ShouldEqual, "enter")
		})

		Convey("Submit produces WireIntent with timestamp", func() {
			server := execution.NewSubmit()
			client := execution.Submit_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params execution.Submit_write_Params) error {
				params.SetAction("enter")
				params.SetSymbol("BTC/USD")
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			sym, _ := results.Symbol()
			action, _ := results.Action()
			status, _ := results.Status()
			So(sym, ShouldEqual, "BTC/USD")
			So(action, ShouldEqual, "enter")
			So(status, ShouldEqual, "SUBMITTED")
			So(results.Timestamp(), ShouldBeGreaterThan, 0)
		})
	})
}
