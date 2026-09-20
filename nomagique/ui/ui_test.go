package ui_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/ui"
)

func TestUIPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given UI primitives", t, func() {
		Convey("Broadcast streams data through Done", func() {
			server := ui.NewBroadcast()
			client := ui.Broadcast_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params ui.Broadcast_write_Params) error {
				return params.SetIn([]byte("dashboard_event"))
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "dashboard_event")
		})
	})
}
