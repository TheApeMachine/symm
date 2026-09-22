package probability_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestPoisson(t *testing.T) {
	Convey("Given a Poisson server and client", t, func() {
		ctx := context.Background()
		server := probability.NewPoisson(ctx)
		So(server, ShouldNotBeNil)

		client := probability.Poisson_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params probability.Poisson_write_Params) error {
				params.SetLambda(2.0)
				params.SetX(2.0)
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
