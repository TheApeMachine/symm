package volume_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/volume"
)

func TestKvo(t *testing.T) {
	Convey("Given a Kvo server and client", t, func() {
		ctx := context.Background()
		server := volume.NewKvo(ctx)
		So(server, ShouldNotBeNil)

		client := volume.Kvo_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params volume.Kvo_write_Params) error {
				params.SetHigh(110.0)
				params.SetLow(90.0)
				params.SetVolume(1000.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.IsValid(), ShouldBeTrue)
		})
	})
}
