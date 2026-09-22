package momentum_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/momentum"
)

func TestTdSequential(t *testing.T) {
	Convey("Given a TdSequential server and client", t, func() {
		ctx := context.Background()
		server := momentum.NewTdSequential(ctx)
		So(server, ShouldNotBeNil)

		client := momentum.TdSequential_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params momentum.TdSequential_write_Params) error {
				params.SetClose(100.0)
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
