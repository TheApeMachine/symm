package momentum_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/momentum"
)

func TestPvo(t *testing.T) {
	Convey("Given a Pvo server and client", t, func() {
		ctx := context.Background()
		server := momentum.NewPvo(ctx)
		So(server, ShouldNotBeNil)

		client := momentum.Pvo_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params momentum.Pvo_write_Params) error {
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
