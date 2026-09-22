package sampling_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/sampling"
)

func TestRejection(t *testing.T) {
	Convey("Given a Rejection server and client", t, func() {
		ctx := context.Background()
		server := sampling.NewRejection(ctx)
		So(server, ShouldNotBeNil)

		client := sampling.Rejection_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params sampling.Rejection_write_Params) error {
				params.SetCount(5)
				params.SetC(2.0)
				params.SetTargetScale(1.0)
				params.SetProposalScale(1.5)
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
